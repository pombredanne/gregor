package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	server "github.com/keybase/gregor/rpc/server"
	"github.com/keybase/gregor/stats"
	"github.com/keybase/gregor/storage"
	"github.com/keybase/gregor/test"
	"golang.org/x/net/context"
)

// Test with: MYSQL_DSN=gregor:@/gregor_test
// (if you use MYSQL_DSN and test everything in this
// package, you'll get an error because a flag test
// will be incorrect due to the env var)
func TestConsumeBroadcastFlow(t *testing.T) {
	if os.Getenv("TEST_WITH_SLEEP") != "" {
		withSleep = 3 * time.Millisecond
		t.Logf("using periodic artifical sleep of %s", withSleep)
		defer func() {
			withSleep = 0
			t.Logf("cleared artifical sleep")
		}()
	}
	db := test.AcquireTestDB(t)
	defer test.ReleaseTestDB()
	auth := newMockAuth()

	srvAddr, events, clock, cleanup := startTestGregord(t, db, auth)
	defer cleanup()

	t.Logf("gregord server started on %v", srvAddr)

	clients := make([]*client, 5)
	tmp := make([]byte, 16)
	if _, err := rand.Read(tmp); err != nil {
		t.Fatalf("error making new UID: %s", err)
	}
	tok, _, err := auth.newUser()
	if err != nil {
		t.Fatalf("error in newUser: %s", err)
	}
	for i := 0; i < len(clients); i++ {
		c, clean := startTestClient(t, tok, srvAddr)
		defer clean()
		clients[i] = c
	}

	for i := 0; i < len(clients); i++ {
		<-events.ConnCreated
	}

	// send a message to the server from clients[0]
	m0 := newUpdateMessage(t, clients[0].uid)
	if err := clients[0].IncomingClient().ConsumeMessage(context.TODO(), m0); err != nil {
		t.Fatal(err)
	}

	// wait for the broadcast
	<-events.BcastSent

	// check that all the clients received the message
	for i := 0; i < len(clients); i++ {
		if len(clients[i].broadcasts) != 1 {
			t.Errorf("clients[%d] broadcasts: %d, expected 1", i, len(clients[i].broadcasts))
		}
	}

	post, clean := startTestClient(t, tok, srvAddr)
	defer clean()

	<-events.ConnCreated

	if len(post.broadcasts) != 0 {
		t.Errorf("client connected after broadcast, has %d broadcasts, expected 0", len(post.broadcasts))
	}

	clock.Advance(time.Minute)
	t0 := clock.Now()
	clock.Advance(time.Hour)

	remindIn := 4 * time.Hour
	m1 := newUpdateMessageWithReminder(t, clients[0].uid, remindIn)

	if err := clients[0].IncomingClient().ConsumeMessage(context.TODO(), m1); err != nil {
		t.Fatal(err)
	}

	syncArg := gregor1.SyncArg{
		Uid:   clients[0].uid,
		Ctime: gregor1.ToTime(t0),
	}
	state, err := clients[0].IncomingClient().Sync(context.TODO(), syncArg)
	if err != nil {
		t.Fatal(err)
	}

	if len(state.Msgs) != 1 {
		t.Fatalf("Expected 1 msg; got %d\n", len(state.Msgs))
	}
	if !bytes.Equal(state.Msgs[0].ToStateUpdateMessage().Metadata().MsgID().Bytes(),
		m1.ToInBandMessage().Metadata().MsgID().Bytes()) {
		t.Fatal("Wrong msg ID returned in sync")
	}

	_, err = clients[0].RemindClient().GetReminders(context.TODO(), 0)
	if err == nil {
		t.Fatal("wanted an authentication error")
	} else if status, ok := err.(keybase1.Status); !ok {
		t.Fatal("wanted a keybase1.Status from the channel")
	} else if status.Code != int(keybase1.StatusCode_SCBadSession) {
		t.Fatal("wrong status code")
	}

	superToken, _, err := auth.newSuperUser()
	if err != nil {
		t.Fatal("error making new super token: %s\n", err)
	}
	superCli, superClean := startTestClient(t, superToken, srvAddr)
	defer superClean()

	reminders, err := superCli.RemindClient().GetReminders(context.TODO(), 0)
	if err != nil {
		t.Fatal("Error fetching reminders: %s", err)
	}

	findReminder := func(m gregor1.Message, reminders gregor1.ReminderSet) *gregor1.Reminder {
		for _, reminder := range reminders.Reminders_ {
			if bytes.Equal(m1.Ibm_.StateUpdate_.Md_.MsgID_, reminder.Item_.Md_.MsgID_) {
				return &reminder
			}
		}
		return nil
	}

	if findReminder(m1, reminders) != nil {
		t.Fatal("expected not to find our reminder")
	}

	clock.Advance(remindIn)

	reminders, err = superCli.RemindClient().GetReminders(context.TODO(), 0)
	if err != nil {
		t.Fatal("Error fetching reminders: %s", err)
	}
	rem := findReminder(m1, reminders)
	if rem == nil {
		t.Fatal("expected to find our reminder")
	}

	err = clients[0].RemindClient().DeleteReminders(context.TODO(), []gregor1.ReminderID{})
	if err == nil {
		t.Fatal("wanted an authentication error")
	} else if status, ok := err.(keybase1.Status); !ok {
		t.Fatal("wanted a keybase1.Status from the channel")
	} else if status.Code != int(keybase1.StatusCode_SCBadSession) {
		t.Fatal("wrong status code")
	}

	rid := gregor1.ReminderID{
		Uid_:   rem.Item_.Md_.Uid_,
		MsgID_: rem.Item_.Md_.MsgID_,
		Seqno_: rem.Seqno_,
	}
	err = superCli.RemindClient().DeleteReminders(context.TODO(), []gregor1.ReminderID{rid})
	if err != nil {
		t.Fatalf("Error deleting reminder: %s\n", err)
	}

	clock.Advance(24 * time.Hour)
	reminders, err = superCli.RemindClient().GetReminders(context.TODO(), 0)
	if err != nil {
		t.Fatal("Error fetching reminders: %s", err)
	}
	if findReminder(m1, reminders) != nil {
		t.Fatal("expected not to find our reminder (since we've deleted it)")
	}

}

var withSleep time.Duration

func maybeSleep() {
	if withSleep > 0 {
		time.Sleep(withSleep)
	}
}

func startTestGregord(t *testing.T, db *sql.DB, auth *mockAuth) (net.Addr, *test.Events, clockwork.FakeClock, func()) {
	opts := server.ServerOpts{
		BroadcastTimeout: 10 * time.Second,
		PublishChSize:    1000,
		NumPublishers:    10,
		PublishTimeout:   200 * time.Millisecond,
		StorageHandlers:  10,
		StorageQueueSize: 10000,
	}
	srv := server.NewServer(rpc.SimpleLogOutput{}, clockwork.NewFakeClock(), stats.DummyRegistry{}, opts)
	srv.SetAuthenticator(auth)
	e := test.NewEvents()
	srv.SetEventHandler(e)

	ms := newMainServer(&Options{BindAddress: "127.0.0.1:0"}, srv)
	ms.stopCh = make(chan struct{})
	cleanup := func() {
		db.Close()
		close(ms.stopCh)
	}
	sm, clock := storage.NewTestMySQLEngine(db, gregor1.ObjFactory{})
	srv.SetStorageStateMachine(sm)

	go func() {
		maybeSleep()
		if err := ms.listenAndServe(); err != nil {
			t.Fatal(err)
		}
	}()

	return <-ms.addr, e, clock, cleanup
}

type client struct {
	conn       net.Conn
	tr         rpc.Transporter
	cli        *rpc.Client
	broadcasts []gregor1.Message
	uid        gregor1.UID
	sid        gregor1.SessionID
}

func startTestClient(t *testing.T, tok gregor1.SessionToken, addr net.Addr) (*client, func()) {
	t.Logf("startTestClient dialing %v", addr)
	maybeSleep()
	c, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		t.Fatal(err)
	}
	maybeSleep()
	tr := rpc.NewTransport(c, nil, keybase1.WrapError)

	x := &client{
		conn: c,
		tr:   tr,
		cli:  rpc.NewClient(tr, keybase1.ErrorUnwrapper{}),
	}

	maybeSleep()
	srv := rpc.NewServer(tr, nil)
	if err := srv.Register(gregor1.OutgoingProtocol(x)); err != nil {
		t.Fatal(err)
	}

	res, err := x.AuthClient().AuthenticateSessionToken(context.TODO(), tok)
	if err != nil {
		t.Fatal(err)
	}
	x.uid = res.Uid
	x.sid = res.Sid

	cleanup := func() {
		x.conn.Close()
	}

	return x, cleanup
}

func (c *client) BroadcastMessage(ctx context.Context, m gregor1.Message) error {
	c.broadcasts = append(c.broadcasts, m)
	return nil
}

func (c *client) AuthClient() gregor1.AuthClient {
	return gregor1.AuthClient{Cli: c.cli}
}

func (c *client) IncomingClient() gregor1.IncomingClient {
	return gregor1.IncomingClient{Cli: c.cli}
}

func (c *client) RemindClient() gregor1.RemindClient {
	return gregor1.RemindClient{Cli: c.cli}
}

func newUpdateMessage(t *testing.T, uid gregor1.UID) gregor1.Message {
	msgid := make([]byte, 8)
	if _, err := rand.Read(msgid); err != nil {
		t.Fatal(err)
	}

	return gregor1.Message{
		Ibm_: &gregor1.InBandMessage{
			StateUpdate_: &gregor1.StateUpdateMessage{
				Md_: gregor1.Metadata{
					Uid_:   uid,
					MsgID_: msgid,
				},
				Creation_: &gregor1.Item{
					Category_: "testing",
					Body_:     msgid,
				},
			},
		},
	}
}

func newUpdateMessageWithReminder(t *testing.T, uid gregor1.UID, remindIn time.Duration) gregor1.Message {
	m := newUpdateMessage(t, uid)
	remindTimes := []gregor1.TimeOrOffset{{Offset_: gregor1.DurationMsec(remindIn / time.Millisecond)}}
	m.Ibm_.StateUpdate_.Creation_.RemindTimes_ = remindTimes
	return m
}
