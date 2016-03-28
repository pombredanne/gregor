package rpc

import (
	"errors"
	"fmt"
	"net"
	"testing"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
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

func startTestServer(x gregor.NetworkInterfaceIncoming) (*Server, net.Listener) {
	s := NewServer()
	s.auth = mockAuth{}
	l := newLocalListener()
	go s.Serve(x)
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

type client struct {
	tr         rpc.Transporter
	cli        *rpc.Client
	broadcasts []protocol.Message
}

func newClient(addr net.Addr) *client {
	c, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		panic(fmt.Sprintf("failed to connect to test server: %s", err))
	}
	t := rpc.NewTransport(c, nil, nil)

	x := &client{
		tr:  t,
		cli: rpc.NewClient(t, nil),
	}

	srv := rpc.NewServer(t, nil)
	if err := srv.Register(protocol.OutgoingProtocol(x)); err != nil {
		panic(err.Error())
	}

	return x
}

func (c *client) AuthClient() protocol.AuthClient {
	return protocol.AuthClient{Cli: c.cli}
}

func (c *client) BroadcastMessage(ctx context.Context, m protocol.Message) error {
	c.broadcasts = append(c.broadcasts, m)
	return nil
}

func TestAuthentication(t *testing.T) {
	_, l := startTestServer(nil)
	defer l.Close()

	c := newClient(l.Addr())

	if err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	if err := c.AuthClient().Authenticate(context.TODO(), badToken); err == nil {
		t.Fatal("badtoken passed authentication")
	}
}

func TestCreatePerUIDServer(t *testing.T) {
	s, l := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())

	if err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	ch := make(chan *Stats)
	s.statsCh <- ch
	stats := <-ch
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
	s, l := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	if err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		t.Fatal(err)
	}

	if len(c.broadcasts) != 1 {
		t.Errorf("client broadcasts received: %d, expected 1", len(c.broadcasts))
	}
}
