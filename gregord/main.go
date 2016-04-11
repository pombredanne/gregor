package main

import (
	"log"
	"os"
	"time"

	"github.com/keybase/client/go/libkb"
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

	Cli := rpc.NewClient(rpc.NewTransport(conn, nil, libkb.WrapError), libkb.ErrorUnwrapper{})
	srv.SetAuthenticator(grpc.NewSessionCacher(gregor1.AuthClient{Cli}, 10*time.Minute))

	log.Fatal(newMainServer(opts, srv).listenAndServe())
}
