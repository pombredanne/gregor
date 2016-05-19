package main

import (
	"database/sql"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"golang.org/x/net/context"
)

var of gregor1.ObjFactory

type rSender struct {
	db     *sql.DB
	sm     gregor.StateMachine
	remind gregor1.RemindInterface
	log    rpc.LogOutput
}

func newRSender(db *sql.DB, remindServer *rpc.FMPURI, log rpc.LogOutput, mysqlDSN string) *rSender {
	transport := rpc.NewConnectionTransport(remindServer, rpc.NewSimpleLogFactory(log, nil), keybase1.WrapError)
	conn := rpc.NewConnectionWithTransport(nil, transport, keybase1.ErrorUnwrapper{},
		true, keybase1.WrapError, log, nil)
	return &rSender{db, storage.NewMySQLEngine(db, of, mysqlDSN), gregor1.RemindClient{conn.GetClient()}, log}
}

func (r *rSender) sendReminders() error {
	reminders, err := r.sm.Reminders()
	if err != nil {
		return err
	}

	r.log.Debug("sending %d reminders", len(reminders))
	for _, rm := range reminders {
		if err := r.sm.DeleteReminder(rm); err != nil {
			return err
		}
		if rm, ok := rm.(gregor1.Reminder); ok {
			ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
			if err := r.remind.Remind(ctx, rm); err != nil {
				return err
			}
		}
	}
	r.log.Debug("reminders sent succesfully")
	return nil
}
