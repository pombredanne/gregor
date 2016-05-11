package rpc

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
)

type aliveGroup struct {
	group     map[string]rpc.GenericClient
	status    Aliver
	authToken gregor1.SessionToken
	selfHost  string
	clock     clockwork.Clock
	done      chan struct{}
	log       rpc.LogOutput
	sync.RWMutex
}

func newAliveGroup(status Aliver, selfHost string, authToken gregor1.SessionToken, clock clockwork.Clock, done chan struct{}, log rpc.LogOutput) *aliveGroup {
	a := &aliveGroup{
		group:     make(map[string]rpc.GenericClient),
		status:    status,
		selfHost:  selfHost,
		authToken: authToken,
		clock:     clock,
		done:      done,
		log:       log,
	}
	a.update()
	go a.check()
	return a
}

func (a *aliveGroup) Publish(ctx context.Context, msg gregor1.Message) error {
	a.RLock()
	defer a.RUnlock()
	perr := &pubErr{}
	for host, cli := range a.group {
		ic := gregor1.IncomingClient{Cli: cli}
		if err := ic.ConsumePubMessage(ctx, msg); err != nil {
			a.log.Warning("host %q consumePubMessage error: %s", host, err)
			perr.Add(host, err)
		} else {
			a.log.Debug("host %q consumePubMessage success", host)
		}
	}

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

	newgroup := make(map[string]rpc.GenericClient)

	a.RLock()
	for _, host := range alive {
		if host == a.selfHost {
			continue
		}
		if cli, ok := a.group[host]; ok {
			newgroup[host] = cli
		} else {
			newcli, err := a.connect(host)
			if err != nil {
				a.log.Warning("error connecting to %q: %s", host, err)
			} else {
				newgroup[host] = newcli
			}
		}
	}

	for host, _ := range a.group {
		if _, exists := newgroup[host]; !exists {
			a.log.Debug("gregord on host %q no longer alive, shutting connection down", host)
			// cli.Shutdown()
		}
	}
	a.RUnlock()

	a.Lock()
	a.group = newgroup
	a.Unlock()

	return nil
}

func (a *aliveGroup) connect(host string) (rpc.GenericClient, error) {
	a.log.Debug("%s: connecting to gregord on host %q", a.selfHost, host)
	c, err := net.Dial("tcp", host)
	if err != nil {
		return nil, err
	}
	t := rpc.NewTransport(c, nil, keybase1.WrapError)
	cli := rpc.NewClient(t, keybase1.ErrorUnwrapper{})
	ac := gregor1.AuthClient{Cli: cli}
	if _, err := ac.AuthenticateSessionToken(context.TODO(), a.authToken); err != nil {
		a.log.Warning("host %q auth error: %s", host, err)
		return nil, err
	}

	a.log.Debug("%s: success connecting to gregord on host %q", a.selfHost, host)

	return cli, nil
}

type hostErr struct {
	host string
	err  error
}

type pubErr struct {
	errors []hostErr
}

func (p *pubErr) Error() string {
	s := make([]string, len(p.errors))
	for i, e := range p.errors {
		s[i] = fmt.Sprintf("host %q: error %s", e.host, e.err)
	}
	return fmt.Sprintf("(errors: %d) %s", len(p.errors), strings.Join(s, ", "))
}

func (p *pubErr) Add(host string, e error) {
	p.errors = append(p.errors, hostErr{host: host, err: e})
}

func (p *pubErr) Empty() bool {
	return len(p.errors) == 0
}
