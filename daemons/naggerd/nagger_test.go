package main

import (
	"database/sql"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/daemons"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"github.com/keybase/gregor/test"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestNagger(t *testing.T) {
	remind := new(mockRemind)
	n := newMockNagger(t, remind)
	fc, ok := n.sm.Clock().(clockwork.FakeClock)
	if !ok {
		t.Fatal("state machine doesn't have a FakeClock")
	}
	doneCh := make(chan bool)
	go sendReminders(t, n, fc, doneCh)
	defer close(doneCh)

	rm1 := test.AddReminder(n.sm, time.Millisecond)
	rm2 := test.AddReminder(n.sm, 2*time.Second+time.Millisecond)

	fc.BlockUntil(1)
	assert.Equal(t, 0, len(remind.rms), "no reminders should be received yet")

	fc.Advance(time.Second)
	fc.BlockUntil(1)
	assert.Equal(t, 1, len(remind.rms), "1 reminder should be received")
	assert.Equal(t, rm1.Item().Metadata().UID(), remind.rms[0].Item().Metadata().UID(), "first reminder sent should be first received")
	assert.Equal(t, rm1.Item().Metadata().MsgID(), remind.rms[0].Item().Metadata().MsgID(), "first reminder sent should be first received")
	assert.Equal(t, rm1.RemindTime(), remind.rms[0].RemindTime(), "first reminder sent should be first received")

	fc.Advance(time.Second)
	fc.BlockUntil(1)
	assert.Equal(t, 1, len(remind.rms), "1 reminder should be received")
	fc.Advance(time.Second)
	fc.BlockUntil(1)
	assert.Equal(t, 2, len(remind.rms), "2 reminders should be received")
	assert.Equal(t, rm2.Item().Metadata().UID(), remind.rms[1].Item().Metadata().UID(), "second reminder sent should be second received")
	assert.Equal(t, rm2.Item().Metadata().MsgID(), remind.rms[1].Item().Metadata().MsgID(), "second reminder sent should be second received")
	assert.Equal(t, rm2.RemindTime(), remind.rms[1].RemindTime(), "second reminder sent should be second received")

}

func sendReminders(t *testing.T, n *nagger, cl clockwork.Clock, doneCh chan bool) {
	for {
		select {
		case <-cl.After(time.Second):
			if err := n.sendReminders(); err != nil {
				t.Fatal(err)
			}
		case <-doneCh:
			return
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
	return &nagger{db, storage.NewTestMySQLEngine(db, of), remind, daemons.NewLogger()}
}

type mockRemind struct {
	rms []gregor1.Reminder
}

func (m *mockRemind) Remind(_ context.Context, rms []gregor1.Reminder) error {
	m.rms = append(m.rms, rms...)
	return nil
}

var _ gregor1.RemindInterface = (*mockRemind)(nil)
