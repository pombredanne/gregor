package rpc

import (
	"errors"
	"io"
	"sync"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/stats"
	"golang.org/x/net/context"
)

type statsRes struct {
	totalConns int
}

type perUIDServer struct {
	sync.RWMutex

	uid   gregor1.UID
	conns map[*connection]bool
	alive bool

	sendBroadcastCh  chan broadcastArg
	shutdownCh       chan struct{}
	shouldShutdownCh chan struct{}
	events           EventHandler

	log   rpc.LogOutput
	stats stats.Registry

	broadcastTimeout time.Duration // in MS
}

func newPerUIDServer(uid gregor1.UID, parentShutdownCh chan struct{}, events EventHandler,
	log rpc.LogOutput, stats stats.Registry, bt time.Duration) *perUIDServer {

	s := &perUIDServer{
		uid:              uid,
		conns:            make(map[*connection]bool),
		sendBroadcastCh:  make(chan broadcastArg, 1000), // make this a huge queue for slow devices
		shutdownCh:       make(chan struct{}),
		shouldShutdownCh: make(chan struct{}, 10), // give this some space just in case
		events:           events,
		log:              log,
		stats:            stats.SetPrefix("user_server"),
		broadcastTimeout: bt,
		alive:            true,
	}

	s.stats.Count("new")

	// Spawn thread to look for our parent going down so we can clean up
	go func() {
		select {
		case <-s.shutdownCh:
			return
		case <-parentShutdownCh:
			s.Shutdown()
		}
	}()

	// Spawn thread for running broadcast requests
	go s.broadcastHandler()

	if s.events != nil {
		s.events.UIDServerCreated(s.uid)
	}

	return s
}

func (s *perUIDServer) GetStats() statsRes {
	s.RLock()
	defer s.RUnlock()
	return statsRes{totalConns: len(s.conns)}
}

// AddConnection will add a new connection to the list for this user server
func (s *perUIDServer) AddConnection(conn *connection) {
	s.Lock()
	defer s.Unlock()

	// Spawn a thread which will watch for this connection going down
	go s.waitOnConnection(conn)

	s.conns[conn] = true
	s.log.Info("uid server %s: added connection: count: %d", s.uid, len(s.conns))

	s.stats.Count("new conn")
	if s.events != nil {
		s.events.ConnectionCreated(s.uid)
	}
}

// waitOnConnection will wait for the connection to terminate, or for the
// UID server to be shutdown
func (s *perUIDServer) waitOnConnection(conn *connection) {
	select {
	case <-conn.serverDoneChan():
		s.checkConnections()
	case <-s.shutdownCh:
		return
	}
}

// checkConnections loops over all active connections and removes any that
// are currently dead
func (s *perUIDServer) checkConnections() {
	s.Lock()
	defer s.Unlock()

	s.log.Info("uid server %s: received connection closed message, checking all connections", s.uid)
	for conn, _ := range s.conns {
		if conn.xprt.IsConnected() {
			continue
		}
		s.log.Info("uid server %s: connection closed", s.uid)
		s.removeConnection(conn)
	}
}

// connections makes a copy of all the current connections and returns it
func (s *perUIDServer) connections() []*connection {
	s.RLock()
	defer s.RUnlock()
	var cl []*connection
	for k := range s.conns {
		cl = append(cl, k)
	}
	return cl
}

// Shutdown shuts down all goroutines spawned by the UID server and
// removes all connection
func (s *perUIDServer) Shutdown() {
	s.Lock()
	defer s.Unlock()

	if !s.alive {
		return
	}

	s.log.Info("shutting down uid server: uid: %s", s.uid)
	close(s.shutdownCh)
	s.removeAllConns()
	s.alive = false
}

func (s *perUIDServer) ConfirmShutdown() bool {
	s.Lock()
	defer s.Unlock()
	return len(s.conns) == 0
}

func (s *perUIDServer) ShouldShutdown() <-chan struct{} {
	return s.shouldShutdownCh
}

// removeAllConns removes all connections. This function must be called with
// the UID server write lock
func (s *perUIDServer) removeAllConns() {
	for conn, _ := range s.conns {
		s.removeConnection(conn)
	}
}

// removeConnection removes a connection for the server. This function must
// be called with the perUIDServer write lock
func (s *perUIDServer) removeConnection(conn *connection) {

	// This might be slow, so we should be careful about blocking on it while
	// holding a lock
	conn.close()

	s.stats.Count("remove conn")
	delete(s.conns, conn)
	if s.events != nil {
		s.events.ConnectionDestroyed(s.uid)
	}

	// We seem to be done here, let parent know we can be shutdown
	s.log.Info("uid server %s: removed connection: count: %d", s.uid, len(s.conns))
	if len(s.conns) == 0 {
		select {
		case s.shouldShutdownCh <- struct{}{}:
		default:
			s.log.Error("uid server: failed to send a shutdown message! orphaned uid server for %s", s.uid)
		}
	}
}

type broadcastArg struct {
	m       gregor1.Message
	resChan chan struct{}
}

// BroadcastMessage will dispatch a broadcast request to the broadcast handler
// goroutine. If it cannot queue it onto the channel it will fail so it does
// does not block. It will also return a channel that can be waited on for the
// broadcast to complete
func (s *perUIDServer) BroadcastMessage(m gregor1.Message) (error, <-chan struct{}) {
	resChan := make(chan struct{}, 1)
	select {
	case s.sendBroadcastCh <- broadcastArg{m: m, resChan: resChan}:
	default:
		return errors.New("broadcast queue full, rejected"), nil
	}
	return nil, resChan
}

func (s *perUIDServer) broadcastHandler() {
	s.log.Debug("uid server %s: starting broadcast handler", s.uid)
	for {
		select {
		case a := <-s.sendBroadcastCh:
			s.broadcast(a.m, a.resChan)
		case <-s.shutdownCh:
			return
		}
	}
}

// broadcast loops over all connections and sends the messages to them
func (s *perUIDServer) broadcast(m gregor1.Message, resChan chan struct{}) {
	var errCh = make(chan *connection)
	var wg sync.WaitGroup

	conns := s.connections()
	s.stats.Count("broadcast")
	s.stats.ValueInt("broadcast - conns", len(conns))

	for _, conn := range conns {
		if err := conn.checkMessageAuth(context.Background(), m); err != nil {
			s.log.Info("[connection auth failed]: %s", err)
			s.Lock()
			s.removeConnection(conn)
			s.Unlock()
			continue
		}
		oc := gregor1.OutgoingClient{Cli: rpc.NewClient(conn.xprt, keybase1.ErrorUnwrapper{})}
		// Two interesting things going on here:
		// 1.) All broadcast calls time out after 10 seconds in case
		//     of a super slow/buggy client never getting back to us
		// 2.) Spawn each call into it's own goroutine so a slow device doesn't
		//     prevent faster devices from getting these messages timely.
		wg.Add(1)
		go func(conn *connection) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), s.broadcastTimeout)
			defer cancel()
			if err := oc.BroadcastMessage(ctx, m); err != nil {
				s.log.Info("broadcast error: %s", err)

				// Push these onto a channel to clean up afterward
				if s.isConnDown(err) {
					errCh <- conn
				}
			}
		}(conn)
	}

	// Wait on all the calls
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Clean up all the connections we determined are dead
	for conn := range errCh {
		s.Lock()
		s.removeConnection(conn)
		s.Unlock()
	}

	if s.events != nil {
		s.events.BroadcastSent(m)
	}

	resChan <- struct{}{}
}

func (s *perUIDServer) isConnDown(err error) bool {
	if IsSocketClosedError(err) {
		return true
	}
	if err == io.EOF {
		return true
	}
	return false
}
