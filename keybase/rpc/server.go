package rpc

import (
	"errors"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	gregor "github.com/keybase/gregor"
	protocol "github.com/keybase/gregor/keybase/protocol/go"
	context "golang.org/x/net/context"
)

var ErrBadCast = errors.New("bad cast from gregor type to protocol type")

type ConnectionID int

type broadcastArgs struct {
	c     context.Context
	m     protocol.Message
	retCh chan<- error
}

type PerUIDServer struct {
	uid   protocol.UID
	conns map[ConnectionID]rpc.Transporter

	shutdownCh      <-chan protocol.UID
	newConnectionCh chan rpc.Transporter
	sendBroadcastCh chan broadcastArgs
}

type Server struct {
	nii   gregor.NetworkInterfaceIncoming
	users map[protocol.UID](*PerUIDServer)

	shutdownCh chan protocol.UID
}

func (s *Server) getPerUIDServer(u gregor.UID) (*PerUIDServer, error) {
	tuid, ok := u.(protocol.UID)
	if !ok {
		return nil, ErrBadCast
	}
	ret := s.users[tuid]
	if ret != nil {
		return ret, nil
	}
	return nil, nil
}

func (s *Server) BroadcastMessage(c context.Context, m gregor.Message) error {
	tm, ok := m.(protocol.Message)
	if !ok {
		return ErrBadCast
	}
	srv, err := s.getPerUIDServer(gregor.UIDFromMessage(m))
	if err != nil {
		return err
	}
	// Nothing to do...
	if srv == nil {
		return nil
	}
	retCh := make(chan error)
	args := broadcastArgs{c, tm, retCh}
	srv.sendBroadcastCh <- args
	err = <-retCh
	return err
}

func (s *Server) serve() error {
	// listen for incoming RPCs on all of the PerUIDServers, and when we get
	// one, forward down to s.nii
	return nil
}

func (s *Server) Serve(i gregor.NetworkInterfaceIncoming) error {
	s.nii = i
	return s.serve()
}

var _ gregor.NetworkInterfaceOutgoing = (*Server)(nil)
var _ gregor.NetworkInterface = (*Server)(nil)
