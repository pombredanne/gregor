package rpc

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	protocol "github.com/keybase/gregor/protocol/go"
)

type connectionID int

type perUIDServer struct {
	uid   protocol.UID
	conns map[connectionID]*connection

	parentShutdownCh chan protocol.UID
	newConnectionCh  chan *connection
	sendBroadcastCh  chan messageArgs
	tryShutdownCh    chan bool
	nextConnID       connectionID
}

func newPerUIDServer(uid protocol.UID, parentShutdownCh chan protocol.UID) *perUIDServer {
	s := &perUIDServer{
		uid:              uid,
		conns:            make(map[connectionID]*connection),
		newConnectionCh:  make(chan *connection),
		sendBroadcastCh:  make(chan messageArgs),
		tryShutdownCh:    make(chan bool, 1), // buffered so it can receive inside process
		parentShutdownCh: parentShutdownCh,
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
		case c := <-s.newConnectionCh:
			s.logError("addConn", s.addConn(c))
		case a := <-s.sendBroadcastCh:
			s.broadcast(a)
		case <-s.tryShutdownCh:
			if s.tryShutdown() {
				return
			}
		}
	}
}

func (s *perUIDServer) addConn(c *connection) error {
	s.conns[s.nextConnID] = c
	s.nextConnID++
	return nil
}

func (s *perUIDServer) broadcast(a messageArgs) {
	var errMsgs []string
	for id, conn := range s.conns {
		log.Printf("uid %x broadcast to %d", s.uid, id)
		oc := protocol.OutgoingClient{Cli: rpc.NewClient(conn.xprt, nil)}
		if err := oc.BroadcastMessage(a.c, a.m); err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("[connection %d]: %s", id, err))

			if s.isConnDown(err) {
				conn.close()
				delete(s.conns, id)
			}
		}
	}

	if len(errMsgs) == 0 {
		a.retCh <- nil
	}
	a.retCh <- errors.New(strings.Join(errMsgs, ", "))

	if len(s.conns) == 0 {
		s.tryShutdownCh <- true
	}
}

// tryShutdown checks if it is ok to shutdown.  Returns true if it
// is ok.
func (s *perUIDServer) tryShutdown() bool {
	// make sure no connections have been added
	if len(s.conns) != 0 {
		log.Printf("tried shutdown, but %d conns for %x", len(s.conns), s.uid)
		return false
	}
	log.Printf("shutting down perUIDServer for %x", s.uid)
	// tell parent that the server for this uid is shutting down
	s.parentShutdownCh <- s.uid
	return true
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
