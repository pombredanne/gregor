package main

import (
	"database/sql"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"github.com/keybase/gregor/test"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestNagger(t *testing.T) {
	var remind mockRemind
	n := newMockNagger(t, remind)
	fc, ok := n.sm.Clock().(clockwork.FakeClock)
	if !ok {
		t.Fatal("state machine doesn't have a FakeClock")
	}
	doneCh := make(chan bool)
	go sendReminders(t, n, fc, doneCh)

	test.AddReminder(n.sm, time.Millisecond)
	test.AddReminder(n.sm, 2*time.Second+time.Millisecond)
	assert.Equal(t, 0, len(remind.rms), "no reminders should be sent yet")

	fc.Advance(time.Second)
	assert.Equal(t, 1, len(remind.rms), "1 reminders should have been sent")
	fc.Advance(time.Second)
	assert.Equal(t, 1, len(remind.rms), "1 reminders should have been sent")
	fc.Advance(time.Second)
	assert.Equal(t, 2, len(remind.rms), "2 reminders should have been sent")

	close(doneCh)
}

func sendReminders(t *testing.T, n *nagger, cl clockwork.Clock, doneCh chan bool) {
	for {
		select {
		case <-doneCh:
			return
		default:
			cl.Sleep(time.Second)
			if err := n.sendReminders(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func newMockNagger(t *testing.T, remind gregor1.RemindInterface) *nagger {
	name := os.Getenv("TEST_MYSQL_DSN")
	if name == "" {
		t.Skip("TEST_MYSQL_DSN not set")
	}

	dsn, err := url.Parse(name)
	if err != nil {
		t.Fatal(err)
	}
	dsn = storage.ForceParseTime(dsn)
	db, err := sql.Open("mysql", dsn.String())
	if err != nil {
		t.Fatal(err)
	}

	var of gregor1.ObjFactory
	return &nagger{db, storage.NewTestMySQLEngine(db, of), remind}
}

type mockRemind struct {
	rms []gregor1.Reminder
}

func (m mockRemind) Remind(_ context.Context, rms []gregor1.Reminder) error {
	m.rms = append(m.rms, rms...)
	return nil
}

var _ gregor1.RemindInterface = mockRemind{}
