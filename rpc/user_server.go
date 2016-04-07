package rpc

import (
	"fmt"
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

	parentConfirmCh chan confirmUIDShutdownArgs
	newConnectionCh chan *connectionArgs
	sendBroadcastCh chan messageArgs
	tryShutdownCh   chan bool
	closeListenCh   chan error
	shutdownCh      chan struct{}
}

func newPerUIDServer(uid protocol.UID, parentConfirmCh chan confirmUIDShutdownArgs, shutdownCh chan struct{}) *perUIDServer {
	s := &perUIDServer{
		uid:             uid,
		conns:           make(map[connectionID]*connection),
		newConnectionCh: make(chan *connectionArgs, 1),
		sendBroadcastCh: make(chan messageArgs, 1),
		tryShutdownCh:   make(chan bool, 1), // buffered so it can receive inside serve()
		closeListenCh:   make(chan error),
		parentConfirmCh: parentConfirmCh,
		shutdownCh:      shutdownCh,
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
	fmt.Printf("serving it up!!!!\n")
	for {
		fmt.Printf("F1\n")
		select {
		case a := <-s.newConnectionCh:
			fmt.Printf("F2\n")
			s.logError("addConn", s.addConn(a))
		case a := <-s.sendBroadcastCh:
			fmt.Printf("J0\n")
			s.broadcast(a)
		case <-s.closeListenCh:
			fmt.Printf("F3\n")
			s.checkClosed()
			s.tryShutdown()
			fmt.Printf("F5\n")
		case <-s.tryShutdownCh:
			fmt.Printf("F6\n")
			s.tryShutdown()
			fmt.Printf("F7\n")
		case <-s.shutdownCh:
			fmt.Printf("F8\n")
			s.removeAllConns()
			fmt.Printf("F9\n")
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
	var errMsgs []string
	fmt.Printf("J1\n")
	for id, conn := range s.conns {
		fmt.Printf("J2\n")
		log.Printf("uid %x broadcast to %d", s.uid, id)
		oc := protocol.OutgoingClient{Cli: rpc.NewClient(conn.xprt, nil)}
		if err := oc.BroadcastMessage(a.c, a.m); err != nil {
			// Just log error messages...
			errMsgs = append(errMsgs, fmt.Sprintf("[connection %d]: %s", id, err))

			if s.isConnDown(err) {
				s.removeConnection(conn, id)
			}
		}
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
	fmt.Printf("T1\n")
	s.parentConfirmCh <- args
	fmt.Printf("T2\n")
	fmt.Printf("T3\n")
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
