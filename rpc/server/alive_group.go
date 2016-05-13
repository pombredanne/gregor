package rpc

import (
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/jonboulle/clockwork"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
)

type aliveGroup struct {
	group     map[string]*sibConn
	status    Aliver
	authToken gregor1.SessionToken
	selfHost  string
	clock     clockwork.Clock
	done      chan struct{}
	log       rpc.LogOutput
	timeout   time.Duration
	sync.RWMutex
}

func newAliveGroup(status Aliver, selfHost string, authToken gregor1.SessionToken, timeout time.Duration, clock clockwork.Clock, done chan struct{}, log rpc.LogOutput) *aliveGroup {
	a := &aliveGroup{
		group:     make(map[string]*sibConn),
		status:    status,
		selfHost:  selfHost,
		authToken: authToken,
		clock:     clock,
		done:      done,
		log:       log,
		timeout:   timeout,
	}
	a.update()
	go a.check()
	return a
}

func (a *aliveGroup) Publish(ctx context.Context, msg gregor1.Message) error {
	a.RLock()
	defer a.RUnlock()

	perr := &pubErr{}

	var wg sync.WaitGroup
	for host, conn := range a.group {
		wg.Add(1)
		go func() {
			if err := conn.CallConsumePubMessage(ctx, msg); err != nil {
				a.log.Warning("host %q consumePubMessage error: %s", host, err)
				perr.Add(host, err)
			} else {
				a.log.Debug("host %q consumePubMessage success", host)
			}
			wg.Done()
		}()
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
	if len(alive) != len(a.group) {
		return true, nil
	}

	for _, host := range alive {
		if _, ok := a.group[host]; !ok {
			return true, nil
		}
	}

	return false, nil
}

func (a *aliveGroup) update() error {
	if a.status == nil {
		return nil
	}
	alive, err := a.status.Alive()
	if err != nil {
		return err
	}

	newgroup := make(map[string]*sibConn)

	a.RLock()
	for _, host := range alive {
		if host == a.selfHost {
			continue
		}
		if conn, ok := a.group[host]; ok {
			newgroup[host] = conn
		} else {
			newconn, err := NewSibConn(host, a.authToken, a.timeout, a.log)
			if err != nil {
				a.log.Warning("error connecting to %q: %s", host, err)
			} else {
				newgroup[host] = newconn
			}
		}
	}

	for host, conn := range a.group {
		if _, exists := newgroup[host]; !exists {
			a.log.Debug("gregord on host %q no longer alive, shutting connection down", host)
			conn.Shutdown()
		}
	}
	a.RUnlock()

	a.Lock()
	a.group = newgroup
	a.Unlock()

	return nil
}
