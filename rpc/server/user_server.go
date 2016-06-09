package rpc

import (
	"io"
	"sync"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/stats"
	"golang.org/x/net/context"
)

type connectionArgs struct {
	c  *connection
	id connectionID
}

type statsRes struct {
	totalConns int
}

type perUIDServer struct {
	uid        gregor1.UID
	conns      map[connectionID]*connection
	lastConnID connectionID

	parentConfirmCh  chan confirmUIDShutdownArgs
	newConnectionCh  chan *connectionArgs
	sendBroadcastCh  chan gregor1.Message
	tryShutdownCh    chan bool
	closeListenCh    chan error
	parentShutdownCh chan struct{}
	statsCh          chan chan statsRes
	selfShutdownCh   chan struct{}
	events           EventHandler

	log   rpc.LogOutput
	stats stats.Registry

	broadcastTimeout time.Duration // in MS
}

func newPerUIDServer(uid gregor1.UID, parentConfirmCh chan confirmUIDShutdownArgs,
	shutdownCh chan struct{}, events EventHandler, log rpc.LogOutput, stats stats.Registry,
	bt time.Duration) *perUIDServer {

	s := &perUIDServer{
		uid:              uid,
		conns:            make(map[connectionID]*connection),
		newConnectionCh:  make(chan *connectionArgs, 1),
		sendBroadcastCh:  make(chan gregor1.Message, 1000), // make this a huge queue for slow devices
		tryShutdownCh:    make(chan bool, 1),               // buffered so it can receive inside serve()
		closeListenCh:    make(chan error, 100),            // each connection uses the same closeListenCh, so buffer it more than 1
		parentConfirmCh:  parentConfirmCh,
		parentShutdownCh: shutdownCh,
		statsCh:          make(chan chan statsRes),
		selfShutdownCh:   make(chan struct{}),
		events:           events,
		log:              log,
		stats:            stats.SetPrefix("user_server"),
		broadcastTimeout: bt,
	}

	s.stats.Count("new")

	go s.serve()

	if s.events != nil {
		s.events.UIDServerCreated(s.uid)
	}

	return s
}

func (s *perUIDServer) logError(prefix string, err error) {
	if err == nil {
		return
	}
	s.log.Info("[uid %s] %s error: %s", s.uid, prefix, err)
}

func (s *perUIDServer) sendStats(resChan chan statsRes) {
	resChan <- statsRes{totalConns: len(s.conns)}
}

func (s *perUIDServer) serve() {
	defer func() {
		if s.events != nil {
			s.events.UIDServerDestroyed(s.uid)
		}
		s.stats.Count("destroyed")
	}()
	for {
		select {
		case a := <-s.newConnectionCh:
			s.logError("addConn", s.addConn(a))
		case m := <-s.sendBroadcastCh:
			s.broadcast(m)
		case <-s.closeListenCh:
			s.checkClosed()
			s.tryShutdown()
		case <-s.tryShutdownCh:
			s.tryShutdown()
		case <-s.parentShutdownCh:
			s.removeAllConns()
			return
		case <-s.selfShutdownCh:
			s.removeAllConns()
			return
		case r := <-s.statsCh:
			s.sendStats(r)
		}
	}
}

func (s *perUIDServer) addConn(a *connectionArgs) error {
	// Multiplex the connection close errors into closeListenCh.
	go func() {
		<-a.c.serverDoneChan()
		s.closeListenCh <- a.c.serverDoneErr()
	}()
	s.stats.Count("new conn")
	s.conns[a.id] = a.c
	s.lastConnID = a.id
	if s.events != nil {
		s.events.ConnectionCreated(s.uid)
	}

	return nil
}

func (s *perUIDServer) broadcast(m gregor1.Message) {
	var errCh = make(chan connectionArgs)
	var wg sync.WaitGroup

	s.stats.Count("broadcast")
	s.stats.ValueInt("broadcast - conns", len(s.conns))

	for id, conn := range s.conns {
		s.log.Info("uid %s broadcast to %d", s.uid, id)
		if err := conn.checkMessageAuth(context.Background(), m); err != nil {
			s.log.Info("[connection %d]: %s", id, err)
			s.removeConnection(conn, id)
			continue
		}
		oc := gregor1.OutgoingClient{Cli: rpc.NewClient(conn.xprt, keybase1.ErrorUnwrapper{})}
		// Two interesting things going on here:
		// 1.) All broadcast calls time out after 10 seconds in case
		//     of a super slow/buggy client never getting back to us
		// 2.) Spawn each call into it's own goroutine so a slow device doesn't
		//     prevent faster devices from getting these messages timely.
		wg.Add(1)
		go func(conn *connection, id connectionID) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), s.broadcastTimeout)
			defer cancel()
			if err := oc.BroadcastMessage(ctx, m); err != nil {
				s.log.Info("[connection %d]: %s", id, err)

				// Push these onto a channel to clean up afterward
				if s.isConnDown(err) {
					errCh <- connectionArgs{conn, id}
				}
			}
		}(conn, id)
	}

	// Wait on all the calls
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Clean up all the connections we determined are dead
	for ca := range errCh {
		s.removeConnection(ca.c, ca.id)
	}

	if s.events != nil {
		s.events.BroadcastSent(m)
	}

	if len(s.conns) == 0 {
		s.tryShutdownCh <- true
	}
}

// tryShutdown makes a request to the parent server for this
// server to be terminated.
func (s *perUIDServer) tryShutdown() {
	// make sure no connections have been added
	if len(s.conns) != 0 {
		s.log.Info("tried shutdown, but %d conns for %s", len(s.conns), s.uid)
		return
	}

	// confirm with the server that it is ok to shutdown
	args := confirmUIDShutdownArgs{
		uid:        s.uid,
		lastConnID: s.lastConnID,
	}
	s.parentConfirmCh <- args
}

func (s *perUIDServer) checkClosed() {
	s.log.Info("uid server %s: received connection closed message, checking all connections", s.uid)
	for id, conn := range s.conns {
		if conn.xprt.IsConnected() {
			continue
		}
		s.log.Info("uid server %s: connection %d closed", s.uid, id)
		s.removeConnection(conn, id)
	}
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

func (s *perUIDServer) removeConnection(conn *connection, id connectionID) {
	s.log.Info("uid server %s: removing connection %d", s.uid, id)
	conn.close()
	s.stats.Count("remove conn")
	delete(s.conns, id)
	if s.events != nil {
		s.events.ConnectionDestroyed(s.uid)
	}
}

func (s *perUIDServer) removeAllConns() {
	for id, conn := range s.conns {
		s.removeConnection(conn, id)
	}
}
