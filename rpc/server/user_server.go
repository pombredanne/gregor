package rpc

import (
	"io"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

type connectionArgs struct {
	c  *connection
	id connectionID
}

type perUIDServer struct {
	uid        gregor1.UID
	conns      map[connectionID]*connection
	lastConnID connectionID

	parentConfirmCh  chan confirmUIDShutdownArgs
	newConnectionCh  chan *connectionArgs
	sendBroadcastCh  chan messageArgs
	tryShutdownCh    chan bool
	closeListenCh    chan error
	parentShutdownCh chan struct{}
	selfShutdownCh   chan struct{}
	events           EventHandler

	log rpc.LogOutput
}

func newPerUIDServer(uid gregor1.UID, parentConfirmCh chan confirmUIDShutdownArgs, shutdownCh chan struct{}, events EventHandler, log rpc.LogOutput) *perUIDServer {
	s := &perUIDServer{
		uid:              uid,
		conns:            make(map[connectionID]*connection),
		newConnectionCh:  make(chan *connectionArgs, 1),
		sendBroadcastCh:  make(chan messageArgs, 1),
		tryShutdownCh:    make(chan bool, 1),    // buffered so it can receive inside serve()
		closeListenCh:    make(chan error, 100), // each connection uses the same closeListenCh, so buffer it more than 1
		parentConfirmCh:  parentConfirmCh,
		parentShutdownCh: shutdownCh,
		selfShutdownCh:   make(chan struct{}),
		events:           events,
		log:              log,
	}

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
	s.log.Info("[uid %x] %s error: %s", s.uid, prefix, err)
}

func (s *perUIDServer) serve() {
	defer func() {
		if s.events != nil {
			s.events.UIDServerDestroyed(s.uid)
		}
	}()
	for {
		select {
		case a := <-s.newConnectionCh:
			s.logError("addConn", s.addConn(a))
		case a := <-s.sendBroadcastCh:
			s.broadcast(a)
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
		}
	}
}

func (s *perUIDServer) addConn(a *connectionArgs) error {
	// Multiplex the connection close errors into closeListenCh.
	go func() {
		<-a.c.serverDoneChan()
		s.closeListenCh <- a.c.serverDoneErr()
	}()
	s.conns[a.id] = a.c
	s.lastConnID = a.id
	if s.events != nil {
		s.events.ConnectionCreated(s.uid)
	}

	return nil
}

func (s *perUIDServer) broadcast(a messageArgs) {
	for id, conn := range s.conns {
		s.log.Info("uid %x broadcast to %d", s.uid, id)
		if err := conn.checkMessageAuth(a.c, a.m); err != nil {
			s.log.Info("[connection %d]: %s", id, err)
			s.removeConnection(conn, id)
			continue
		}
		oc := gregor1.OutgoingClient{Cli: rpc.NewClient(conn.xprt, keybase1.ErrorUnwrapper{})}
		// Two interesting things going on here:
		// 1.) All broadcast calls time out after 10 seconds in case
		//     of a super slow/buggy  client never getting back to us
		// 2.) Spawn each call into it's own goroutine so a slow device doesn't
		//     prevent faster devices from getting these messages timely.
		go func() {
			ctx, cancel := context.WithTimeout(a.c, 10000*time.Millisecond)
			defer cancel()
			if err := oc.BroadcastMessage(ctx, a.m); err != nil {
				s.log.Info("[connection %d]: %s", id, err)

				if s.isConnDown(err) {
					s.removeConnection(conn, id)
				}
			}
		}()
	}

	if s.events != nil {
		s.events.BroadcastSent(a.m)
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
		s.log.Info("tried shutdown, but %d conns for %x", len(s.conns), s.uid)
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
	s.log.Info("uid server %x: received connection closed message, checking all connections", s.uid)
	for id, conn := range s.conns {
		if conn.xprt.IsConnected() {
			continue
		}
		s.log.Info("uid server %x: connection %d closed", s.uid, id)
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
	s.log.Info("uid server %x: removing connection %d", s.uid, id)
	conn.close()
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
