package main

import (
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"golang.org/x/net/context"
)

// authdHandler implements rpc.ConnectionHandler
type authdHandler struct {
	log       rpc.LogOutput
	connectCh chan rpc.GenericClient
}

var _ rpc.ConnectionHandler = (*authdHandler)(nil)

func NewAuthdHandler(log rpc.LogOutput) authdHandler {
	return authdHandler{
		log:       log,
		connectCh: make(chan rpc.GenericClient),
	}
}

func (a *authdHandler) OnConnect(ctx context.Context, conn *rpc.Connection, cli rpc.GenericClient, srv *rpc.Server) error {
	a.log.Debug("authd handler: connected")
	a.connectCh <- conn.GetClient()
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
