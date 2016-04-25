package main

import (
	"database/sql"
	"errors"
	"log"
	"net/url"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"golang.org/x/net/context"
)

type reminder struct {
	db     *sql.DB
	sm     gregor.StateMachine
	remind gregor1.RemindInterface
}

func newReminder(dsn *url.URL, remind gregor1.RemindInterface) (*reminder, error) {
	if dsn == nil {
		return nil, errors.New("nil mysql dsn provided to newConsumer")
	}

	dsn = storage.ForceParseTime(dsn)
	log.Printf("opening mysql connection to %s", dsn)
	db, err := sql.Open("mysql", dsn.String())
	if err != nil {
		return nil, err
	}

	var of gregor1.ObjFactory
	sm := storage.NewMySQLEngine(db, of)
	return &reminder{db, sm, remind}, nil
}

func (r *reminder) shutdown() {
	if r.db != nil {
		r.db.Close()
	}
}

func (r *reminder) sendReminders() error {
	rs, err := r.sm.Reminders()
	if err != nil {
		return err
	}

	var rs1 []gregor1.Reminder
	for _, rm := range rs {
		if rm, ok := rm.(gregor1.Reminder); ok {
			rs1 = append(rs1, rm)
		}
	}

	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	return r.remind.Remind(ctx, rs1)
}
