package main

import (
	"os"
	"time"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"
	"golang.org/x/net/context"
)

func main() {
	log := newLogger()

	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(1)
	}

	srv := grpc.NewServer(log)

	if opts.MockAuth {
		srv.SetAuthenticator(mockAuth{})
	} else {
		conn, err := opts.SessionServer.Dial()
		if err != nil {
			log.Error("%#v", err)
			os.Exit(2)
		}
		Cli := rpc.NewClient(rpc.NewTransport(conn, rpc.NewSimpleLogFactory(log, nil), keybase1.WrapError), keybase1.ErrorUnwrapper{})
		sc := grpc.NewSessionCacher(gregor1.AuthClient{Cli}, clockwork.NewRealClock(), 10*time.Minute)
		srv.SetAuthenticator(sc)
		defer sc.Close()
	}

	// create a message consumer state machine
	consumer, err := newConsumer(opts.MysqlDSN, log)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(3)
	}
	defer consumer.shutdown()
	go srv.Serve(consumer)

	log.Error("%#v", newMainServer(opts, srv).listenAndServe())
	os.Exit(4)
}

type mockAuth struct{}

func (m mockAuth) AuthenticateSessionToken(_ context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {
	return gregor1.AuthResult{Uid: gregor1.UID("gooduid"), Sid: gregor1.SessionID("1")}, nil
}
func (m mockAuth) RevokeSessionIDs(_ context.Context, sessionIDs []gregor1.SessionID) error {
	return nil
}

var _ gregor1.AuthInterface = mockAuth{}
