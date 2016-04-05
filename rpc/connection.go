package rpc

import (
	"errors"
	"log"
	"net"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	protocol "github.com/keybase/gregor/protocol/go"
	"golang.org/x/net/context"
)

// ErrBadUID occurs when there is a bad UID on the auth channel.
var ErrBadUID = errors.New("bad UID on channel")

type connection struct {
	c          net.Conn
	xprt       rpc.Transporter
	uid        protocol.UID
	session    string
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

func (c *connection) Authenticate(ctx context.Context, tok string) error {
	log.Printf("Authenticate: %+v", tok)
	uid, sess, err := c.parent.auth.Authenticate(ctx, tok)
	if err == nil {
		c.uid = uid
		c.session = sess
		c.lastAuthed = c.parent.clock.Now()
	}
	c.authCh <- err
	return err
}

func (c *connection) ConsumeMessage(ctx context.Context, m protocol.Message) error {
	log.Printf("ConsumeMessage: %+v", m)
	return c.parent.consume(ctx, m)
}

func (c *connection) startRPCServer() <-chan error {
	// TODO: error wrapping mechanism
	srv := rpc.NewServer(c.xprt, nil)

	prots := []rpc.Protocol{
		protocol.AuthProtocol(c),
		protocol.IncomingProtocol(c),
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
