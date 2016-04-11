package rpc

import (
	"errors"
	"log"
	"net"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

// ErrBadUID occurs when there is a bad UID on the auth channel.
var ErrBadUID = errors.New("bad UID on channel")

type connection struct {
	c          net.Conn
	xprt       rpc.Transporter
	uid        gregor1.UID
	tok        gregor1.SessionToken
	sid        gregor1.SessionID
	lastAuthed time.Time
	parent     *Server
	authCh     chan error
	errCh      <-chan error
}

func newConnection(c net.Conn, parent *Server) *connection {
	// TODO: logging and error wrapping mechanisms.
	xprt := rpc.NewTransport(c, nil, nil)

	conn := &connection{
		c:      c,
		xprt:   xprt,
		parent: parent,
		authCh: make(chan error, 100),
	}
	conn.errCh = conn.startRPCServer()
	return conn
}

func (c *connection) checkAuth(ctx context.Context) error {
	_, err := c.AuthenticateSessionToken(ctx, c.tok)
	return err
}

func (c *connection) AuthenticateSessionToken(ctx context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {
	log.Printf("Authenticate: %+v", tok)
	res, err := c.parent.auth.AuthenticateSessionToken(ctx, tok)
	if err == nil {
		c.tok = tok
		c.uid = res.Uid
		c.sid = res.Sid
		c.lastAuthed = c.parent.clock.Now()
	}
	c.authCh <- err
	return res, err
}

func (c *connection) RevokeSessionIDs(ctx context.Context, sessionIDs []gregor1.SessionID) error {
	for _, sid := range sessionIDs {
		log.Printf("Revoke: %+v", sid)
		if sid == c.sid {
			c.tok = ""
			c.uid = nil
			c.sid = ""
			c.lastAuthed = time.Time{}
			break
		}
	}
	return c.parent.auth.RevokeSessionIDs(ctx, sessionIDs)
}

func (c *connection) ConsumeMessage(ctx context.Context, m gregor1.Message) error {
	log.Printf("ConsumeMessage: %+v", m)
	if err := c.checkAuth(ctx); err != nil {
		c.close()
		return err
	}
	return c.parent.consume(ctx, m)
}

func (c *connection) startRPCServer() <-chan error {
	// TODO: error wrapping mechanism
	srv := rpc.NewServer(c.xprt, nil)

	prots := []rpc.Protocol{
		gregor1.AuthProtocol(c),
		gregor1.IncomingProtocol(c),
	}
	for _, prot := range prots {
		log.Printf("registering protocol %s", prot.Name)
		if err := srv.Register(prot); err != nil {
			errCh := make(chan error, 1)
			errCh <- err
			return errCh
		}
	}

	return srv.RunAsync()
}

func (c *connection) startAuthentication() error {
	err := <-c.authCh
	if err != nil {
		return err
	}
	if c.uid == nil {
		return ErrBadUID
	}
	return nil
}

func (c *connection) close() {
	close(c.authCh)
	c.c.Close()
}

var _ gregor1.AuthInterface = (*connection)(nil)
