package main

import (
	"database/sql"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"golang.org/x/net/context"
)

type notifier struct {
	db            *sql.DB
	sm            gregor.StateMachine
	notifications <-chan gregor.Item
	notify        gregor1.NotifyInterface
}

func newNotifier(dsn *url.URL, notify gregor1.NotifyInterface) (*notifier, error) {
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
	return &notifier{db, sm, notify}, nil
}

func (n *notifier) shutdown() {
	if n.notifications != nil {
		close(n.notifications)
	}
	if n.db != nil {
		n.db.Close()
	}
}

func (n *notifier) handleNotifications() {
	n.notifications = n.sm.Notifications()
	for notification := range n.notifications {
		if notification, ok := notification.(gregor1.ItemAndMetadata); ok {
			ctx, _ := context.WithTimeout(context.Background(), time.Second)
			if err := n.notify.Notify(ctx, notification); err != nil {
				log.Println(err)
			}
		}
	}
}
