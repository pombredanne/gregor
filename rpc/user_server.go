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
}

func newPerUIDServer(uid protocol.UID, parentConfirmCh chan confirmUIDShutdownArgs, shutdownCh chan struct{}) *perUIDServer {
	s := &perUIDServer{
		uid:              uid,
		conns:            make(map[connectionID]*connection),
		newConnectionCh:  make(chan *connectionArgs, 1),
		sendBroadcastCh:  make(chan messageArgs, 1),
		tryShutdownCh:    make(chan bool, 1), // buffered so it can receive inside serve()
		closeListenCh:    make(chan error),
		parentConfirmCh:  parentConfirmCh,
		parentShutdownCh: shutdownCh,
		selfShutdownCh:   make(chan struct{}),
	}

	go s.serve()

	return s
}

func (s *perUIDServer) logError(prefix string, err error) {
	if err == nil {
		return
	}
	log.Printf("[uid %x] %s error: %s", s.uid, prefix, err)
}

func (s *perUIDServer) serve() {
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

	if len(s.conns) == 0 {
		s.tryShutdownCh <- true
	}

	// if the global broadcastsSent channel exists, put the message on it.
	// (this is primarily for testing purposes)
	if broadcastsSent != nil {
		broadcastsSent <- a.m
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
}

func (s *perUIDServer) removeAllConns() {
	for id, conn := range s.conns {
		s.removeConnection(conn, id)
	}
}
