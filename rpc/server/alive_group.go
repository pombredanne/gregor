package rpc

import (
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/jonboulle/clockwork"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/srvup"
)

type aliveGroup struct {
	group       map[srvup.NodeId]*sibConn
	status      Aliver
	authToken   gregor1.SessionToken
	authTokenCh chan gregor1.SessionToken
	selfID      srvup.NodeId
	clock       clockwork.Clock
	done        chan struct{}
	log         rpc.LogOutput
	timeout     time.Duration
	sync.RWMutex
}

func waitToken(authTokenCh chan gregor1.SessionToken, log rpc.LogOutput) gregor1.SessionToken {
	for {
		select {
		case t := <-authTokenCh:
			return t
		case <-time.After(1 * time.Second):
			log.Debug("newAliveGroup: slow receive of super user session token")
		}
	}
}

func newAliveGroup(status Aliver, selfID srvup.NodeId, authTokenCh chan gregor1.SessionToken,
	timeout time.Duration, clock clockwork.Clock, done chan struct{}, log rpc.LogOutput) *aliveGroup {

	authToken := waitToken(authTokenCh, log)
	a := &aliveGroup{
		group:       make(map[srvup.NodeId]*sibConn),
		status:      status,
		selfID:      selfID,
		authToken:   authToken,
		authTokenCh: authTokenCh,
		clock:       clock,
		done:        done,
		log:         log,
		timeout:     timeout,
	}
	a.update()
	go a.check()
	return a
}

func (a *aliveGroup) Publish(ctx context.Context, msg gregor1.Message) error {
	a.RLock()
	defer a.RUnlock()

	perr := &pubErr{}

	a.log.Debug("publishing message to %d peers", len(a.group))
	var wg sync.WaitGroup
	for id, conn := range a.group {
		wg.Add(1)
		go func(id srvup.NodeId, conn *sibConn) {
			if err := conn.CallConsumePublishMessage(ctx, msg); err != nil {
				a.log.Warning("consumePubMessage error: id: %s uri: %s err: %s", id, conn.uri, err)
				perr.Add(conn.uri.String(), err)
			} else {
				a.log.Debug("consumePubMessage success: id: %s uri: %s", id, conn.uri)
			}
			wg.Done()
		}(id, conn)
	}
	wg.Wait()

	if perr.Empty() {
		return nil
	}
	return perr
}

func (a *aliveGroup) check() {
	for {
		select {
		case <-a.done:
			return
		case t := <-a.authTokenCh:
			a.log.Debug("aliveGroup received new super auth token")
			a.authToken = t
		case <-a.clock.After(1 * time.Second):
		}

		changed, err := a.changed()
		if err != nil {
			a.log.Error("aliveGroup changed error: %s", err)
			continue
		}
		if !changed {
			continue
		}

		if err := a.update(); err != nil {
			a.log.Error("aliveGroup update error: %s", err)
			continue
		}
	}
}

func (a *aliveGroup) changed() (bool, error) {
	if a.status == nil {
		return false, nil
	}
	alive, err := a.status.Alive()
	if err != nil {
		return false, err
	}

	a.RLock()
	defer a.RUnlock()

	// Check length for early out
	if len(alive) != len(a.group) {
		return true, nil
	}
	// Compare IDs to see if anything else is different
	for _, node := range alive {
		if _, ok := a.group[node.Id]; !ok {
			return true, nil
		}
	}

	return false, nil
}

func (a *aliveGroup) update() error {
	if a.status == nil {
		return nil
	}

	// Grab current list of alive servers
	alive, err := a.status.Alive()
	if err != nil {
		return err
	}

	newgroup := make(map[srvup.NodeId]*sibConn)

	// Build up new alive group by establishing connections to new gregors
	a.RLock()
	for _, node := range alive {
		// Don't connect to ourselves
		if node.Id == a.selfID {
			continue
		}
		if conn, ok := a.group[node.Id]; ok {
			newgroup[node.Id] = conn
		} else {
			newconn, err := NewSibConn(node.URI, a.authToken, a.timeout, a.log)
			if err != nil {
				a.log.Warning("error connecting to %q: %s", node.URI, err)
			} else {
				newgroup[node.Id] = newconn
			}
		}
	}

	// Shut down gregors that have disappeared
	for id, conn := range a.group {
		if _, exists := newgroup[id]; !exists {
			a.log.Debug("gregord on host [ id: %s uri: %q ] no longer alive, shutting connection down",
				id, conn.uri)
			conn.Shutdown()
		}
	}
	a.RUnlock()

	// Commit the new group with a write lock
	a.Lock()
	a.group = newgroup
	a.Unlock()

	return nil
}
