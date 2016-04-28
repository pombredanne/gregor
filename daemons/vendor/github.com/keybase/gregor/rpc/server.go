package rpc

import (
	"encoding/hex"
	"errors"
	"net"
	"time"

	"github.com/jonboulle/clockwork"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

// ErrBadCast occurs when there is a problem casting a type from
// gregor to protocol.
var ErrBadCast = errors.New("bad cast from gregor type to protocol type")

type connectionID int

func (s *Server) deadlocker() {
	if s.useDeadlocker {
		time.Sleep(3 * time.Millisecond)
	}
}

type syncRet struct {
	res gregor1.SyncResult
	err error
}

type syncArgs struct {
	c     context.Context
	a     gregor1.SyncArg
	retCh chan<- syncRet
}

type messageArgs struct {
	c     context.Context
	m     gregor1.Message
	retCh chan<- error
}

type confirmUIDShutdownArgs struct {
	uid        gregor1.UID
	lastConnID connectionID
}

// Stats contains information about the current state of the
// server.
type Stats struct {
	UserServerCount int
}

// Server is an RPC server that implements gregor.NetworkInterfaceOutgoing
// and gregor.NetworkInterface.
type Server struct {
	incoming gregor1.IncomingInterface
	auth     gregor1.AuthInterface
	clock    clockwork.Clock

	// key is the Hex-encoding of the binary UIDs
	users map[string](*perUIDServer)

	// last connection added per UID
	lastConns map[string]connectionID

	newConnectionCh  chan *connection
	statsCh          chan chan *Stats
	syncCh           chan syncArgs
	consumeCh        chan messageArgs
	broadcastCh      chan messageArgs
	closeCh          chan struct{}
	confirmCh        chan confirmUIDShutdownArgs
	nextConnectionID connectionID

	// events allows checking various server event occurrences
	// (useful for testing, ok if left a default nil value)
	events EventHandler

	// Useful for testing. Insert arbitrary waits throughout the
	// code and wait for something bad to happen.
	useDeadlocker bool

	log rpc.LogOutput
}

// NewServer creates a Server.  You must call ListenLoop(...) and Serve(...)
// for it to be functional.
func NewServer(log rpc.LogOutput) *Server {
	s := &Server{
		clock:           clockwork.NewRealClock(),
		users:           make(map[string]*perUIDServer),
		lastConns:       make(map[string]connectionID),
		newConnectionCh: make(chan *connection),
		statsCh:         make(chan chan *Stats, 1),
		syncCh:          make(chan syncArgs),
		consumeCh:       make(chan messageArgs),
		broadcastCh:     make(chan messageArgs),
		closeCh:         make(chan struct{}),
		confirmCh:       make(chan confirmUIDShutdownArgs),
		log:             log,
	}

	return s
}

func (s *Server) SetAuthenticator(a gregor1.AuthInterface) {
	s.auth = a
}

func (s *Server) SetEventHandler(e EventHandler) {
	s.events = e
}

func (s *Server) uidKey(u gregor.UID) (string, error) {
	tuid, ok := u.(gregor1.UID)
	if !ok {
		s.log.Info("can't cast %v (%T) to gregor1.UID", u, u)
		return "", ErrBadCast
	}
	return hex.EncodeToString(tuid), nil
}

func (s *Server) getPerUIDServer(u gregor.UID) (*perUIDServer, error) {
	s.deadlocker()
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

func (s *Server) setPerUIDServer(u gregor.UID, usrv *perUIDServer) error {
	s.deadlocker()
	k, err := s.uidKey(u)
	if err != nil {
		return err
	}
	s.users[k] = usrv
	return nil
}

func (s *Server) addUIDConnection(c *connection) error {
	_, res, _ := c.authInfo.get()
	usrv, err := s.getPerUIDServer(res.Uid)
	if err != nil {
		return err
	}

	if usrv == nil {
		usrv = newPerUIDServer(res.Uid, s.confirmCh, s.closeCh, s.events, s.log)
		if err := s.setPerUIDServer(res.Uid, usrv); err != nil {
			return err
		}
	}

	k, err := s.uidKey(res.Uid)
	if err != nil {
		return err
	}
	s.lastConns[k] = s.nextConnectionID
	s.deadlocker()
	usrv.newConnectionCh <- &connectionArgs{c: c, id: s.nextConnectionID}
	s.deadlocker()
	s.nextConnectionID++
	return nil
}

func (s *Server) confirmUIDShutdown(a confirmUIDShutdownArgs) {
	s.deadlocker()
	k, err := s.uidKey(a.uid)
	if err != nil {
		s.log.Info("confirmUIDShutdown, uidKey error: %s", err)
		return
	}
	serverLast, ok := s.lastConns[k]
	if !ok {
		s.log.Info("confirmUIDShutdown, bad state: no lastConns entry for %s", k)
		return
	}

	// it's ok to shutdown if the last connection that the server knows about
	// matches the last connection in the perUIDServer
	if serverLast == a.lastConnID {
		su := s.users[k]

		// remove the perUIDServer from users, lastConns
		delete(s.users, k)
		delete(s.lastConns, k)

		// close perUser's selfShutdown channel so it will
		// self-destruct
		if su != nil {
			s.deadlocker()
			close(su.selfShutdownCh)
		}

		return
	}
}

func (s *Server) reportStats(c chan *Stats) {
	s.log.Info("reportStats")
	stats := &Stats{
		UserServerCount: len(s.users),
	}
	c <- stats
}

func (s *Server) logError(prefix string, err error) {
	if err == nil {
		return
	}
	s.log.Info("%s error: %s", prefix, err)
}

// BroadcastMessage implements gregor.NetworkInterfaceOutgoing.
func (s *Server) BroadcastMessage(c context.Context, m gregor.Message) error {
	tm, ok := m.(gregor1.Message)
	if !ok {
		return ErrBadCast
	}
	s.broadcastCh <- messageArgs{c, tm, nil}
	return nil
}

func (s *Server) sendBroadcast(c context.Context, m gregor1.Message) error {
	srv, err := s.getPerUIDServer(gregor.UIDFromMessage(m))
	if err != nil {
		return err
	}
	// Nothing to do...
	if srv == nil {
		// even though nothing to do, create an event if
		// an event handler in place:
		if s.events != nil {
			s.log.Info("sendBroadcast: no PerUIDServer for %s", gregor.UIDFromMessage(m))
			s.events.BroadcastSent(m)
		}
		return nil
	}
	srv.sendBroadcastCh <- messageArgs{c, m, nil}
	return nil
}

func (s *Server) sync(c context.Context, arg gregor1.SyncArg) (gregor1.SyncResult, error) {
	retCh := make(chan syncRet)
	s.syncCh <- syncArgs{c, arg, retCh}
	ret := <-retCh
	return ret.res, ret.err
}

func (s *Server) consume(c context.Context, m gregor1.Message) error {
	retCh := make(chan error)
	args := messageArgs{c, m, retCh}
	s.consumeCh <- args
	if err := <-retCh; err != nil {
		return err
	}
	s.broadcastCh <- args
	return nil
}

func (s *Server) serve() error {
	for {
		select {
		case c := <-s.newConnectionCh:
			s.logError("addUIDConnection", s.addUIDConnection(c))
		case a := <-s.consumeCh:
			err := s.incoming.ConsumeMessage(a.c, a.m)
			a.retCh <- err
		case a := <-s.syncCh:
			var ret syncRet
			ret.res, ret.err = s.incoming.Sync(a.c, a.a)
			a.retCh <- ret
		case a := <-s.broadcastCh:
			s.sendBroadcast(a.c, a.m)
		case c := <-s.statsCh:
			s.reportStats(c)
		case a := <-s.confirmCh:
			s.confirmUIDShutdown(a)
		case <-s.closeCh:
			return nil
		}
	}
}

// Serve starts the serve loop for Server.
func (s *Server) Serve(i gregor1.IncomingInterface) error {
	s.incoming = i
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

// ListenLoop listens for new connections on net.Listener.
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

// Shutdown tells the server to stop its Serve loop.
func (s *Server) Shutdown() {
	close(s.closeCh)
}
