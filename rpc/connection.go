package rpc

import (
	"log"
	"net"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	protocol "github.com/keybase/gregor/protocol/go"
	context "golang.org/x/net/context"
)

type connection struct {
	c          net.Conn
	xprt       rpc.Transporter
	uid        protocol.UID
	session    protocol.SessionID
	lastAuthed time.Time
	parent     *Server
	authCh     chan error
}

func newConnection(c net.Conn, parent *Server) (*connection, error) {
	// TODO: logging and error wrapping mechanisms.
	xprt := rpc.NewTransport(c, nil, nil)

	conn := &connection{
		c:      c,
		xprt:   xprt,
		parent: parent,
		authCh: make(chan error, 100),
	}

	if err := conn.startRPCServer(); err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *connection) Authenticate(ctx context.Context, tok protocol.AuthToken) error {
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

func (c *connection) startRPCServer() error {
	// TODO: error wrapping mechanism
	srv := rpc.NewServer(c.xprt, nil)

	prots := []rpc.Protocol{
		protocol.AuthProtocol(c),
		protocol.IncomingProtocol(c),
	}
	for _, prot := range prots {
		log.Printf("registering protocol %s", prot.Name)
		if err := srv.Register(prot); err != nil {
			return err
		}
	}

	return srv.Run(true /* async */)
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
