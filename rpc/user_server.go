package rpc

import (
	"io"
	"log"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	protocol "github.com/keybase/gregor/protocol/go"
)

type connectionArgs struct {
	c  *connection
	id connectionID
}

type perUIDServer struct {
	uid        protocol.UID
	conns      map[connectionID]*connection
	lastConnID connectionID

	parentConfirmCh  chan confirmUIDShutdownArgs
	newConnectionCh  chan *connectionArgs
	sendBroadcastCh  chan messageArgs
	tryShutdownCh    chan bool
	closeListenCh    chan error
	parentShutdownCh chan struct{}
	selfShutdownCh   chan struct{}
	events           eventHandler
}

func newPerUIDServer(uid protocol.UID, parentConfirmCh chan confirmUIDShutdownArgs, shutdownCh chan struct{}, events eventHandler) *perUIDServer {
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
	}

	go s.serve()

	if s.events != nil {
		s.events.uidServerCreated(s.uid)
	}

	return s
}

func (s *perUIDServer) logError(prefix string, err error) {
	if err == nil {
		return
	}
	log.Printf("[uid %x] %s error: %s", s.uid, prefix, err)
}

func (s *perUIDServer) serve() {
	defer func() {
		if s.events != nil {
			s.events.uidServerDestroyed(s.uid)
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

	a.c.xprt.AddCloseListener(s.closeListenCh)

	s.conns[a.id] = a.c
	s.lastConnID = a.id
	if s.events != nil {
		s.events.connectionCreated(s.uid)
	}

	// Well this is cute. We might have missed the s.closeListenCh above
	// because we started listening too late. So what we might do instead is
	// check that we're connected, and if not, to send an artificial message
	// on that channel.
	if !a.c.xprt.IsConnected() {
		s.closeListenCh <- nil
	}

	return nil
}

func (s *perUIDServer) broadcast(a messageArgs) {
	for id, conn := range s.conns {
		log.Printf("uid %x broadcast to %d", s.uid, id)
		oc := protocol.OutgoingClient{Cli: rpc.NewClient(conn.xprt, nil)}
		if err := oc.BroadcastMessage(a.c, a.m); err != nil {
			log.Printf("[connection %d]: %s", id, err)

			if s.isConnDown(err) {
				s.removeConnection(conn, id)
			}
		}
	}

	if s.events != nil {
		s.events.broadcastSent(a.m)
	}

	if len(s.conns) == 0 {
		s.tryShutdownCh <- true
	}
}

// tryShutdown checks if it is ok to shutdown.  Returns true if it
// is ok.
func (s *perUIDServer) tryShutdown() {
	// make sure no connections have been added
	if len(s.conns) != 0 {
		log.Printf("tried shutdown, but %d conns for %x", len(s.conns), s.uid)
	}

	// confirm with the server that it is ok to shutdown
	args := confirmUIDShutdownArgs{
		uid:        s.uid,
		lastConnID: s.lastConnID,
	}
	s.parentConfirmCh <- args
}

func (s *perUIDServer) checkClosed() {
	log.Printf("uid server %x: received connection closed message, checking all connections", s.uid)
	for id, conn := range s.conns {
		if conn.xprt.IsConnected() {
			continue
		}
		log.Printf("uid server %x: connection %d closed", s.uid, id)
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
	log.Printf("uid server %x: removing connection %d", s.uid, id)
	conn.close()
	delete(s.conns, id)
	if s.events != nil {
		s.events.connectionDestroyed(s.uid)
	}
}

func (s *perUIDServer) removeAllConns() {
	for id, conn := range s.conns {
		s.removeConnection(conn, id)
	}
}
