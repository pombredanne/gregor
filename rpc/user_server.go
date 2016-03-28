package rpc

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	protocol "github.com/keybase/gregor/protocol/go"
)

type ConnectionID int

type PerUIDServer struct {
	uid   protocol.UID
	conns map[ConnectionID]*connection

	shutdownCh      <-chan protocol.UID
	newConnectionCh chan *connection
	sendBroadcastCh chan broadcastArgs
	wg              sync.WaitGroup
	nextConnID      ConnectionID
}

func newPerUIDServer(uid protocol.UID) *PerUIDServer {
	s := &PerUIDServer{
		uid:             uid,
		conns:           make(map[ConnectionID]*connection),
		newConnectionCh: make(chan *connection),
		sendBroadcastCh: make(chan broadcastArgs),
	}

	s.wg.Add(1)
	go s.process()

	return s
}

func (s *PerUIDServer) logError(prefix string, err error) {
	if err == nil {
		return
	}
	log.Printf("[uid %x] %s error: %s", s.uid, prefix, err)
}

func (s *PerUIDServer) process() {
	defer s.wg.Done()
	for {
		select {
		case c := <-s.newConnectionCh:
			s.logError("addConn", s.addConn(c))
		case a := <-s.sendBroadcastCh:
			s.broadcast(a)
		}
	}
}

func (s *PerUIDServer) addConn(c *connection) error {
	s.conns[s.nextConnID] = c
	s.nextConnID++
	return nil
}

func (s *PerUIDServer) broadcast(a broadcastArgs) {
	var errMsgs []string
	for id, conn := range s.conns {
		log.Printf("uid %x broadcast to %d", s.uid, id)
		oc := protocol.OutgoingClient{Cli: rpc.NewClient(conn.xprt, nil)}
		if err := oc.BroadcastMessage(a.c, a.m); err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("[connection %d]: %s", id, err))

			// TODO check for closed connection error
		}
	}

	if len(errMsgs) == 0 {
		a.retCh <- nil
	}
	a.retCh <- errors.New(strings.Join(errMsgs, ", "))
}

func (s *PerUIDServer) Shutdown() {
	s.wg.Wait()
}
