package main

import (
	"database/sql"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
)

var of gregor1.ObjFactory

type consumer struct {
	db *sql.DB
	sm gregor.StateMachine
	gregor1.IncomingInterface
	log rpc.LogOutput
}

func newConsumer(db *sql.DB, log rpc.LogOutput) *consumer {
	c := &consumer{
		db:  db,
		log: log,
		sm:  storage.NewMySQLEngine(db, of),
	}
	c.IncomingInterface = gregor1.NewLocalIncoming(c.sm)
	return c
}

func (c *consumer) shutdown() {
	c.log.Info("shutting down consumer")
	if c.db != nil {
		c.db.Close()
	}
}
