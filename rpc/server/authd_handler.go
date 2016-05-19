package rpc

import (
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

// authdHandler implements rpc.ConnectionHandler
type authdHandler struct {
	authserver gregor1.AuthUpdateInterface
	log        rpc.LogOutput
	superCh    chan gregor1.SessionToken
}

var _ rpc.ConnectionHandler = (*authdHandler)(nil)
var _ gregor1.AuthUpdateInterface = (*authdHandler)(nil)

func NewAuthdHandler(authserver gregor1.AuthUpdateInterface, log rpc.LogOutput, superCh chan gregor1.SessionToken) *authdHandler {
	return &authdHandler{
		authserver: authserver,
		log:        log,
		superCh:    superCh,
	}
}

func (a *authdHandler) OnConnect(ctx context.Context, conn *rpc.Connection, cli rpc.GenericClient, srv *rpc.Server) error {
	a.log.Debug("authd handler: registering protocols")
	if err := srv.Register(gregor1.AuthUpdateProtocol(a)); err != nil {
		return err
	}

	// get a super user token
	ac := gregor1.AuthInternalClient{Cli: cli}
	tok, err := ac.CreateGregorSuperUserSessionToken(ctx)
	if err != nil {
		a.log.Debug("authd handler: error creating super user session token: %s", err)
		return err
	}
	a.log.Debug("authd handler: created super user session token")
	a.superCh <- tok

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
