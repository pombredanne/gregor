package rpc

import (
	"sync"

	protocol "github.com/keybase/gregor/protocol/go"
)

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
	}

	s.wg.Add(1)
	go s.process()

	return s
}

func (s *PerUIDServer) process() {
	defer s.wg.Done()
	for {
		select {
		case c := <-s.newConnectionCh:
			s.addConn(c)
		}
	}
}

func (s *PerUIDServer) addConn(c *connection) {
	s.conns[s.nextConnID] = c
	s.nextConnID++
}

func (s *PerUIDServer) Shutdown() {
	s.wg.Wait()
}
