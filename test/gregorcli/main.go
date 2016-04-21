package main

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

var (
	tok, rawAddr, caCerts string
)

func init() {
	flag.StringVar(&tok, "auth-tok", "", "Authenitcation token")
	flag.StringVar(&rawAddr, "remote", "", "Remote address to test")
	flag.StringVar(&caCerts, "ca", "", "PEM CA cert(s) to trust")
}

func setup() (*rpc.Client, error) {
	flag.Parse()
	fmpuri, err := rpc.ParseFMPURI(rawAddr)
	if err != nil {
		return nil, err
	}

	var config *tls.Config
	if caCerts != "" {
		if !fmpuri.UseTLS() {
			return nil, errors.New("attempting to set CAs for non-TLS connection")
		}

		pemCerts, err := ioutil.ReadFile(caCerts)
		if err != nil {
			return nil, err
		}
		config = &tls.Config{RootCAs: x509.NewCertPool()}
		if !config.RootCAs.AppendCertsFromPEM(pemCerts) {
			return nil, errors.New("no valid PEM certs found")
		}
	}
	conn, err := fmpuri.DialWithConfig(config)
	if err != nil {
		return nil, err
	}

	xp := rpc.NewTransport(conn, nil, keybase1.WrapError)

	Srv := rpc.NewServer(xp, keybase1.WrapError)
	var outgoing OutgoingHandler
	if err := Srv.Register(gregor1.OutgoingProtocol(outgoing)); err != nil {
		return nil, err
	}

	ch := Srv.Run()
	go func() {
		for _ = range ch {
		}
		log.Fatal("server chan closed")
	}()

	return rpc.NewClient(xp, keybase1.ErrorUnwrapper{}), nil
}

func testAuth(Cli *rpc.Client) (gregor1.UID, error) {
	auth := gregor1.AuthClient{Cli}
	authRes, err := auth.AuthenticateSessionToken(context.TODO(), gregor1.SessionToken(tok))
	if err != nil {
		return nil, err
	}
	return authRes.Uid, nil
}

func testPing(incoming gregor1.IncomingInterface) error {
	res, err := incoming.Ping(context.TODO())
	if err != nil {
		return err
	}
	if res != "pong" {
		return fmt.Errorf(`res (%s) != "pong"`, res)
	}
	return nil
}

func main() {
	Cli, err := setup()
	if err != nil {
		log.Fatal(err)
	}

	// Test AuthenticateSessionToken
	uid, err := testAuth(Cli)
	if err != nil {
		log.Fatal(err)
	}

	// Test Ping
	incoming := gregor1.IncomingClient{Cli}
	if err := testPing(incoming); err != nil {
		log.Fatal(err)
	}

	devid := gregor1.DeviceID("fakedevid1234567")

	// Test ConsumeMessage
	msg := makeMsg(uid, devid, []byte("fake body"), "foo")
	if err := incoming.ConsumeMessage(context.TODO(), msg); err != nil {
		log.Fatal(err)
	}

	// Test Sync
	syncRes, err := incoming.Sync(context.TODO(), gregor1.SyncArg{
		Uid:      uid,
		Deviceid: devid,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%d messages synced\n", len(syncRes.Msgs))
}

func makeMsgID() gregor1.MsgID {
	msgid := gregor1.MsgID(make([]byte, 16))
	if _, err := rand.Read(msgid); err != nil {
		log.Fatal(err)
	}
	return msgid
}

func makeMsg(uid gregor1.UID, devid gregor1.DeviceID, body []byte, category string) gregor1.Message {
	var of gregor1.ObjFactory
	i, err := of.MakeItem(uid, makeMsgID(), devid, time.Now(), gregor1.Category(category), nil, gregor1.Body(body))
	if err != nil {
		log.Fatal(err)
	}

	ibmsg, err := of.MakeInBandMessageFromItem(i)
	if err != nil {
		log.Fatal(err)
	}

	ibm, ok := ibmsg.(gregor1.InBandMessage)
	if !ok {
		log.Fatal("invalid InBandMessage")
	}

	return gregor1.Message{Ibm_: &ibm}
}

type OutgoingHandler struct{}

func (o OutgoingHandler) BroadcastMessage(_ context.Context, m gregor1.Message) error {
	if ibm := m.ToInBandMessage(); ibm != nil {
		devid := ibm.Metadata().DeviceID()
		fmt.Printf("received message: %#+v\n", devid)
	}

	return nil
}

var _ gregor1.OutgoingInterface = OutgoingHandler{}
