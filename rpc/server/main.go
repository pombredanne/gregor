package rpc

import (
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"
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
	c context.Context
	m gregor1.Message
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

type storageReq interface{}

// Aliver is an interface to an object that can tell Server which
// other servers are alive.
type Aliver interface {
	Alive() ([]string, error)
}

// Server is an RPC server that implements gregor.NetworkInterfaceOutgoing
// and gregor.NetworkInterface.
type Server struct {
	// At first we only allow one state machine, but there might be a need to
	// chain them together.
	storage gregor.StateMachine

	auth  gregor1.AuthInterface
	clock clockwork.Clock

	// key is the Hex-encoding of the binary UIDs
	users map[string](*perUIDServer)

	// last connection added per UID
	lastConns map[string]connectionID

	newConnectionCh   chan *connection
	statsCh           chan chan *Stats
	broadcastCh       chan messageArgs
	publishCh         chan messageArgs
	closeCh           chan struct{}
	confirmCh         chan confirmUIDShutdownArgs
	storageDispatchCh chan storageReq
	nextConnectionID  connectionID

	// used to determine which other servers are alive
	statusGroup Aliver
	addr        chan net.Addr
	authToken   gregor1.SessionToken
	groupOnce   sync.Once
	group       *aliveGroup

	// events allows checking various server event occurrences
	// (useful for testing, ok if left a default nil value)
	events EventHandler

	// Useful for testing. Insert arbitrary waits throughout the
	// code and wait for something bad to happen.
	useDeadlocker bool

	log rpc.LogOutput

	// The amount of time a perUIDServer should wait on a BroadcastMessage
	// response (in MS)
	broadcastTimeout time.Duration
}

// NewServer creates a Server.  You must call ListenLoop(...) and Serve(...)
// for it to be functional.
func NewServer(log rpc.LogOutput, broadcastTimeout time.Duration, storageHandlers int, storageQueueSize int) *Server {
	s := &Server{
		clock:             clockwork.NewRealClock(),
		users:             make(map[string]*perUIDServer),
		lastConns:         make(map[string]connectionID),
		newConnectionCh:   make(chan *connection),
		statsCh:           make(chan chan *Stats, 1),
		broadcastCh:       make(chan messageArgs),
		publishCh:         make(chan messageArgs, 1000),
		closeCh:           make(chan struct{}),
		confirmCh:         make(chan confirmUIDShutdownArgs),
		storageDispatchCh: make(chan storageReq, storageQueueSize),
		log:               log,
		broadcastTimeout:  broadcastTimeout,
		addr:              make(chan net.Addr, 1),
	}

	for i := 0; i < storageHandlers; i++ {
		go s.storageDispatchHandler()
	}

	s.publishSpawn()

	return s
}

func (s *Server) SetAuthenticator(a gregor1.AuthInterface) {
	s.auth = a
}

func (s *Server) SetEventHandler(e EventHandler) {
	s.events = e
}

func (s *Server) SetStorageStateMachine(sm gregor.StateMachine) {
	s.storage = sm
}

func (s *Server) SetStatusGroup(g Aliver) {
	s.statusGroup = g
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
		usrv = newPerUIDServer(res.Uid, s.confirmCh, s.closeCh, s.events, s.log, s.broadcastTimeout)
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
	s.broadcastCh <- messageArgs{c, tm}
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

	// If this is going to block, we just drop the broadcast. It means that
	// the user has a device that is not responding fast enough
	select {
	case srv.sendBroadcastCh <- messageArgs{c, m}:
	default:
		s.log.Error("user %s super slow receiving broadcasts, rejecting!",
			gregor.UIDFromMessage(m))
		return errors.New("broadcast queue full, rejected")
	}
	return nil
}

// startSync gets called from the connection object that exists for each
// client of gregord for a sync call. It is on a different thread than
// the main Serve loop
func (s *Server) startSync(c context.Context, arg gregor1.SyncArg) (gregor1.SyncResult, error) {
	res := s.storageSync(s.storage, s.log, arg)
	return res.res, res.err
}

// runConsumeMessageMainSequence is the main entry point to the gregor flow described in the
// architecture documents. It receives a new message, writes it to the StateMachine,
// and broadcasts it to the other clients. Like startSync, this function is called
// from connection, and is on a different thread than Serve
func (s *Server) runConsumeMessageMainSequence(c context.Context, m gregor1.Message) error {
	if err := s.storageConsumeMessage(m); err != nil {
		return err
	}
	s.broadcastConsumeMessage(c, m)

	select {
	case s.publishCh <- messageArgs{c: c, m: m}:
	default:
		s.log.Warning("publishCh full: %d", len(s.publishCh))
		return ErrPublishChannelFull
	}
	return nil
}

// consumePub handles published messages from other gregord
// servers.  It doesn't store to db or publish.
func (s *Server) consumePub(c context.Context, m gregor1.Message) error {
	s.broadcastCh <- messageArgs{c: c, m: m}
	return nil
}

func (s *Server) publishSpawn() {
	for i := 0; i < 10; i++ {
		go s.publishProcess()
	}
}

func (s *Server) publishProcess() {
	for {
		select {
		case marg := <-s.publishCh:
			if err := s.publish(marg); err != nil {
				s.log.Warning("publish error: %s", err)
			}
		case <-s.closeCh:
			return
		}
	}
}

func (s *Server) createAliveGroup() {
	s.log.Debug("creating new aliveGroup")
	a := <-s.addr
	s.group = newAliveGroup(s.statusGroup, a.String(), s.authToken, s.clock, s.closeCh, s.log)
}

func (s *Server) publish(marg messageArgs) error {
	s.log.Debug("publish: %+v", marg)
	s.groupOnce.Do(s.createAliveGroup)

	if err := s.group.Publish(marg.c, marg.m); err != nil {
		return err
	}

	if s.events != nil {
		s.events.PublishSent(marg.m)
	}

	return nil
}

func (s *Server) broadcastConsumeMessage(c context.Context, m gregor1.Message) {
	s.broadcastCh <- messageArgs{c, m}
}

// Serve starts the serve loop for Server.
func (s *Server) Serve() error {
	for {
		select {
		case c := <-s.newConnectionCh:
			s.logError("addUIDConnection", s.addUIDConnection(c))
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

type consumeMessageReq struct {
	m      gregor.Message
	respCh chan<- error
}

type syncReq struct {
	sm     gregor.StateMachine
	log    rpc.LogOutput
	arg    gregor1.SyncArg
	respCh chan<- syncRet
}

// storageConsumeMessage schedules a Consume request on the dispatch handler
func (s *Server) storageConsumeMessage(m gregor.Message) error {
	retCh := make(chan error)
	req := consumeMessageReq{m: m, respCh: retCh}
	err := s.storageDispatch(req)
	if err != nil {
		return err
	}
	return <-retCh
}

// storageSync schedules a Sync request on the dispatch handler
func (s *Server) storageSync(sm gregor.StateMachine, log rpc.LogOutput, arg gregor1.SyncArg) syncRet {
	retCh := make(chan syncRet)
	req := syncReq{sm: sm, log: log, arg: arg, respCh: retCh}
	err := s.storageDispatch(req)
	if err != nil {
		return syncRet{
			res: gregor1.SyncResult{Msgs: []gregor1.InBandMessage{}, Hash: []byte{}},
			err: err,
		}
	}
	return <-retCh
}

// storageDispatch dispatches a new StorageMachine request. The storageDispatchCh channel
// is buffered with a lot of space, but in the case where the StorageMachine
// is totally locked, we will fill the queue and possibly reject the request.
func (s *Server) storageDispatch(req storageReq) error {
	select {
	case s.storageDispatchCh <- req:
	default:
		s.log.Error("XXX: dispatch queue full, rejecting!")
		return errors.New("dispatch queue full, rejected")
	}
	return nil
}

// storageDispatchHandler handles pulling requests off the storageDispatchCh queue
// and routing them to the proper StorageMachine function. Many of these
// run at once in different threads. This also makes it such that
// any StorageMachine implementation must be thread safe.
func (s *Server) storageDispatchHandler() {
	for req := range s.storageDispatchCh {
		switch req := req.(type) {
		case consumeMessageReq:
			req.respCh <- s.storage.ConsumeMessage(req.m)
		case syncReq:
			res, err := grpc.Sync(req.sm, req.log, req.arg)
			req.respCh <- syncRet{res: res, err: err}
		default:
			s.log.Error("storageDispatchHandler(): unknown request type!")
		}
	}
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
	s.addr <- l.Addr()
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

// Shutdown tells the server to stop its Serve loop and storage dispatch
// handlers
func (s *Server) Shutdown() {
	close(s.closeCh)
	close(s.storageDispatchCh)
}
