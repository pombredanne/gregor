package rpc

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

type mockAuth struct{}

const (
	goodToken = "goodtoken"
	badToken  = "badtoken"
)

var goodUID = gregor1.UID("gooduid")

func (m mockAuth) Authenticate(_ context.Context, tok string) (gregor1.UID, error) {
	if tok == goodToken {
		return goodUID, nil
	}

	return gregor1.UID{}, errors.New("invalid token")
}

type mockConsumer struct {
	consumed []gregor.Message
}

func (m *mockConsumer) ConsumeMessage(ctx context.Context, msg gregor.Message) error {
	m.consumed = append(m.consumed, msg)
	return nil
}

func startTestServer(x gregor.NetworkInterfaceIncoming) (*Server, net.Listener, *events) {
	ev := newEvents()
	s := NewServer()
	s.auth = mockAuth{}
	s.events = ev
	s.useDeadlocker = true
	l := newLocalListener()
	go s.Serve(x)
	go s.ListenLoop(l)
	return s, l, ev
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
	conn       net.Conn
	tr         rpc.Transporter
	cli        *rpc.Client
	broadcasts []gregor1.Message
	shutdown   bool
}

func newClient(addr net.Addr) *client {
	c, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		panic(fmt.Sprintf("failed to connect to test server: %s", err))
	}
	t := rpc.NewTransport(c, nil, nil)

	x := &client{
		conn: c,
		tr:   t,
		cli:  rpc.NewClient(t, nil),
	}

	srv := rpc.NewServer(t, nil)
	if err := srv.Register(gregor1.OutgoingProtocol(x)); err != nil {
		panic(err.Error())
	}

	return x
}

func (c *client) Shutdown() {
	c.conn.Close()
	// this is required as closing the connection only closes one direction
	// and there is a race in figuring out that the whole connection is closed.
	c.shutdown = true
}

func (c *client) AuthClient() gregor1.AuthClient {
	return gregor1.AuthClient{Cli: c.cli}
}

func (c *client) IncomingClient() gregor1.IncomingClient {
	return gregor1.IncomingClient{Cli: c.cli}
}

func (c *client) BroadcastMessage(ctx context.Context, m gregor1.Message) error {
	if c.shutdown {
		return io.EOF
	}
	c.broadcasts = append(c.broadcasts, m)
	return nil
}

func TestAuthentication(t *testing.T) {
	_, l, _ := startTestServer(nil)
	defer l.Close()

	c := newClient(l.Addr())
	defer c.Shutdown()

	if _, err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	if _, err := c.AuthClient().Authenticate(context.TODO(), badToken); err == nil {
		t.Fatal("badtoken passed authentication")
	}
}

func TestCreatePerUIDServer(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()

	if _, err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// wait for events from above before checking state
	<-ev.connCreated

	ch := make(chan *Stats)
	s.statsCh <- ch
	stats := <-ch
	if stats.UserServerCount != 1 {
		t.Errorf("user servers: %d, expected 1", stats.UserServerCount)
	}
}

func newOOBMessage(uid gregor1.UID, system gregor1.System, body gregor1.Body) gregor1.Message {
	return gregor1.Message{
		Oobm_: &gregor1.OutOfBandMessage{
			Uid_:    uid,
			System_: system,
			Body_:   body,
		},
	}
}

func newUpdateMessage(uid gregor1.UID) gregor1.Message {
	return gregor1.Message{
		Ibm_: &gregor1.InBandMessage{
			StateUpdate_: &gregor1.StateUpdateMessage{
				Md_: gregor1.Metadata{
					Uid_: uid,
				},
			},
		},
	}

}

func TestBroadcast(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()
	if _, err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.connCreated

	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		t.Fatal(err)
	}

	// wait for the broadcast to be sent
	<-ev.bcastSent

	if len(c.broadcasts) != 1 {
		t.Errorf("client broadcasts received: %d, expected 1", len(c.broadcasts))
	}
}

func TestConsume(t *testing.T) {
	mc := &mockConsumer{}
	s, l, _ := startTestServer(mc)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()
	if _, err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	if err := c.IncomingClient().ConsumeMessage(context.TODO(), newUpdateMessage(goodUID)); err != nil {
		t.Fatal(err)
	}

	if len(mc.consumed) != 1 {
		t.Errorf("consumer messages received: %d, expected 1", len(mc.consumed))
	}
}

func TestCloseOne(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())

	if _, err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.perUIDCreated

	ch := make(chan *Stats)
	s.statsCh <- ch
	stats := <-ch
	if stats.UserServerCount != 1 {
		t.Errorf("user servers: %d, expected 1", stats.UserServerCount)
	}

	c.Shutdown()

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn being closed:
		t.Logf("broadcast error: %s", err)
	}

	// make sure it didn't receive the broadcast
	if len(c.broadcasts) != 0 {
		t.Errorf("c broadcasts: %d, expected 0", len(c.broadcasts))
	}

	// wait for the perUID server to be shutdown
	<-ev.perUIDDestroyed

	// and the user server should be deleted:
	s.statsCh <- ch
	stats = <-ch
	if stats.UserServerCount != 0 {
		t.Errorf("user servers: %d, expected 0", stats.UserServerCount)
	}
}

func TestCloseConnect(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c1 := newClient(l.Addr())
	if _, err := c1.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// close the first connection, start a new connection
	c1.Shutdown()
	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if _, err := c2.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.connCreated
	<-ev.connDestroyed
	<-ev.connCreated

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn1 being closed:
		t.Logf("broadcast error: %s", err)
	}

	<-ev.bcastSent

	// the user server should still exist due to c2:
	ch := make(chan *Stats)
	s.statsCh <- ch
	stats := <-ch
	if stats.UserServerCount != 1 {
		t.Errorf("user servers: %d, expected 1", stats.UserServerCount)
	}

	// c1 shouldn't have received the broadcast:
	if len(c1.broadcasts) != 0 {
		t.Errorf("c1 broadcasts: %d, expected 0", len(c1.broadcasts))
	}

	// c2 should have received the broadcast:
	if len(c2.broadcasts) != 1 {
		t.Errorf("c2 broadcasts: %d, expected 1", len(c2.broadcasts))
	}
}

func TestCloseConnect2(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c1 := newClient(l.Addr())
	if _, err := c1.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.connCreated
	<-ev.perUIDCreated

	// close the first connection
	c1.Shutdown()

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn1 being closed:
		t.Logf("broadcast error: %s", err)
	}

	<-ev.perUIDDestroyed

	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if _, err := c2.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.connCreated

	// the user server should exist due to c2:
	ch := make(chan *Stats)
	s.statsCh <- ch
	stats := <-ch
	if stats.UserServerCount != 1 {
		t.Errorf("user servers: %d, expected 1", stats.UserServerCount)
	}

	// c1 shouldn't have received the broadcast:
	if len(c1.broadcasts) != 0 {
		t.Errorf("c1 broadcasts: %d, expected 0", len(c1.broadcasts))
	}

	// c2 shouldn't have received the broadcast:
	if len(c2.broadcasts) != 0 {
		t.Errorf("c2 broadcasts: %d, expected 0", len(c2.broadcasts))
	}
}

func TestCloseConnect3(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c1 := newClient(l.Addr())
	if _, err := c1.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// close the first connection
	c1.Shutdown()

	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if _, err := c2.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// wait for two connection created events
	<-ev.connCreated
	<-ev.connCreated

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn1 being closed:
		t.Logf("broadcast error: %s", err)
	}

	// wait for the broadcast to be sent
	<-ev.bcastSent

	// wait for c1 connection destroyed
	<-ev.connDestroyed

	// the user server should exist due to c2:
	ch := make(chan *Stats)
	s.statsCh <- ch
	stats := <-ch
	if stats.UserServerCount != 1 {
		t.Errorf("user servers: %d, expected 1", stats.UserServerCount)
	}

	// c1 shouldn't have received the broadcast:
	if len(c1.broadcasts) != 0 {
		t.Errorf("c1 broadcasts: %d, expected 0", len(c1.broadcasts))
	}

	// c2 should have received the broadcast:
	if len(c2.broadcasts) != 1 {
		t.Errorf("c2 broadcasts: %d, expected 1", len(c2.broadcasts))
	}
}
