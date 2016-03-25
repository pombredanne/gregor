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
			panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
		}
	}
	return l
}

func newClient(addr net.Addr) (*rpc.Client, error) {
	c, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		return nil, err
	}
	t := rpc.NewTransport(c, nil, nil)
	return rpc.NewClient(t, nil), nil
}

func TestAuthentication(t *testing.T) {
	_, l := startTestServer()
	defer l.Close()

	c, err := newClient(l.Addr())
	if err != nil {
		t.Fatal(err)
	}

	ac := protocol.AuthClient{Cli: c}

	err = ac.Authenticate(context.TODO(), "goodtoken")
	if err != nil {
		t.Fatal(err)
	}

	err = ac.Authenticate(context.TODO(), "badtoken")
	if err == nil {
		t.Fatal("badtoken passed authentication")
	}
}
