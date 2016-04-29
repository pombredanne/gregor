package main

import (
	"database/sql"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"golang.org/x/net/context"
)

var of gregor1.ObjFactory

type nagger struct {
	db     *sql.DB
	sm     gregor.StateMachine
	remind gregor1.RemindInterface
	log    rpc.LogOutput
}

func newNagger(db *sql.DB, remind gregor1.RemindInterface, log rpc.LogOutput) *nagger {
	return &nagger{db, storage.NewMySQLEngine(db, of), remind, log}
}

func (n *nagger) shutdown() {
	n.log.Info("shutting down nagger")
	if n.db != nil {
		n.db.Close()
	}
}

func (n *nagger) sendReminders() error {
	reminders, err := n.sm.Reminders()
	if err != nil {
		return err
	}

	n.log.Debug("sending %d reminders", len(reminders))
	for _, rm := range reminders {
		if err := n.sm.DeleteReminder(rm); err != nil {
			return err
		}
		if rm, ok := rm.(gregor1.Reminder); ok {
			ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
			if err := n.remind.Remind(ctx, rm); err != nil {
				return err
			}
		}
	}
	n.log.Debug("reminders sent succesfully")
	return nil
}
