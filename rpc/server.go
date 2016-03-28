package rpc

import (
	"encoding/hex"
	"errors"
	"log"
	"net"

	"github.com/jonboulle/clockwork"
	gregor "github.com/keybase/gregor"
	protocol "github.com/keybase/gregor/protocol/go"
	context "golang.org/x/net/context"
)

var ErrBadCast = errors.New("bad cast from gregor type to protocol type")
var ErrBadUID = errors.New("bad UID on channel")

type messageArgs struct {
	c     context.Context
	m     protocol.Message
	retCh chan<- error
}

type Stats struct {
	UserServerCount int
}

type Server struct {
	nii   gregor.NetworkInterfaceIncoming
	auth  Authenticator
	clock clockwork.Clock

	// key is the Hex-encoding of the binary UIDs
	users map[string](*PerUIDServer)

	shutdownCh      chan protocol.UID
	newConnectionCh chan *connection
	statsCh         chan chan *Stats
	consumeCh       chan messageArgs
	closeCh         chan struct{}
}

func NewServer() *Server {
	s := &Server{
		clock:           clockwork.NewRealClock(),
		users:           make(map[string]*PerUIDServer),
		newConnectionCh: make(chan *connection),
		statsCh:         make(chan chan *Stats),
		consumeCh:       make(chan messageArgs),
		closeCh:         make(chan struct{}),
	}

	return s
}

func (s *Server) uidKey(u gregor.UID) (string, error) {
	tuid, ok := u.(protocol.UID)
	if !ok {
		log.Printf("can't cast %v (%T) to protocol.UID", u, u)
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

func (s *Server) addUIDConnection(c *connection) error {
	usrv, err := s.getPerUIDServer(c.uid)
	if err != nil {
		return err
	}

	if usrv == nil {
		usrv = newPerUIDServer(c.uid)
		if err := s.setPerUIDServer(c.uid, usrv); err != nil {
			return err
		}
	}

	usrv.newConnectionCh <- c
	return nil
}

func (s *Server) reportStats(c chan *Stats) {
	stats := &Stats{
		UserServerCount: len(s.users),
	}
	c <- stats
}

func (s *Server) logError(prefix string, err error) {
	if err == nil {
		return
	}
	log.Printf("%s error: %s", prefix, err)
}

func (s *Server) BroadcastMessage(c context.Context, m gregor.Message) error {
	tm, ok := m.(protocol.Message)
	if !ok {
		return ErrBadCast
	}

	// XXX fix this, race here:

	srv, err := s.getPerUIDServer(gregor.UIDFromMessage(m))
	if err != nil {
		return err
	}
	// Nothing to do...
	if srv == nil {
		return nil
	}
	retCh := make(chan error)
	args := messageArgs{c, tm, retCh}
	srv.sendBroadcastCh <- args
	return <-retCh
}

func (s *Server) consume(c context.Context, m protocol.Message) error {
	retCh := make(chan error)
	args := messageArgs{c, m, retCh}
	s.consumeCh <- args
	return <-retCh
}

func (s *Server) serve() error {
	for {
		select {
		case c := <-s.newConnectionCh:
			s.logError("addUIDConnection", s.addUIDConnection(c))
		case a := <-s.consumeCh:
			err := s.nii.ConsumeMessage(a.c, a.m)
			a.retCh <- err
		case c := <-s.statsCh:
			s.reportStats(c)
		case <-s.closeCh:
			return nil
		}
	}
}

func (s *Server) Serve(i gregor.NetworkInterfaceIncoming) error {
	s.nii = i
	return s.serve()
}

func (s *Server) handleNewConnection(c net.Conn) error {
	nc, err := newConnection(c, s)
	if err != nil {
		return err
	}
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
}

func (s *Server) Shutdown() {
	close(s.closeCh)
}

var _ gregor.NetworkInterfaceOutgoing = (*Server)(nil)
var _ gregor.NetworkInterface = (*Server)(nil)
