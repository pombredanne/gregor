package main

import (
	"log"
	"os"

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

	Cli := rpc.NewClient(rpc.NewTransport(conn, nil, nil), nil)
	srv.SetAuthenticator(grpc.NewSessionCacher(gregor1.AuthClient{Cli}))

	log.Fatal(newMainServer(opts, srv).listenAndServe())
}
