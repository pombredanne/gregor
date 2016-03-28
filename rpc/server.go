package rpc

import (
	"encoding/hex"
	"errors"
	"log"
	"net"
	"sync"
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

func newPerUIDServer(uid protocol.UID) *PerUIDServer {
	return &PerUIDServer{
		uid: uid,
	}
}

type Stats struct {
	UserServerCount int
}

type StatsReporter func(s *Stats)

type Server struct {
	nii   gregor.NetworkInterfaceIncoming
	auth  Authenticator
	clock clockwork.Clock

	// key is the Hex-encoding of the binary UIDs
	users map[string](*PerUIDServer)

	shutdownCh      chan protocol.UID
	newConnectionCh chan *connection
	statsCh         chan StatsReporter
	closeCh         chan struct{}
	wg              sync.WaitGroup
}

func NewServer() *Server {
	s := &Server{
		clock:           clockwork.NewRealClock(),
		users:           make(map[string]*PerUIDServer),
		newConnectionCh: make(chan *connection),
		statsCh:         make(chan StatsReporter),
		closeCh:         make(chan struct{}),
	}

	s.wg.Add(1)
	go s.process()

	return s
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
	if err := srv.Run(true /* async */); err != nil {
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

func (s *Server) uidKey(u gregor.UID) (string, error) {
	tuid, ok := u.(protocol.UID)
	if !ok {
		return "", ErrBadCast
	}
	return hex.EncodeToString(tuid), nil
}

func (s *Server) getPerUIDServer(u gregor.UID) (*PerUIDServer, error) {
	k, err := s.uidKey(u)
	if err != nil {
		return nil, err
	}
	ret := s.users[k]
	if ret != nil {
		return ret, nil
	}
	return nil, nil
}

func (s *Server) setPerUIDServer(u gregor.UID, usrv *PerUIDServer) error {
	k, err := s.uidKey(u)
	if err != nil {
		return err
	}
	s.users[k] = usrv
	return nil
}

func (s *Server) addUIDConnection(c *connection) {
	usrv, err := s.getPerUIDServer(c.uid)
	if err != nil {
		log.Printf("getPerUIDServer error: %s", err)
		return
	}

	if usrv == nil {
		usrv = newPerUIDServer(c.uid)
		if err := s.setPerUIDServer(c.uid, usrv); err != nil {
			log.Printf("setPerUIDServer error: %s", err)
			return
		}
	}
}

func (s *Server) reportStats(f StatsReporter) {
	stats := &Stats{
		UserServerCount: len(s.users),
	}
	f(stats)
}

func (s *Server) process() {
	defer s.wg.Done()
	for {
		select {
		case c := <-s.newConnectionCh:
			s.addUIDConnection(c)
		case f := <-s.statsCh:
			s.reportStats(f)
		case <-s.closeCh:
			return
		}
	}
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
		c, err := l.Accept()
		if err != nil {
			if IsSocketClosedError(err) {
				err = nil
			}
			return err
		}

		go s.handleNewConnection(c)
	}
	return nil
}

func (s *Server) Shutdown() {
	close(s.closeCh)
	s.wg.Wait()
}

var _ gregor.NetworkInterfaceOutgoing = (*Server)(nil)
var _ gregor.NetworkInterface = (*Server)(nil)
