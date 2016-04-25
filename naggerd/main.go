package main

import (
	"log"
	"os"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
)

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := opts.RemindServer.Dial()
	if err != nil {
		log.Fatal(err)
	}

	Cli := rpc.NewClient(rpc.NewTransport(conn, nil, keybase1.WrapError), keybase1.ErrorUnwrapper{})
	n, err := newNagger(opts.MysqlDSN, gregor1.RemindClient{Cli})
	if err != nil {
		log.Fatal(err)
	}
	defer n.shutdown()

	for _ = range time.Tick(opts.RemindDuration) {
		if err := n.sendReminders(); err != nil {
			log.Fatal(err)
		}
	}
}
