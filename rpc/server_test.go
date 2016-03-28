package rpc

import (
	"errors"
	"fmt"
	"net"
	"testing"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	protocol "github.com/keybase/gregor/protocol/go"
	"golang.org/x/net/context"
)

type mockAuth struct{}

const (
	goodToken = "goodtoken"
	badToken  = "badtoken"
)

var goodUID = protocol.UID("gooduid")

func (m mockAuth) Authenticate(_ context.Context, tok protocol.AuthToken) (protocol.UID, protocol.SessionID, error) {
	if tok == goodToken {
		return goodUID, protocol.SessionID(""), nil
	}

	return protocol.UID{}, protocol.SessionID(""), errors.New("invalid token")
}

func startTestServer() (*Server, net.Listener) {
	s := NewServer()
	s.auth = mockAuth{}
	l := newLocalListener()
	go s.ListenLoop(l)
	return s, l
}

func newLocalListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if l, err = net.Listen("tcp6", "[::1]:0"); err != nil {
			panic(fmt.Sprintf("failed to listen on a port: %v", err))
		}
	}
	return l
}

func newClient(addr net.Addr) *rpc.Client {
	c, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		panic(fmt.Sprintf("failed to connect to test server: %s", err))
	}
	t := rpc.NewTransport(c, nil, nil)
	return rpc.NewClient(t, nil)
}

func TestAuthentication(t *testing.T) {
	_, l := startTestServer()
	defer l.Close()

	ac := protocol.AuthClient{Cli: newClient(l.Addr())}

	if err := ac.Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	if err := ac.Authenticate(context.TODO(), badToken); err == nil {
		t.Fatal("badtoken passed authentication")
	}
}

func TestCreatePerUIDServer(t *testing.T) {
	s, l := startTestServer()
	defer l.Close()
	defer s.Shutdown()

	ac := protocol.AuthClient{Cli: newClient(l.Addr())}

	if err := ac.Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	c := make(chan *Stats)
	s.statsCh <- c
	stats := <-c
	if stats.UserServerCount != 1 {
		t.Errorf("user servers: %d, expected 1", stats.UserServerCount)
	}
}

func newOOBMessage(uid protocol.UID, system protocol.System, body protocol.Body) protocol.Message {
	return protocol.Message{
		Oobm_: &protocol.OutOfBandMessage{
			Uid_:    uid,
			System_: system,
			Body_:   body,
		},
	}
}

func TestBroadcast(t *testing.T) {
	s, l := startTestServer()
	defer l.Close()
	defer s.Shutdown()

	cli := newClient(l.Addr())
	ac := protocol.AuthClient{Cli: cli}
	if err := ac.Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		t.Fatal(err)
	}
}
