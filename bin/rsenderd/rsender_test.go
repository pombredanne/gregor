package main

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/bin"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/storage"
	"github.com/keybase/gregor/test"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestRSender(t *testing.T) {
	remind := new(mockRemind)
	r := newMockRSender(t, remind)
	defer storage.ReleaseTestDB()
	fc, ok := r.sm.Clock().(clockwork.FakeClock)
	if !ok {
		t.Fatal("state machine doesn't have a FakeClock")
	}
	doneCh := make(chan bool)
	defer close(doneCh)

	rm1 := test.AddReminder(r.sm, time.Millisecond)
	rm2 := test.AddReminder(r.sm, 2*time.Second+time.Millisecond)

	if err := r.sendReminders(); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 0, len(remind.rms), "no reminders should be received yet")

	fc.Advance(time.Second)
	if err := r.sendReminders(); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(remind.rms), "1 reminder should be received")
	assert.Equal(t, rm1.Item().Metadata().UID(), remind.rms[0].Item().Metadata().UID(), "first reminder sent should be first received")
	assert.Equal(t, rm1.Item().Metadata().MsgID(), remind.rms[0].Item().Metadata().MsgID(), "first reminder sent should be first received")
	assert.Equal(t, rm1.RemindTime(), remind.rms[0].RemindTime(), "first reminder sent should be first received")

	fc.Advance(time.Second)
	if err := r.sendReminders(); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(remind.rms), "1 reminder should be received")

	fc.Advance(time.Second)
	if err := r.sendReminders(); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(remind.rms), "2 reminders should be received")
	assert.Equal(t, rm2.Item().Metadata().UID(), remind.rms[1].Item().Metadata().UID(), "second reminder sent should be second received")
	assert.Equal(t, rm2.Item().Metadata().MsgID(), remind.rms[1].Item().Metadata().MsgID(), "second reminder sent should be second received")
	assert.Equal(t, rm2.RemindTime(), remind.rms[1].RemindTime(), "second reminder sent should be second received")
}

func newMockRSender(t *testing.T, remind gregor1.RemindInterface) *rSender {
	var of gregor1.ObjFactory
	db := storage.AcquireTestDB(t)
	sm, _ := storage.NewTestMySQLEngine(db, of)
	return &rSender{db: db, sm: sm, remind: remind, log: bin.NewLogger("rsenderd")}
}

type mockRemind struct {
	rms []gregor1.Reminder
}

func (m *mockRemind) Remind(_ context.Context, r gregor1.Reminder) error {
	m.rms = append(m.rms, r)
	return nil
}

var _ gregor1.RemindInterface = (*mockRemind)(nil)