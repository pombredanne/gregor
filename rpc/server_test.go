package rpc

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/jonboulle/clockwork"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	protocol "github.com/keybase/gregor/protocol/go"
	"golang.org/x/net/context"
)

type mockAuth struct{}

func (m mockAuth) Authenticate(_ context.Context, tok protocol.AuthToken) (protocol.UID, protocol.SessionID, error) {
	if tok == "goodtoken" {
		return protocol.UID{}, protocol.SessionID(""), nil
	}

	return protocol.UID{}, protocol.SessionID(""), errors.New("invalid token")
}

func startTestServer() (*Server, net.Listener) {
	s := &Server{
		auth:  mockAuth{},
		clock: clockwork.NewRealClock(),
	}
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

	if err := ac.Authenticate(context.TODO(), "goodtoken"); err != nil {
		t.Fatal(err)
	}

	if err := ac.Authenticate(context.TODO(), "badtoken"); err == nil {
		t.Fatal("badtoken passed authentication")
	}
}
