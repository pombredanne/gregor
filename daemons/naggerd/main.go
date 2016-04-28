package main

import (
	"os"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/daemons"
	"github.com/keybase/gregor/protocol/gregor1"
)

func main() {
	log := daemons.NewLogger()

	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(1)
	}

	conn, err := opts.RemindServer.Dial()
	if err != nil {
		log.Error("%#v", err)
		os.Exit(2)
	}

	Cli := rpc.NewClient(rpc.NewTransport(conn, rpc.NewSimpleLogFactory(log, nil), keybase1.WrapError), keybase1.ErrorUnwrapper{})
	n, err := newNagger(opts.MysqlDSN, gregor1.RemindClient{Cli}, log)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(3)
	}
	defer n.shutdown()

	for _ = range time.Tick(opts.RemindDuration) {
		if err := n.sendReminders(); err != nil {
			log.Error("%#v", err)
			os.Exit(4)
		}
	}
}
