package rpc

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

// ErrBadUID occurs when there is a bad UID on the auth channel.
var ErrBadUID = errors.New("bad UID on channel")

type authInfo struct {
	// Protects all variables below.
	authLock sync.RWMutex
	tok      gregor1.SessionToken
	res      gregor1.AuthResult
	authTime time.Time
}

func (i *authInfo) get() (
	tok gregor1.SessionToken, res gregor1.AuthResult, authTime time.Time) {
	i.authLock.RLock()
	defer i.authLock.RUnlock()
	return i.tok, i.res, i.authTime
}

func (i *authInfo) set(
	tok gregor1.SessionToken, res gregor1.AuthResult, authTime time.Time) {
	i.authLock.Lock()
	defer i.authLock.Unlock()
	i.tok = tok
	i.res = res
	i.authTime = authTime
}

func (i *authInfo) clear(sid gregor1.SessionID) bool {
	i.authLock.Lock()
	defer i.authLock.Unlock()
	if i.res.Sid != sid {
		return false
	}
	i.tok = ""
	i.res = gregor1.AuthResult{}
	i.authTime = time.Time{}
	return true
}

type connection struct {
	c      net.Conn
	xprt   rpc.Transporter
	parent *Server

	authCh   chan error
	authInfo authInfo

	server *rpc.Server

	// Suitable for receiving externally after
	// startAuthentication() finishes successfully.
	serverDoneCh <-chan struct{}

	log rpc.LogOutput
}

func newConnection(c net.Conn, parent *Server) (*connection, error) {
	xprt := rpc.NewTransport(c, rpc.NewSimpleLogFactory(parent.log, nil), keybase1.WrapError)

	conn := &connection{
		c:      c,
		xprt:   xprt,
		parent: parent,
		authCh: make(chan error, 1),
		log:    parent.log,
	}

	if err := conn.startRPCServer(); err != nil {
		return nil, err
	}

	return conn, nil
}

var superUID = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00}

func matchUIDorSuperuser(uid, expected []byte) bool {
	return bytes.Equal(uid, superUID) || bytes.Equal(uid, expected)
}

func (c *connection) checkUIDAuth(ctx context.Context, uid gregor1.UID) error {
	tok, res, _ := c.authInfo.get()
	if _, err := c.AuthenticateSessionToken(ctx, tok); err != nil {
		return err
	}

	if !matchUIDorSuperuser(res.Uid, uid.Bytes()) {
		return fmt.Errorf("mismatched UIDs: %v != %v", uid, res.Uid)
	}
	return nil
}

func (c *connection) checkMessageAuth(ctx context.Context, m gregor1.Message) error {
	if ibm := m.ToInBandMessage(); ibm != nil {
		if ibm.Metadata() == nil || ibm.Metadata().UID() == nil {
			return errors.New("no valid UID in message")
		}
		if err := c.checkUIDAuth(ctx, ibm.Metadata().UID().Bytes()); err != nil {
			return err
		}
	}
	if oobm := m.ToOutOfBandMessage(); oobm != nil {
		if oobm.UID() == nil {
			return errors.New("no valid UID in message")
		}
		if err := c.checkUIDAuth(ctx, oobm.UID().Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (c *connection) AuthenticateSessionToken(ctx context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {

	c.log.Info("Authenticate: %+v", tok)
	if tok == "" {
		var UnauthenticatedSessionError = keybase1.Status{
			Name: "BAD_SESSION",
			Code: int(keybase1.StatusCode_SCBadSession),
			Desc: "unauthed session"}
		c.log.Error("Authenticate: blank session token for connection!")
		return gregor1.AuthResult{}, UnauthenticatedSessionError
	}

	res, err := c.parent.auth.AuthenticateSessionToken(ctx, tok)
	if err == nil {
		c.authInfo.set(tok, res, c.parent.clock.Now())
	}
	select {
	case c.authCh <- err:
		// First auth call, or first auth call after authCh is
		// read from.
	default:
		// Subsequent auth calls -- just drop.
	}
	return res, err
}

func (c *connection) RevokeSessionIDs(ctx context.Context, sessionIDs []gregor1.SessionID) error {
	for _, sid := range sessionIDs {
		c.log.Info("Revoke: %+v", sid)
		if c.authInfo.clear(sid) {
			break
		}
	}
	return c.parent.auth.RevokeSessionIDs(ctx, sessionIDs)
}

func (c *connection) Sync(ctx context.Context, arg gregor1.SyncArg) (gregor1.SyncResult, error) {
	if err := c.checkUIDAuth(ctx, arg.Uid); err != nil {
		return gregor1.SyncResult{}, err
	}

	return c.parent.startSync(ctx, arg)
}

func (c *connection) ConsumeMessage(ctx context.Context, m gregor1.Message) error {
	c.log.Info("ConsumeMessage: %+v", m)
	if err := c.checkMessageAuth(ctx, m); err != nil {
		return err
	}

	return c.parent.startConsume(ctx, m)
}

func (c *connection) Ping(ctx context.Context) (string, error) {
	return "pong", nil
}

func (c *connection) startRPCServer() error {
	c.server = rpc.NewServer(c.xprt, keybase1.WrapError)

	prots := []rpc.Protocol{
		gregor1.AuthProtocol(c),
		gregor1.IncomingProtocol(c),
	}
	for _, prot := range prots {
		c.log.Info("registering protocol %s", prot.Name)
		if err := c.server.Register(prot); err != nil {
			return err
		}
	}

	c.serverDoneCh = c.server.Run()
	return nil
}

func (c *connection) startAuthentication() error {
	select {
	case <-c.serverDoneCh:
		return c.server.Err()

	case err := <-c.authCh:
		return err
	}
}

func (c *connection) serverDoneChan() <-chan struct{} {
	return c.serverDoneCh
}

// serverDoneErr returns a non-nil error only after serverDoneChan() is
// closed.
func (c *connection) serverDoneErr() error {
	return c.server.Err()
}

func (c *connection) close() {
	// Should trigger the c.serverDoneCh case in
	// startAuthentication.
	c.c.Close()
}

var _ gregor1.AuthInterface = (*connection)(nil)

// var _ gregor1.IncomingInterface = (*connection)(nil)
