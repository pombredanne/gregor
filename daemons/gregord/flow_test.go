package main

import (
	"crypto/rand"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"
	"github.com/keybase/gregor/storage"
	"github.com/keybase/gregor/test"
	"golang.org/x/net/context"
)

// Test with: TEST_MYSQL_DSN=gregor:@/gregor_test
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
	storage.InitMySQLEngine(t)

	srvAddr, events, cleanup := startTestGregord(t)
	defer cleanup()

	t.Logf("gregord server started on %v", srvAddr)

	clients := make([]*client, 5)
	for i := 0; i < len(clients); i++ {
		c, clean := startTestClient(t, srvAddr)
		defer clean()
		clients[i] = c
	}

	for i := 0; i < len(clients); i++ {
		<-events.ConnCreated
	}

	// send a message to the server from clients[0]
	if err := clients[0].IncomingClient().ConsumeMessage(context.TODO(), newUpdateMessage(t, clients[0].uid)); err != nil {
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

	post, clean := startTestClient(t, srvAddr)
	defer clean()

	<-events.ConnCreated

	if len(post.broadcasts) != 0 {
		t.Errorf("client connected after broadcast, has %d broadcasts, expected 0", len(post.broadcasts))
	}
}

var withSleep time.Duration

func maybeSleep() {
	if withSleep > 0 {
		time.Sleep(withSleep)
	}
}

func startTestGregord(t *testing.T) (net.Addr, *test.Events, func()) {
	name := os.Getenv("TEST_MYSQL_DSN")
	if name == "" {
		t.Skip("TEST_MYSQL_DSN not set")
	}
	u, err := url.Parse(name)
	if err != nil {
		t.Fatal(err)
	}

	opts := Options{
		MockAuth:    true,
		MysqlDSN:    u,
		BindAddress: "127.0.0.1:0",
		Debug:       true,
	}

	srv := grpc.NewServer(rpc.SimpleLogOutput{})
	srv.SetAuthenticator(mockAuth{})
	e := test.NewEvents()
	srv.SetEventHandler(e)

	consumer, err := newConsumer(u, rpc.SimpleLogOutput{})
	if err != nil {
		t.Fatal(err)
	}

	ms := newMainServer(&opts, srv)
	ms.stopCh = make(chan struct{})
	cleanup := func() {
		consumer.shutdown()
		close(ms.stopCh)
	}

	go srv.Serve(consumer)
	go func() {
		maybeSleep()
		if err := ms.listenAndServe(); err != nil {
			t.Fatal(err)
		}
	}()

	return <-ms.addr, e, cleanup
}

type client struct {
	conn       net.Conn
	tr         rpc.Transporter
	cli        *rpc.Client
	broadcasts []gregor1.Message
	uid        gregor1.UID
	sid        gregor1.SessionID
}

func startTestClient(t *testing.T, addr net.Addr) (*client, func()) {
	t.Logf("startTestClient dialing %v", addr)
	maybeSleep()
	c, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		t.Fatal(err)
	}
	maybeSleep()
	tr := rpc.NewTransport(c, nil, nil)

	x := &client{
		conn: c,
		tr:   tr,
		cli:  rpc.NewClient(tr, nil),
	}

	maybeSleep()
	srv := rpc.NewServer(tr, nil)
	if err := srv.Register(gregor1.OutgoingProtocol(x)); err != nil {
		t.Fatal(err)
	}

	res, err := x.AuthClient().AuthenticateSessionToken(context.TODO(), "anything")
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
			},
		},
	}

}
