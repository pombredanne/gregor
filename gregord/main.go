package main

import (
	"log"
	"os"
	"time"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"
)

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Fatal(err)
	}

	srv := grpc.NewServer()
	conn, err := opts.SessionServer.Dial()
	if err != nil {
		log.Fatal(err)
	}

	Cli := rpc.NewClient(rpc.NewTransport(conn, nil, keybase1.WrapError), keybase1.ErrorUnwrapper{})
	sc := grpc.NewSessionCacher(gregor1.AuthClient{Cli}, clockwork.NewRealClock(), 10*time.Minute)
	defer sc.Close()
	srv.SetAuthenticator(sc)

	// create a message consumer state machine
	consumer, err := newConsumer(opts.MysqlDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer consumer.shutdown()
	go srv.Serve(consumer)

	log.Fatal(newMainServer(opts, srv).listenAndServe())
}
