package rpc

import (
	"encoding/hex"
	"errors"
	"net"
	"time"

	"github.com/jonboulle/clockwork"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	gregor "github.com/keybase/gregor"
	protocol "github.com/keybase/gregor/protocol/go"
	context "golang.org/x/net/context"
)

var ErrBadCast = errors.New("bad cast from gregor type to protocol type")
var ErrBadUID = errors.New("bad UID on channel")

type ConnectionID int

type broadcastArgs struct {
	c     context.Context
	m     protocol.Message
	retCh chan<- error
}

type connection struct {
	c          net.Conn
	xprt       rpc.Transporter
	uid        protocol.UID
	session    protocol.SessionID
	lastAuthed time.Time
	parent     *Server
	authCh     chan error
}

type PerUIDServer struct {
	uid   protocol.UID
	conns map[ConnectionID](*connection)

	shutdownCh      <-chan protocol.UID
	newConnectionCh chan rpc.Transporter
	sendBroadcastCh chan broadcastArgs
}

type Server struct {
	nii   gregor.NetworkInterfaceIncoming
	auth  Authenticator
	clock clockwork.Clock

	// key is the Hex-encoding of the binary UIDs
	users map[string](*PerUIDServer)

	shutdownCh      chan protocol.UID
	newConnectionCh chan *connection
}

func (s *Server) newConnection(c net.Conn) *connection {
	// TODO: logging and error wrapping mechanisms.
	xprt := rpc.NewTransport(c, nil, nil)
	return &connection{c: c, xprt: xprt, parent: s}
}

func (c *connection) Authenticate(ctx context.Context, tok protocol.AuthToken) error {
	uid, sess, err := c.parent.auth.Authenticate(ctx, tok)
	if err == nil {
		c.uid = uid
		c.session = sess
		c.lastAuthed = c.parent.clock.Now()
	}
	c.authCh <- err
	return err
}

func (c *connection) startAuthentication() error {
	// TODO: error wrapping mechanism
	srv := rpc.NewServer(c.xprt, nil)
	prot := protocol.AuthProtocol(c)
	c.authCh = make(chan error, 100)
	if err := srv.Register(prot); err != nil {
		return err
	}
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

func (s *Server) getPerUIDServer(u gregor.UID) (*PerUIDServer, error) {
	tuid, ok := u.(protocol.UID)
	if !ok {
		return nil, ErrBadCast
	}
	k := hex.EncodeToString(tuid)
	ret := s.users[k]
	if ret != nil {
		return ret, nil
	}
	return nil, nil
}

func (s *Server) BroadcastMessage(c context.Context, m gregor.Message) error {
	tm, ok := m.(protocol.Message)
	if !ok {
		return ErrBadCast
	}
	srv, err := s.getPerUIDServer(gregor.UIDFromMessage(m))
	if err != nil {
		return err
	}
	// Nothing to do...
	if srv == nil {
		return nil
	}
	retCh := make(chan error)
	args := broadcastArgs{c, tm, retCh}
	srv.sendBroadcastCh <- args
	err = <-retCh
	return err
}

func (s *Server) serve() error {
	// listen for incoming RPCs on all of the PerUIDServers, and when we get
	// one, forward down to s.nii
	return nil
}

func (s *Server) Serve(i gregor.NetworkInterfaceIncoming) error {
	s.nii = i
	return s.serve()
}

func (s *Server) handleNewConnection(c net.Conn) error {
	nc := s.newConnection(c)
	if err := nc.startAuthentication(); err != nil {
		nc.close()
		return err
	}
	s.newConnectionCh <- nc
	return nil
}

func (s *Server) ListenLoop(l net.Listener) error {
	for {
		if c, err := l.Accept(); err != nil {
			if IsSocketClosedError(err) {
				err = nil
			}
			return err
		} else {
			go s.handleNewConnection(c)
		}
	}
	return nil
}

var _ gregor.NetworkInterfaceOutgoing = (*Server)(nil)
var _ gregor.NetworkInterface = (*Server)(nil)
