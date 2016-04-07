package rpc

import (
	"errors"
	"fmt"
	"io"
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

type mockConsumer struct {
	consumed []gregor.Message
}

func (m *mockConsumer) ConsumeMessage(ctx context.Context, msg gregor.Message) error {
	m.consumed = append(m.consumed, msg)
	return nil
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
	conn       net.Conn
	tr         rpc.Transporter
	cli        *rpc.Client
	broadcasts []protocol.Message
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
	if err := srv.Register(protocol.OutgoingProtocol(x)); err != nil {
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

func (c *client) AuthClient() protocol.AuthClient {
	return protocol.AuthClient{Cli: c.cli}
}

func (c *client) IncomingClient() protocol.IncomingClient {
	return protocol.IncomingClient{Cli: c.cli}
}

func (c *client) BroadcastMessage(ctx context.Context, m protocol.Message) error {
	if c.shutdown {
		return io.EOF
	}
	c.broadcasts = append(c.broadcasts, m)
	return nil
}

func TestAuthentication(t *testing.T) {
	_, l := startTestServer(nil)
	defer l.Close()

	c := newClient(l.Addr())
	defer c.Shutdown()

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
	defer c.Shutdown()

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

func newUpdateMessage(uid protocol.UID) protocol.Message {
	return protocol.Message{
		Ibm_: &protocol.InBandMessage{
			StateUpdate_: &protocol.StateUpdateMessage{
				Md_: protocol.Metadata{
					Uid_: uid,
				},
			},
		},
	}

}

func TestBroadcast(t *testing.T) {
	// create this channel so we can figure out when the async
	// broadcast is complete.  Destroy it when the test is finished.
	broadcastsSent = make(chan protocol.Message, 100)
	defer func() {
		broadcastsSent = nil
	}()

	s, l := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()
	if err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		t.Fatal(err)
	}

	// wait for the broadcast to be sent
	<-broadcastsSent

	if len(c.broadcasts) != 1 {
		t.Errorf("client broadcasts received: %d, expected 1", len(c.broadcasts))
	}
}

func TestConsume(t *testing.T) {
	mc := &mockConsumer{}
	s, l := startTestServer(mc)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()
	if err := c.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
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
	// create this channel so we can figure out when the async
	// broadcast is complete.  Destroy it when the test is finished.
	broadcastsSent = make(chan protocol.Message, 100)
	defer func() {
		broadcastsSent = nil
	}()

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

	c.Shutdown()

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn being closed:
		t.Logf("broadcast error: %s", err)
	}

	// wait for the message to be broadcast
	<-broadcastsSent

	// make sure it didn't receive the broadcast
	if len(c.broadcasts) != 0 {
		t.Errorf("c broadcasts: %d, expected 0", len(c.broadcasts))
	}

	// and the user server should be deleted:
	s.statsCh <- ch
	stats = <-ch
	if stats.UserServerCount != 0 {
		t.Errorf("user servers: %d, expected 0", stats.UserServerCount)
	}
}

func TestCloseConnect(t *testing.T) {
	// create this channel so we can figure out when the async
	// broadcast is complete.  Destroy it when the test is finished.
	broadcastsSent = make(chan protocol.Message, 100)
	defer func() {
		broadcastsSent = nil
	}()

	s, l := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c1 := newClient(l.Addr())
	if err := c1.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// close the first connection, start a new connection
	c1.Shutdown()
	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if err := c2.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn1 being closed:
		t.Logf("broadcast error: %s", err)
	}

	// wait for the broadcast to be sent
	<-broadcastsSent

	// the user server should still exist due to c2:
	ch := make(chan *Stats)
	s.statsCh <- ch
	stats := <-ch
	if stats.UserServerCount != 1 {
		t.Errorf("user servers: %d, expected 0", stats.UserServerCount)
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
	// create this channel so we can figure out when the async
	// broadcast is complete.  Destroy it when the test is finished.
	broadcastsSent = make(chan protocol.Message, 100)
	defer func() {
		broadcastsSent = nil
	}()

	s, l := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c1 := newClient(l.Addr())
	if err := c1.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// close the first connection
	c1.Shutdown()

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn1 being closed:
		t.Logf("broadcast error: %s", err)
	}

	// wait for the broadcast to be sent
	<-broadcastsSent

	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if err := c2.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

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
	s, l := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c1 := newClient(l.Addr())
	if err := c1.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// close the first connection
	c1.Shutdown()

	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if err := c2.AuthClient().Authenticate(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// the user server should exist due to c2:
	ch := make(chan *Stats)
	s.statsCh <- ch
	stats := <-ch
	if stats.UserServerCount != 1 {
		t.Errorf("user servers: %d, expected 1", stats.UserServerCount)
	}
}
