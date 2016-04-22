package main

import (
	"log"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"
	"golang.org/x/net/context"
)

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Fatal(err)
	}
	Cli := rpc.NewClient(rpc.NewTransport(conn, nil, keybase1.WrapError), keybase1.ErrorUnwrapper{})
	n := newNotifier(opts.MysqlDSN, gregor1.NotifyClient{Cli})
	defer n.shutdown()

	n.handleNotifications()
}
