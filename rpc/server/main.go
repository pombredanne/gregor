package rpc

import (
	"crypto/tls"
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
	"github.com/keybase/gregor/srvup"
	"github.com/keybase/gregor/stats"
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

type storageReq interface{}

// Aliver is an interface to an object that can tell Server which
// other servers are alive.
type Aliver interface {
	Alive() ([]srvup.NodeDesc, error)
	MyID() srvup.NodeId
}

type Authenticator interface {
	AuthenticateSessionToken(context.Context, gregor1.SessionToken) (gregor1.AuthResult, error)
	GetSuperToken() gregor1.SessionToken
}

// Server is an RPC server that implements gregor.NetworkInterfaceOutgoing
// and gregor.NetworkInterface.
type Server struct {
	// At first we only allow one state machine, but there might be a need to
	// chain them together.
	storage gregor.StateMachine

	auth  Authenticator
	clock clockwork.Clock

	// key is the Hex-encoding of the binary UIDs
	users map[string](*perUIDServer)

	shutdownCh        chan struct{}
	publishDispatchCh chan gregor1.Message
	storageDispatchCh chan storageReq

	// used to determine which other servers are alive
	statusGroup Aliver
	groupOnce   sync.Once
	group       *aliveGroup

	// events allows checking various server event occurrences
	// (useful for testing, ok if left a default nil value)
	events EventHandler

	// Useful for testing. Insert arbitrary waits throughout the
	// code and wait for something bad to happen.
	useDeadlocker bool

	log rpc.LogOutput

	// Stats registry
	stats stats.Registry

	// The amount of time a perUIDServer should wait on a BroadcastMessage
	// response (in MS)
	broadcastTimeout time.Duration

	// The number of publishProcess goroutines to spawn.
	numPublishers int

	// Timeout for publish RPC calls
	publishTimeout time.Duration

	// TLS Config for reaching out to other similar peers
	tlsConfig *tls.Config

	// UID server lock
	sync.RWMutex
}

type ServerOpts struct {
	BroadcastTimeout time.Duration
	PublishChSize    int
	NumPublishers    int
	PublishTimeout   time.Duration
	StorageHandlers  int
	StorageQueueSize int
	TLSConfig        *tls.Config
}

// NewServer creates a Server.  You must call ListenLoop(...) for it to be functional.
func NewServer(log rpc.LogOutput, clock clockwork.Clock, stats stats.Registry, opts ServerOpts) *Server {
	s := &Server{
		clock:             clock,
		users:             make(map[string]*perUIDServer),
		publishDispatchCh: make(chan gregor1.Message, opts.PublishChSize),
		shutdownCh:        make(chan struct{}),
		storageDispatchCh: make(chan storageReq, opts.StorageQueueSize),
		log:               log,
		stats:             stats.SetPrefix("server"),
		broadcastTimeout:  opts.BroadcastTimeout,
		numPublishers:     opts.NumPublishers,
		publishTimeout:    opts.PublishTimeout,
		tlsConfig:         opts.TLSConfig,
	}

	// Spawn threads for handling storage requests
	for i := 0; i < opts.StorageHandlers; i++ {
		go s.storageDispatchHandler()
	}

	// Spawn threads for handling publishing consume message to peers
	s.publishSpawn()

	// Spawn background thread for stats
	go s.updateServerValueStatsLoop()

	return s
}

func (s *Server) SetAuthenticator(a Authenticator) {
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

func (s *Server) uidKey(u gregor1.UID) string {
	return hex.EncodeToString(u)
}

func (s *Server) updateServerValueStats() {
	s.RLock()
	defer s.RUnlock()

	s.stats.ValueInt("total user servers", len(s.users))
	totalConns := 0
	for _, usrv := range s.users {
		stats := usrv.GetStats()
		totalConns += stats.totalConns
	}
	s.stats.ValueInt("total connections", totalConns)
}

func (s *Server) updateServerValueStatsLoop() {
	for {
		select {
		case <-s.shutdownCh:
			return
		case <-time.After(time.Second):
			s.updateServerValueStats()
		}
	}
}

// getPerUIDServer gets a perUIDServer object representing all user connections.
// This function must be called with the UID server read lock
func (s *Server) getPerUIDServer(u gregor1.UID) (*perUIDServer, error) {
	s.deadlocker()
	k := s.uidKey(u)
	ret := s.users[k]
	if ret != nil {
		return ret, nil
	}
	return nil, nil
}

// setPerUIDServer adds a new UID server to the UID server map. This function
// must be called with the UID server write lock
func (s *Server) setPerUIDServer(u gregor1.UID, usrv *perUIDServer) error {
	s.deadlocker()
	k := s.uidKey(u)
	s.users[k] = usrv
	return nil
}

func (s *Server) UIDServerCount() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.users)
}

// createUIDServer starts up a new UID server, and adds it to the map of UID servers
// This function must be called with the write lock
func (s *Server) createUIDServer(uid gregor1.UID, parentShutdownCh chan struct{}, events EventHandler,
	log rpc.LogOutput, stats stats.Registry, bt time.Duration) (*perUIDServer, error) {

	usrv := newPerUIDServer(uid, s.shutdownCh, s.events, s.log, s.stats, s.broadcastTimeout)
	if err := s.setPerUIDServer(uid, usrv); err != nil {
		usrv.Shutdown(true)
		return nil, err
	}

	// Spawn thread to wait for a signal that it is time to attempt to reap
	// this new UID server
	go s.waitOnUIDServer(usrv)

	return usrv, nil
}

// waitOnUIDServer blocks until the UID server signals that is can be shutdown,
// or the Server shuts down
func (s *Server) waitOnUIDServer(usrv *perUIDServer) {
	for {
		select {
		case <-usrv.ShouldShutdown():
			s.Lock()
			if usrv.Shutdown(false) {
				delete(s.users, s.uidKey(usrv.uid))
				if s.events != nil {
					s.events.UIDServerDestroyed(usrv.uid)
				}
				s.Unlock()
				return
			}
			s.Unlock()
		case <-s.shutdownCh:
			return
		}
	}
}

// addUIDConnection adds a new connection to a UID server. Will also create
// one if no such server currently exists.
func (s *Server) addUIDConnection(c *connection) error {

	s.Lock()
	defer s.Unlock()

	_, res, _ := c.authInfo.get()

	usrv, err := s.getPerUIDServer(res.Uid)
	if err != nil {
		s.stats.Count("addUIDConnection - getPerUIDServer error")
		return err
	}

	s.stats.Count("addUIDConnection - newconn")
	if usrv == nil {
		usrv, err = s.createUIDServer(res.Uid, s.shutdownCh, s.events, s.log, s.stats, s.broadcastTimeout)
		if err != nil {
			return err
		}
	} else {
		s.stats.Count("addUIDConnection - uidserver hit")
	}

	s.deadlocker()
	usrv.AddConnection(c)
	s.deadlocker()
	return nil
}

// startSync gets called from the connection object that exists for each
// client of gregord for a sync call. It is on a different thread than
// the main Serve loop
func (s *Server) startSync(c context.Context, arg gregor1.SyncArg) (gregor1.SyncResult, error) {
	s.stats.Count("startSync")
	res := s.storageSync(s.storage, s.log, arg)
	return res.res, res.err
}

// stateByCategoryPrefix gets called from the connection object that exists
// for each client for a state get call.  It is on a different thread from
// the main loop, but its request has to be served from the main loop.
func (s *Server) stateByCategoryPrefix(_ context.Context, arg gregor1.StateByCategoryPrefixArg) (gregor1.State, error) {
	s.stats.Count("stateByCategoryPrefix")
	res := s.storageStateByCategoryPrefix(arg)
	return res.res, res.err
}

// getReminders gets called from the connection thread.
func (s *Server) getReminders(_ context.Context, maxReminders int) (gregor1.ReminderSet, error) {
	s.stats.Count("getReminders")
	res := s.storageGetReminders(maxReminders)
	return res.reminderSet, res.err
}

// deleteReminders is called from the connection thread.
func (s *Server) deleteReminders(_ context.Context, rids []gregor1.ReminderID) error {
	s.stats.Count("deleteReminders")
	return s.storageDeleteReminders(rids)
}

func (s *Server) doConsumeStats(m gregor1.Message) {
	// Stats
	s.stats.Count("runConsumeMessageMainSequence")
	if ibm := m.ToInBandMessage(); ibm != nil {
		s.stats.Count("runConsumeMessageMainSequence - ibm")
		if update := ibm.ToStateUpdateMessage(); update != nil {
			s.stats.Count("runConsumeMessageMainSequence - ibm - state update")
			if item := update.Creation(); item != nil {
				s.stats.Count("runConsumeMessageMainSequence - ibm - state update - creation")
				s.stats.Count("runConsumeMessageMainSequence - ibm - state update - creation - " + item.Category().String())
			}
			if dis := update.Dismissal(); dis != nil {
				s.stats.Count("runConsumeMessageMainSequence - ibm - state update - dismissal")
			}
		}
		if sync := ibm.ToStateSyncMessage(); sync != nil {
			s.stats.Count("runConsumeMessageMainSequence - ibm - state sync")
		}
	}
	if oobm := m.ToOutOfBandMessage(); oobm != nil {
		s.stats.Count("runConsumeMessageMainSequence - oobm")
		s.stats.Count("runConsumeMessageMainSequence - oobm - " + oobm.System().String())
	}
}

// runConsumeMessageMainSequence is the main entry point to the gregor flow described in the
// architecture documents. It receives a new message, writes it to the StateMachine,
// and broadcasts it to the other clients. Like startSync, this function is called
// from connection, and is on a different thread than Serve
func (s *Server) runConsumeMessageMainSequence(c context.Context, m gregor1.Message) error {

	s.doConsumeStats(m)
	res := s.storageConsumeMessage(m)
	if res.err != nil {
		s.stats.Count("runConsumeMessageMainSequence - storageConsumeMessage failure")
		return res.err
	}
	m.SetCTime(res.ctime)
	s.broadcastConsumeMessage(m)

	return s.publishConsumeMessage(m)
}

// consumePublish handles published messages from other gregord
// servers.  It doesn't store to db or publish.
func (s *Server) consumePublish(c context.Context, m gregor1.Message) error {
	s.broadcastConsumeMessage(m)
	return nil
}

func (s *Server) publishSpawn() {
	for i := 0; i < s.numPublishers; i++ {
		go s.publishProcess()
	}
}

func (s *Server) publishProcess() {
	for marg := range s.publishDispatchCh {
		if err := s.publish(marg); err != nil {
			s.log.Warning("publish error: %s", err)
		}
	}
}

func (s *Server) createAliveGroup() {
	s.log.Debug("creating new aliveGroup")

	// Get our ID, but statusGroup might be nil if we are in a test
	id := srvup.NodeId("")
	if s.statusGroup != nil {
		id = s.statusGroup.MyID()
	}

	s.group = newAliveGroup(s.statusGroup, s.auth, id, s.publishTimeout, s.clock, s.shutdownCh, s.log, s.tlsConfig)
}

func (s *Server) publish(m gregor1.Message) error {

	// Debugging
	ibm := m.ToInBandMessage()
	if ibm != nil {
		s.log.Debug("publish: in-band message: msgID: %s Ctime: %s",
			m.ToInBandMessage().Metadata().MsgID(), m.ToInBandMessage().Metadata().CTime())
	} else {
		s.log.Debug("publish: out-of-band message: uid: %s", m.ToOutOfBandMessage().UID())
	}

	s.groupOnce.Do(s.createAliveGroup)
	if err := s.group.Publish(context.Background(), m); err != nil {
		s.stats.Count("publish failure")
		return err
	}

	if s.events != nil {
		s.events.PublishSent(m)
	}

	return nil
}

// BroadcastMessage implements gregor.NetworkInterfaceOutgoing.
func (s *Server) BroadcastMessage(c context.Context, m gregor.Message) error {
	tm, ok := m.(gregor1.Message)
	if !ok {
		return ErrBadCast
	}
	s.broadcastConsumeMessage(tm)
	return nil
}

func (s *Server) broadcastConsumeMessage(m gregor1.Message) {

	s.RLock()
	srv, err := s.getPerUIDServer(gregor.UIDFromMessage(m).(gregor1.UID))
	s.RUnlock()
	if err != nil {
		s.log.Error("broadcastConsumeMessage: failed to get UID server: %s", err)
		return
	}

	// Nothing to do...
	if srv == nil {
		// even though nothing to do, create an event if
		// an event handler in place:
		if s.events != nil {
			s.log.Info("broadcastConsumeMessahe: no PerUIDServer for %s", gregor.UIDFromMessage(m))
			s.events.BroadcastSent(m)
		}
		return
	}

	s.stats.Count("broadcast")
	if err, _ := srv.BroadcastMessage(m); err != nil {
		s.log.Error("broadcastConsumeMessage: user %s failed BroadcastMessage: %s",
			gregor.UIDFromMessage(m), err)
	}

}

func (s *Server) publishConsumeMessage(m gregor1.Message) error {
	select {
	case s.publishDispatchCh <- m:
	default:
		s.log.Error("publishDispatchCh full: %d", len(s.publishDispatchCh))
		s.stats.Count("publish queue full")
		return ErrPublishChannelFull
	}
	return nil
}

type consumeMessageRet struct {
	err   error
	ctime time.Time
}

type consumeMessageReq struct {
	m      gregor.Message
	respCh chan<- consumeMessageRet
}

type syncReq struct {
	arg    gregor1.SyncArg
	respCh chan<- syncRet
}

type stateByCategoryPrefixRes struct {
	res gregor1.State
	err error
}

type stateByCategoryPrefixReq struct {
	arg    gregor1.StateByCategoryPrefixArg
	respCh chan<- stateByCategoryPrefixRes
}

type getRemindersRes struct {
	reminderSet gregor1.ReminderSet
	err         error
}

type getRemindersReq struct {
	maxReminders int
	respCh       chan<- getRemindersRes
}

type deleteRemindersReq struct {
	rids   []gregor1.ReminderID
	respCh chan<- error
}

// storageConsumeMessage schedules a Consume request on the dispatch handler
func (s *Server) storageConsumeMessage(m gregor.Message) consumeMessageRet {
	retCh := make(chan consumeMessageRet)
	req := consumeMessageReq{m: m, respCh: retCh}
	err := s.storageDispatch(req)
	if err != nil {
		return consumeMessageRet{
			err: err,
		}
	}
	return <-retCh
}

// storageSync schedules a Sync request on the dispatch handler
func (s *Server) storageSync(sm gregor.StateMachine, log rpc.LogOutput, arg gregor1.SyncArg) syncRet {
	retCh := make(chan syncRet)
	req := syncReq{arg: arg, respCh: retCh}
	err := s.storageDispatch(req)
	if err != nil {
		return syncRet{
			res: gregor1.SyncResult{Msgs: []gregor1.InBandMessage{}, Hash: []byte{}},
			err: err,
		}
	}
	return <-retCh
}

// storageStateByCategoryPrefix schedules a query of the state filtered by
// category prefix on the dispatch handler thread.
func (s *Server) storageStateByCategoryPrefix(arg gregor1.StateByCategoryPrefixArg) stateByCategoryPrefixRes {
	respCh := make(chan stateByCategoryPrefixRes)
	req := stateByCategoryPrefixReq{arg: arg, respCh: respCh}
	err := s.storageDispatch(req)
	if err != nil {
		var ret stateByCategoryPrefixRes
		ret.err = err
		return ret
	}
	return <-respCh

}

// storageGetReminders fetches a batch of reminders and locks them in the database.
func (s *Server) storageGetReminders(maxReminders int) getRemindersRes {
	respCh := make(chan getRemindersRes)
	req := getRemindersReq{respCh: respCh, maxReminders: maxReminders}
	err := s.storageDispatch(req)
	if err != nil {
		var ret getRemindersRes
		ret.err = err
		return ret
	}
	return <-respCh
}

func (s *Server) storageDeleteReminders(rids []gregor1.ReminderID) error {
	respCh := make(chan error)
	req := deleteRemindersReq{respCh: respCh, rids: rids}
	err := s.storageDispatch(req)
	if err != nil {
		return err
	}
	return <-respCh
}

// storageDispatch dispatches a new StorageMachine request. The storageDispatchCh channel
// is buffered with a lot of space, but in the case where the StorageMachine
// is totally locked, we will fill the queue and possibly reject the request.
func (s *Server) storageDispatch(req storageReq) error {
	select {
	case s.storageDispatchCh <- req:
	default:
		s.log.Error("XXX: dispatch queue full, rejecting!")
		s.stats.Count("storageDispatch queue full")
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
			ctime, err := s.storage.ConsumeMessage(req.m)
			req.respCh <- consumeMessageRet{ctime: ctime, err: err}

		case syncReq:
			res, err := grpc.Sync(s.storage, s.log, req.arg)
			req.respCh <- syncRet{res: res, err: err}

		case stateByCategoryPrefixReq:
			res, err := s.storage.StateByCategoryPrefix(req.arg.Uid, nil, nil, req.arg.CategoryPrefix)
			var resExportable gregor1.State
			var ok bool
			if resExportable, ok = res.(gregor1.State); !ok {
				err = errors.New("cannot export to gregor1.State as expected")
			}
			req.respCh <- stateByCategoryPrefixRes{res: resExportable, err: err}

		case getRemindersReq:
			reminderSet, err := s.storage.Reminders(req.maxReminders)
			var ok bool
			var reminderSetExportable gregor1.ReminderSet
			if reminderSetExportable, ok = reminderSet.(gregor1.ReminderSet); !ok {
				err = errors.New("cannot export to gregor1.ReminderSet as expected")
			}
			req.respCh <- getRemindersRes{reminderSet: reminderSetExportable, err: err}

		case deleteRemindersReq:
			var err error
			for _, rid := range req.rids {
				if err = s.storage.DeleteReminder(rid); err != nil {
					break
				}
			}
			req.respCh <- err

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
	s.addUIDConnection(nc)
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

// Shutdown tells the server to stop its Serve loop and storage dispatch
// handlers
func (s *Server) Shutdown() {
	close(s.shutdownCh)
	close(s.publishDispatchCh)
	close(s.storageDispatchCh)
}
