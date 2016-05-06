package main

import (
	"database/sql"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/bin"
	"github.com/keybase/gregor/protocol/gregor1"
)

func main() {
	log := bin.NewLogger("naggerd")

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
	db, err := sql.Open("mysql", opts.MysqlDSN)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(3)
	}

	nagger := newNagger(db, gregor1.RemindClient{Cli}, log)
	defer nagger.shutdown()

	for _ = range time.Tick(opts.RemindDuration) {
		if err := nagger.sendReminders(); err != nil {
			log.Error("%#v", err)
			os.Exit(4)
		}
	}
}
