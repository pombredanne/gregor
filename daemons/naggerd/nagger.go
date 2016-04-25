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

type nagger struct {
	db     *sql.DB
	sm     gregor.StateMachine
	remind gregor1.RemindInterface
}

func newNagger(dsn *url.URL, remind gregor1.RemindInterface) (*nagger, error) {
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
	return &nagger{db, sm, remind}, nil
}

func (n *nagger) shutdown() {
	if n.db != nil {
		n.db.Close()
	}
}

func (n *nagger) sendReminders() error {
	reminders, err := n.sm.Reminders()
	if err != nil {
		return err
	}

	var reminders1 []gregor1.Reminder
	for _, rm := range reminders {
		if rm, ok := rm.(gregor1.Reminder); ok {
			reminders1 = append(reminders1, rm)
		}
	}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	if err := n.remind.Remind(ctx, reminders1); err != nil {
		return err
	}

	for _, rm := range reminders {
		if err := n.sm.DeleteReminder(rm); err != nil {
			return err
		}
	}
	return nil
}
