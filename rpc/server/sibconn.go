package rpc

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/net/context"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/rpc/transport"
)

// sibConn manages a connection to a sibling gregord.
type sibConn struct {
	uri       *rpc.FMPURI
	authToken gregor1.SessionToken
	timeout   time.Duration
	log       rpc.LogOutput
	conn      *rpc.Connection
	logPrefix string
}

func NewSibConn(suri string, authToken gregor1.SessionToken, timeout time.Duration,
	log rpc.LogOutput) (*sibConn, error) {

	uri, err := rpc.ParseFMPURI(suri)
	if err != nil {
		return nil, err
	}

	s := &sibConn{
		uri:       uri,
		authToken: authToken,
		timeout:   timeout,
		log:       log,
		logPrefix: fmt.Sprintf("[sibConn %s]", uri),
	}

	if err := s.connect(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *sibConn) CallConsumePublishMessage(ctx context.Context, msg gregor1.Message) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	ic := gregor1.IncomingClient{Cli: s.conn.GetClient()}
	return ic.ConsumePublishMessage(ctx, msg)
}

func (s *sibConn) Shutdown() {
	if s.conn == nil {
		return
	}
	s.conn.Shutdown()
	s.conn = nil
}

func (s *sibConn) connect() error {

	// Get the key for selecting the right bundled CA
	env := os.Getenv("KEYBASE_RUN_MODE")
	if env == "" {
		env = "staging"
	}

	// Connect to our peer using proper protocol
	if s.uri.UseTLS() {
		s.debug("connecting to gregord (tls)")
		s.conn = rpc.NewTLSConnectionWithServerName(s.uri.HostPort, gregor.TLSHostNames[env],
			[]byte(gregor.BundledCAs[env]), keybase1.ErrorUnwrapper{}, s, true,
			rpc.NewSimpleLogFactory(s.log, nil), keybase1.WrapError, s.log, nil)
	} else {
		s.debug("connecting to gregord (no tls)")
		t := transport.NewConnTransport(s.log, nil, s.uri)
		s.conn = rpc.NewConnectionWithTransport(s, t, keybase1.ErrorUnwrapper{}, true, keybase1.WrapError,
			s.log, nil)
	}
	return nil
}

func (s *sibConn) HandlerName() string {
	return "gregord sibling connection"
}

func (s *sibConn) OnConnect(ctx context.Context, conn *rpc.Connection, cli rpc.GenericClient, srv *rpc.Server) error {
	s.debug("OnConnect")

	ac := gregor1.AuthClient{Cli: cli}
	if _, err := ac.AuthenticateSessionToken(ctx, s.authToken); err != nil {
		s.warning("auth error: %s", err)
		return err
	}

	s.debug("OnConnect - success + auth complete")

	return nil
}

func (s *sibConn) OnConnectError(err error, reconnectThrottleDuration time.Duration) {
	s.debug("OnConnectError %s, reconnect throttle duration: %s", err, reconnectThrottleDuration)
}

func (s *sibConn) OnDisconnected(ctx context.Context, status rpc.DisconnectStatus) {
	var desc string
	switch status {
	case rpc.UsingExistingConnection:
		desc = "UsingExistingconnection"
	case rpc.StartingFirstConnection:
		desc = "StartingFirstConnection"
	case rpc.StartingNonFirstConnection:
		desc = "StartingNonFirstConnection"
	default:
		desc = "unknown status value"
	}
	s.debug("OnDisconnected: %v (%s)", status, desc)
}

func (s *sibConn) OnDoCommandError(err error, nextTime time.Duration) {
	s.debug("OnDoCommandError: %s, nextTime: %s", err, nextTime)
}

func (s *sibConn) ShouldRetry(name string, err error) bool {
	s.debug("ShouldRetry: name %s, err %v (returning false)", name, err)
	return false
}

func (s *sibConn) ShouldRetryOnConnect(err error) bool {
	if err == nil {
		s.debug("ShouldRetryOnconnect: nil err (returning true)")
		// so weird that go-framed-msgpack-rpc calls this
		// with err == nil and actually cares what the return
		// value is...
		return true
	}

	s.debug("ShouldRetryOnconnect: err %v (returning true)", err)
	return true
}

func (s *sibConn) debug(f string, args ...interface{}) {
	s.log.Debug(fmt.Sprintf("%s %s", s.logPrefix, f), args...)
}

func (s *sibConn) warning(f string, args ...interface{}) {
	s.log.Warning(fmt.Sprintf("%s %s", s.logPrefix, f), args...)
}
