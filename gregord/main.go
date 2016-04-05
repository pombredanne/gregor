package main

import (
	"log"
	"net"
	"os"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	grpc "github.com/keybase/gregor/rpc"
)

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Fatal(err)
	}

	srv := grpc.NewServer()
	conn, err := net.Dial(opts.SessionServer.Network(), opts.SessionServer.String())
	if err != nil {
		log.Fatal(err)
	}

	srv.SetAuthenticator(grpc.NewAuthClient(rpc.NewClient(rpc.NewTransport(conn, nil, nil), nil)))

	log.Fatal(newMainServer(opts, srv).listenAndServe())
}
