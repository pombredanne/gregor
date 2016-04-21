package main

import (
	"database/sql"
	"errors"
	"net/url"

	_ "github.com/go-sql-driver/mysql"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"

	log "github.com/Sirupsen/logrus"
)

type consumer struct {
	db *sql.DB
	sm gregor.StateMachine
	gregor1.IncomingInterface
}

func newConsumer(dsn *url.URL) (*consumer, error) {
	if dsn == nil {
		return nil, errors.New("nil mysql dsn provided to newConsumer")
	}
	c := &consumer{}
	if err := c.setup(dsn); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *consumer) setup(dsn *url.URL) error {
	if err := c.createDB(dsn); err != nil {
		return err
	}
	if err := c.createStateMachine(); err != nil {
		return err
	}
	return nil
}

func (c *consumer) shutdown() {
	if c.db != nil {
		c.db.Close()
	}
}

func (c *consumer) createDB(dsn *url.URL) error {
	dsn = storage.ForceParseTime(dsn)
	log.Printf("opening mysql connection to %s", dsn)
	db, err := sql.Open("mysql", dsn.String())
	if err != nil {
		return err
	}
	c.db = db
	return nil
}

func (c *consumer) createStateMachine() error {
	var of gregor1.ObjFactory
	c.sm = storage.NewMySQLEngine(c.db, of)
	c.IncomingInterface = gregor1.NewLocalIncoming(c.sm)
	return nil
}
