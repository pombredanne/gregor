package rpc

import (
	"bytes"
	"errors"
	"log"
	"net"
	"sync"
	"time"

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
}

func newConnection(c net.Conn, parent *Server) (*connection, error) {
	// TODO: logging and error wrapping mechanisms.
	xprt := rpc.NewTransport(c, nil, nil)

	conn := &connection{
		c:      c,
		xprt:   xprt,
		parent: parent,
		authCh: make(chan error, 1),
	}

	if err := conn.startRPCServer(); err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *connection) checkAuth(ctx context.Context, m gregor1.Message) error {
	tok, res, _ := c.authInfo.get()
	if _, err := c.AuthenticateSessionToken(ctx, tok); err != nil {
		return err
	}
	if m.ToInBandMessage() != nil {
		if !bytes.Equal(m.ToInBandMessage().Metadata().UID().Bytes(), res.Uid) {
			return errors.New("mismatched UIDs")
		}
	}
	if m.ToOutOfBandMessage() != nil {
		if !bytes.Equal(m.ToOutOfBandMessage().UID().Bytes(), res.Uid) {
			return errors.New("mismatched UIDs")
		}
	}
	return nil
}

func (c *connection) AuthenticateSessionToken(ctx context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {
	log.Printf("Authenticate: %+v", tok)
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
		log.Printf("Revoke: %+v", sid)
		if c.authInfo.clear(sid) {
			break
		}
	}
	return c.parent.auth.RevokeSessionIDs(ctx, sessionIDs)
}

func (c *connection) ConsumeMessage(ctx context.Context, m gregor1.Message) error {
	log.Printf("ConsumeMessage: %+v", m)
	if err := c.checkAuth(ctx, m); err != nil {
		c.close()
		return err
	}

	return c.parent.consume(ctx, m)
}

func (c *connection) startRPCServer() error {
	// TODO: error wrapping mechanism
	c.server = rpc.NewServer(c.xprt, nil)

	prots := []rpc.Protocol{
		gregor1.AuthProtocol(c),
		gregor1.IncomingProtocol(c),
	}
	for _, prot := range prots {
		log.Printf("registering protocol %s", prot.Name)
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
