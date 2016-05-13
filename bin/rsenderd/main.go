package main

import (
	"database/sql"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/keybase/gregor/bin"
)

func main() {
	log := bin.NewLogger("rsenderd")
	log.Info("starting rsenderd")

	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(1)
	}

	db, err := sql.Open("mysql", opts.MysqlDSN)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(2)
	}

	r := newRSender(db, opts.RemindServer, log)
	for _ = range time.Tick(opts.RemindDuration) {
		if err := r.sendReminders(); err != nil {
			log.Error("%#v", err)
		}
	}
}
