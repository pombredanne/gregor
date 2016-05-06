package rpc

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"github.com/keybase/gregor/test"
	gregor "github.com/keybase/gregor"
	"golang.org/x/net/context"
)

type mockAuth struct {
	sessions   map[gregor1.SessionToken]gregor1.AuthResult
	sessionIDs map[gregor1.SessionID]gregor1.SessionToken
}

func (m mockAuth) AuthenticateSessionToken(_ context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {
	if res, ok := m.sessions[tok]; ok {
		return res, nil
	}
	return gregor1.AuthResult{}, errors.New("invalid token")
}

func (m mockAuth) RevokeSessionIDs(_ context.Context, sessionIDs []gregor1.SessionID) (err error) {
	for _, sid := range sessionIDs {
		if _, ok := m.sessions[m.sessionIDs[sid]]; !ok {
			err = fmt.Errorf("invalid id: %s", sid)
		}
		delete(m.sessions, m.sessionIDs[sid])
		delete(m.sessionIDs, sid)
	}
	return
}

func newStorageStateMachine() gregor.StateMachine{
	var of gregor1.ObjFactory
	return storage.NewMemEngine(of, clockwork.NewRealClock())
}

const (
	goodToken  = gregor1.SessionToken("goodtoken")
	evilToken  = gregor1.SessionToken("eviltoken")
	superToken = gregor1.SessionToken("supertoken")
	badToken   = gregor1.SessionToken("badtoken")
	goodSID    = gregor1.SessionID("1")
	evilSID    = gregor1.SessionID("2")
	superSID   = gregor1.SessionID("3")
)

var (
	goodUID     = gregor1.UID("gooduid")
	evilUID     = gregor1.UID("eviluid")
	goodResult  = gregor1.AuthResult{Uid: goodUID, Sid: goodSID}
	evilResult  = gregor1.AuthResult{Uid: evilUID, Sid: evilSID}
	superResult = gregor1.AuthResult{Uid: superUID, Sid: superSID}
)

var mockAuthenticator gregor1.AuthInterface = mockAuth{
	sessions: map[gregor1.SessionToken]gregor1.AuthResult{
		goodToken:  goodResult,
		evilToken:  evilResult,
		superToken: superResult,
	},
	sessionIDs: map[gregor1.SessionID]gregor1.SessionToken{
		goodSID:  goodToken,
		evilSID:  evilToken,
		superSID: superToken,
	},
}

func startTestServer(ssm gregor.StateMachine) (*Server, net.Listener, *test.Events) {
	ev := test.NewEvents()
	s := NewServer(rpc.SimpleLogOutput{})
	s.events = ev
	s.useDeadlocker = true
	s.SetAuthenticator(mockAuthenticator)
	s.SetStorageStateMachine(ssm)
	l := newLocalListener()
	go s.Serve()
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
	sync.Mutex
}

func newClient(addr net.Addr) *client {
	c, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		panic(fmt.Sprintf("failed to connect to test server: %s", err))
	}
	t := rpc.NewTransport(c, nil, keybase1.WrapError)

	x := &client{
		conn: c,
		tr:   t,
		cli:  rpc.NewClient(t, keybase1.ErrorUnwrapper{}),
	}

	srv := rpc.NewServer(t, keybase1.WrapError)
	if err := srv.Register(gregor1.OutgoingProtocol(x)); err != nil {
		panic(err.Error())
	}

	return x
}

func (c *client) Shutdown() {
	c.conn.Close()
	// this is required as closing the connection only closes one direction
	// and there is a race in figuring out that the whole connection is closed.
	c.Lock()
	defer c.Unlock()
	c.shutdown = true
}

func (c *client) AuthClient() gregor1.AuthClient {
	return gregor1.AuthClient{Cli: c.cli}
}

func (c *client) IncomingClient() gregor1.IncomingClient {
	return gregor1.IncomingClient{Cli: c.cli}
}

func (c *client) BroadcastMessage(ctx context.Context, m gregor1.Message) error {
	c.Lock()
	defer c.Unlock()
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

	if _, err := c.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	if _, err := c.AuthClient().AuthenticateSessionToken(context.TODO(), badToken); err == nil {
		t.Fatal("badtoken passed authentication")
	}
}

func TestCreatePerUIDServer(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()

	if _, err := c.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// wait for events from above before checking state
	<-ev.ConnCreated

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
	if _, err := c.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.ConnCreated

	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		t.Fatal(err)
	}

	// wait for the broadcast to be sent
	<-ev.BcastSent

	if len(c.broadcasts) != 1 {
		t.Errorf("client broadcasts received: %d, expected 1", len(c.broadcasts))
	}
}

func TestConsume(t *testing.T) {
	incoming := newStorageStateMachine()
	s, l, _ := startTestServer(incoming)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()
	if _, err := c.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	if err := c.IncomingClient().ConsumeMessage(context.TODO(), newUpdateMessage(goodUID)); err != nil {
		t.Fatal(err)
	}
}

func TestImpersonation(t *testing.T) {
	incoming := newStorageStateMachine()
	s, l, _ := startTestServer(incoming)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()
	if _, err := c.AuthClient().AuthenticateSessionToken(context.TODO(), evilToken); err != nil {
		t.Fatal(err)
	}

	if err := c.IncomingClient().ConsumeMessage(context.TODO(), newUpdateMessage(goodUID)); err == nil {
		t.Fatal("no error when impersonating another user.")
	}
}

func TestSuperUser(t *testing.T) {
	incoming := newStorageStateMachine()
	s, l, _ := startTestServer(incoming)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())
	defer c.Shutdown()
	if _, err := c.AuthClient().AuthenticateSessionToken(context.TODO(), superToken); err != nil {
		t.Fatal(err)
	}

	if err := c.IncomingClient().ConsumeMessage(context.TODO(), newUpdateMessage(goodUID)); err != nil {
		t.Fatal(err)
	}
}

func TestCloseOne(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	c := newClient(l.Addr())

	if _, err := c.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.PerUIDCreated

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
	<-ev.PerUIDDestroyed

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
	if _, err := c1.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// close the first connection, start a new connection
	c1.Shutdown()
	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if _, err := c2.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.ConnCreated
	<-ev.ConnDestroyed
	<-ev.ConnCreated

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn1 being closed:
		t.Logf("broadcast error: %s", err)
	}

	<-ev.BcastSent

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
	if _, err := c1.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.ConnCreated
	<-ev.PerUIDCreated

	// close the first connection
	c1.Shutdown()

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn1 being closed:
		t.Logf("broadcast error: %s", err)
	}

	<-ev.PerUIDDestroyed

	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if _, err := c2.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.ConnCreated

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
	if _, err := c1.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// close the first connection
	c1.Shutdown()

	c2 := newClient(l.Addr())
	defer c2.Shutdown()
	if _, err := c2.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	// wait for two connection created events
	<-ev.ConnCreated
	<-ev.ConnCreated

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		// an error here is ok, as it could be about conn1 being closed:
		t.Logf("broadcast error: %s", err)
	}

	// wait for the broadcast to be sent
	<-ev.BcastSent

	// wait for c1 connection destroyed
	<-ev.ConnDestroyed

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
