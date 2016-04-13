package main

import (
	"crypto/rand"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"golang.org/x/net/context"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"
)

// Test with: MYSQL_DSN=gregor:@/gregor_test
func startTestGregord(t *testing.T) (string, func()) {
	name := os.Getenv("MYSQL_DSN")
	if name == "" {
		t.Skip("MYSQL_DSN not set")
	}
	u, err := url.Parse(name)
	if err != nil {
		t.Fatal(err)
	}

	opts := Options{
		MockAuth:    true,
		MysqlDSN:    u,
		BindAddress: "localhost:19911",
		Debug:       true,
	}

	srv := grpc.NewServer()
	setupMockAuth(srv)

	consumer, err := newConsumer(u)
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		consumer.shutdown()
	}
	go srv.Serve(consumer)

	ms := newMainServer(&opts, srv)
	go func() {
		if err := ms.listenAndServe(); err != nil {
			t.Fatal(err)
		}
	}()
	return opts.BindAddress, cleanup
}

type client struct {
	conn       net.Conn
	tr         rpc.Transporter
	cli        *rpc.Client
	broadcasts []gregor1.Message
	uid        gregor1.UID
	sid        gregor1.SessionID
}

func startTestClient(t *testing.T, addr string) (*client, func()) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	tr := rpc.NewTransport(c, nil, nil)

	x := &client{
		conn: c,
		tr:   tr,
		cli:  rpc.NewClient(tr, nil),
	}

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

func TestConsumeBroadcastFlow(t *testing.T) {
	srvAddr, cleanup := startTestGregord(t)
	defer cleanup()

	c1, c1clean := startTestClient(t, srvAddr)
	c2, c2clean := startTestClient(t, srvAddr)
	c3, c3clean := startTestClient(t, srvAddr)
	defer c1clean()
	defer c2clean()
	defer c3clean()

	// send a message to the server from c1
	if err := c1.IncomingClient().ConsumeMessage(context.TODO(), newUpdateMessage(t, c1.uid)); err != nil {
		t.Fatal(err)
	}

	// need to refactor rpc/events to make it available to other packages
	time.Sleep(100 * time.Millisecond)

	if len(c1.broadcasts) != 1 {
		t.Errorf("c1 broadcasts: %d, expected 1", len(c1.broadcasts))
	}
	if len(c2.broadcasts) != 1 {
		t.Errorf("c2 broadcasts: %d, expected 1", len(c2.broadcasts))
	}
	if len(c3.broadcasts) != 1 {
		t.Errorf("c3 broadcasts: %d, expected 1", len(c3.broadcasts))
	}
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
