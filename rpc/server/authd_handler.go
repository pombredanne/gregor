package rpc

import (
	"errors"
	"sync"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

// authdHandler implements rpc.ConnectionHandler
type authdHandler struct {
	authserver      gregor1.AuthUpdateInterface
	log             rpc.LogOutput
	superReceivedCh chan struct{}
	refreshInterval time.Duration
	doneCh          chan struct{}
	superToken      gregor1.SessionToken
	sync.Mutex
}

var _ rpc.ConnectionHandler = (*authdHandler)(nil)
var _ gregor1.AuthUpdateInterface = (*authdHandler)(nil)

// NewAuthdHandler creates a new authd handler. It will put super
// tokens for connecting to other gregord services on superCh
// after the first connect to authd and every refreshInterval
// afterwards.
func NewAuthdHandler(authserver gregor1.AuthUpdateInterface, log rpc.LogOutput,
	refreshInterval time.Duration) *authdHandler {
	return &authdHandler{
		authserver:      authserver,
		log:             log,
		superReceivedCh: make(chan struct{}),
		refreshInterval: refreshInterval,
		doneCh:          make(chan struct{}),
	}
}

func (a *authdHandler) GetSuperToken() gregor1.SessionToken {
	a.Lock()
	blank := (a.superToken == "")
	a.Unlock()

	// If we don't have a super token yet, wait for one
	if blank {
		a.log.Debug("authd handler: GetSuperToken(): token blank: waiting...")
		<-a.superReceivedCh
		a.log.Debug("authd handler: GetSuperToken(): token blank: complete")
	}

	a.Lock()
	defer a.Unlock()
	return a.superToken
}

func (a *authdHandler) setSuperToken(cli gregor1.AuthInternalClient) error {

	tok, err := cli.CreateGregorSuperUserSessionToken(context.Background())
	if err != nil {
		a.log.Debug("setSuperToken: error creating super user session token: %s", err)
		return err
	} else if tok == "" {
		a.log.Error("setSuperToken: got a blank token back with no error")
		return errors.New("blank token from auth server")
	} else {
		a.log.Debug("setSuperToken: created super user session token")
	}

	a.Lock()
	sendMsg := (a.superToken == "")
	a.superToken = tok
	a.Unlock()

	// People might be waiting on this, close this to wake them all up
	if sendMsg {
		close(a.superReceivedCh)
	}

	return nil
}

func (a *authdHandler) OnConnect(ctx context.Context, conn *rpc.Connection, cli rpc.GenericClient, srv *rpc.Server) error {
	a.log.Debug("authd handler: registering protocols")
	if err := srv.Register(gregor1.AuthUpdateProtocol(a)); err != nil {
		return err
	}

	// Set new super token
	ac := gregor1.AuthInternalClient{Cli: cli}
	if err := a.setSuperToken(ac); err != nil {
		return err
	}

	// Stop any existing update loops.
	close(a.doneCh)
	a.doneCh = make(chan struct{})

	// Start a new update loop.
	go a.updateSuperToken(conn)

	return nil
}

func (a *authdHandler) OnConnectError(err error, reconnectThrottleDuration time.Duration) {
	a.log.Debug("authd handler: connect error %s, reconnect throttle duration: %s", err, reconnectThrottleDuration)
}

func (a *authdHandler) OnDoCommandError(err error, nextTime time.Duration) {
	a.log.Debug("authd handler: do command error: %s, nextTime: %s", err, nextTime)
}

func (a *authdHandler) OnDisconnected(ctx context.Context, status rpc.DisconnectStatus) {
	a.log.Debug("authd handler: disconnected: %v", status)
}

func (a *authdHandler) ShouldRetry(name string, err error) bool {
	a.log.Debug("authd handler: should retry: name %s, err %v (returning false)", name, err)
	return false
}

func (a *authdHandler) ShouldRetryOnConnect(err error) bool {
	if err == nil {
		return false
	}

	a.log.Debug("authd handler: should retry on connect, err %v", err)
	return true
}

func (a *authdHandler) HandlerName() string {
	return "authd"
}

func (a *authdHandler) RevokeSessionIDs(ctx context.Context, sessionIDs []gregor1.SessionID) error {
	a.log.Debug("authd handler: revoke session IDs")
	return a.authserver.RevokeSessionIDs(ctx, sessionIDs)
}

func (a *authdHandler) updateSuperToken(conn *rpc.Connection) {
	for {
		select {
		case <-a.doneCh:
			return
		case <-time.After(a.refreshInterval):
			a.setSuperToken(gregor1.AuthInternalClient{Cli: conn.GetClient()})
		}
	}
}

func (a *authdHandler) Close() {
	close(a.doneCh)
}
