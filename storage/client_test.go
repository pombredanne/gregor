package storage

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/test"
	"github.com/syndtr/goleveldb/leveldb"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
)

type clientServerSM struct {
	client      gregor.StateMachine
	server      gregor.StateMachine
	clientOF    gregor.ObjFactory
	clientClock clockwork.Clock
}

func NewClientServerSM() *clientServerSM {
	var of gregor1.ObjFactory
	fc := clockwork.NewFakeClock()
	return &clientServerSM{
		client:      NewMemEngine(of, fc),
		server:      NewMemEngine(of, fc),
		clientOF:    of,
		clientClock: fc,
	}
}

func (sm *clientServerSM) Clear() error {
	return sm.client.Clear()
}

func (sm *clientServerSM) ConsumeMessage(m gregor.Message) error {
	if sm.client != nil {
		if err := sm.client.ConsumeMessage(m); err != nil {
			return err
		}
	}

	return sm.server.ConsumeMessage(m)
}

func (sm *clientServerSM) State(u gregor.UID, d gregor.DeviceID, t gregor.TimeOrOffset) (gregor.State, error) {
	return sm.client.State(u, d, t)
}

func (sm *clientServerSM) IsEphemeral() bool {
	return sm.client.IsEphemeral()
}

func (sm *clientServerSM) InitState(s gregor.State) error {
	return sm.client.InitState(s)
}

func (sm *clientServerSM) LatestCTime(u gregor.UID, d gregor.DeviceID) *time.Time {
	return sm.client.LatestCTime(u, d)
}

func (sm *clientServerSM) InBandMessagesSince(u gregor.UID, d gregor.DeviceID, t time.Time) ([]gregor.InBandMessage, error) {
	return sm.server.InBandMessagesSince(u, d, t)
}

func (sm *clientServerSM) Reminders() ([]gregor.Reminder, error) {
	return sm.server.Reminders()
}

func (sm *clientServerSM) DeleteReminder(r gregor.Reminder) error {
	return sm.server.DeleteReminder(r)
}

func (sm *clientServerSM) ObjFactory() gregor.ObjFactory {
	return sm.clientOF
}

func (sm *clientServerSM) Clock() clockwork.Clock {
	return sm.clientClock
}

var _ gregor.StateMachine = (*clientServerSM)(nil)

func TestLevelDBClient(t *testing.T) {
	fname, err := ioutil.TempDir("", "gregor")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fname)

	sm := NewClientServerSM()
	user, device := test.TestStateMachinePerDevice(t, sm)
	test.AddStateMachinePerDevice(sm, user, device)
	db, err := leveldb.OpenFile(fname, nil)
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(user, device, sm, &LevelDBStorageEngine{db}, gregor1.NewLocalIncoming(sm.server), rpc.SimpleLogOutput{})

	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	test.AddStateMachinePerDevice(sm.server, user, device)

	if err := sm.Clear(); err != nil {
		t.Fatal(err)
	}

	if err := c.Restore(); err != nil {
		t.Fatal(err)
	}

	if err := c.syncFromTime(c.sm.LatestCTime(c.user, c.device)); err != nil {
		t.Fatal(err)
	}

	test.AddStateMachinePerDevice(sm.client, user, nil)

	err = c.syncFromTime(c.sm.LatestCTime(c.user, c.device))
	if _, ok := err.(errHashMismatch); !ok {
		t.Fatal(err)
	}

	if err := c.Sync(); err != nil {
		t.Fatal(err)
	}
}
