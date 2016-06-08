package rpc

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	gregor "github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"github.com/keybase/gregor/test"
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

func (m mockAuth) CreateGregorSuperUserSessionToken(_ context.Context) (gregor1.SessionToken, error) {
	return superToken, nil
}

func (m mockAuth) GetSuperToken() gregor1.SessionToken {
	return superToken
}

func newStorageStateMachine() gregor.StateMachine {
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

var mockAuthenticator Authenticator = mockAuth{
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
	opts := ServerOpts{
		BroadcastTimeout: 10 * time.Second,
		PublishChSize:    1000,
		NumPublishers:    10,
		PublishTimeout:   200 * time.Millisecond,
		StorageHandlers:  10,
		StorageQueueSize: 10000,
	}
	s := NewServer(rpc.SimpleLogOutput{}, opts)
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

func newClientOneway(addr net.Addr) *client {
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
	return x
}

func newClient(addr net.Addr) *client {
	x := newClientOneway(addr)
	srv := rpc.NewServer(x.tr, keybase1.WrapError)
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

type slowClient struct {
	cli          *client
	responseTime int
}

func (c *slowClient) AuthClient() gregor1.AuthClient {
	return gregor1.AuthClient{Cli: c.cli.cli}
}

func newSlowClient(addr net.Addr, responseTime int) *slowClient {
	cli := newClientOneway(addr)
	x := &slowClient{
		cli:          cli,
		responseTime: responseTime,
	}
	srv := rpc.NewServer(x.cli.tr, keybase1.WrapError)
	if err := srv.Register(gregor1.OutgoingProtocol(x)); err != nil {
		panic(err.Error())
	}
	return x
}

func (c *slowClient) BroadcastMessage(ctx context.Context, m gregor1.Message) error {
	time.Sleep(time.Duration(c.responseTime) * time.Millisecond)
	return c.cli.BroadcastMessage(ctx, m)
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
	id := rand.Int63()
	msgid := strconv.FormatInt(id, 16)
	return gregor1.Message{
		Ibm_: &gregor1.InBandMessage{
			StateUpdate_: &gregor1.StateUpdateMessage{
				Md_: gregor1.Metadata{
					Uid_:   uid,
					MsgID_: []byte(msgid),
				},
				Creation_: &gregor1.Item{
					Category_: "test",
					Body_:     []byte(""),
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

// Test to make sure gregord functions for other users if a user is slow
func TestBlockedUser(t *testing.T) {
	incoming := newStorageStateMachine()
	s, l, ev := startTestServer(incoming)
	defer l.Close()
	defer s.Shutdown()

	// Start up a slow client to annoy the server
	slowCli := newSlowClient(l.Addr(), 20000)
	if _, err := slowCli.AuthClient().AuthenticateSessionToken(context.TODO(), evilToken); err != nil {
		t.Fatal(err)
	}

	cli := newClient(l.Addr())
	if _, err := cli.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.ConnCreated
	<-ev.ConnCreated

	// should broadcast right away
	if err := cli.IncomingClient().ConsumeMessage(context.TODO(),
		newUpdateMessage(goodUID)); err != nil {
		t.Fatal(err)
	}

	// Spawn a bunch of these on the slow client to try and overwhelm and
	// saturate the broadcast channel in the perUIDServer
	for i := 0; i < 20; i++ {
		if err := slowCli.cli.IncomingClient().ConsumeMessage(context.TODO(),
			newUpdateMessage(evilUID)); err != nil {
			t.Fatal(err)
		}
	}

	// In a broken state, this user could get blocked
	if err := cli.IncomingClient().ConsumeMessage(context.TODO(),
		newUpdateMessage(goodUID)); err != nil {
		t.Fatal(err)
	}

	// Give the whole thing 2 seconds to work or we consider it a failure
	success := 0
	for i := 0; i < 2; i++ {
		select {
		case <-ev.BcastSent:
			success++
		case <-time.After(2 * time.Second):
		}
	}

	if success < 2 {
		t.Errorf("broadcasts sent: %d, expected 2", success)
	}

}

// Test that a single user device won't completely block the perUIDServer
// if it is slow
func TestBroadcastTimeout(t *testing.T) {
	s, l, ev := startTestServer(nil)
	defer l.Close()
	defer s.Shutdown()

	s.broadcastTimeout = 500
	slowCli := newSlowClient(l.Addr(), 19000)
	cli := newClient(l.Addr())

	if _, err := slowCli.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	if _, err := cli.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}

	<-ev.ConnCreated
	<-ev.ConnCreated

	// broadcast a message to goodUID
	m := newOOBMessage(goodUID, "sys", nil)
	if err := s.BroadcastMessage(context.TODO(), m); err != nil {
		t.Logf("broadcast error: %s", err)
	}

	success := 0
	select {
	case <-ev.BcastSent:
		success++
	case <-time.After(2 * time.Second):
	}

	if success == 0 {
		t.Errorf("broadcasts sent: %d, expected 1", success)
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
