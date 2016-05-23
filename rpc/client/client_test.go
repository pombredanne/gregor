package storage

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"
	storage "github.com/keybase/gregor/storage"
	"github.com/keybase/gregor/test"
	"github.com/syndtr/goleveldb/leveldb"
	context "golang.org/x/net/context"
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
		client:      storage.NewMemEngine(of, fc),
		server:      storage.NewMemEngine(of, fc),
		clientOF:    of,
		clientClock: fc,
	}
}

func (sm *clientServerSM) Clear() error {
	return sm.client.Clear()
}

func (sm *clientServerSM) ConsumeMessage(m gregor.Message) (time.Time, error) {
	if sm.client != nil {
		if ctime, err := sm.client.ConsumeMessage(m); err != nil {
			return ctime, err
		}
	}

	ctime, err := sm.server.ConsumeMessage(m)
	return ctime, err
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

type mockLocalIncoming struct {
	sm gregor.StateMachine
}

func (m mockLocalIncoming) Sync(_ context.Context, arg gregor1.SyncArg) (gregor1.SyncResult, error) {
	return grpc.Sync(m.sm, rpc.SimpleLogOutput{}, arg)
}
func (m mockLocalIncoming) ConsumeMessage(_ context.Context, _ gregor1.Message) error {
	return errors.New("unimplemented")
}
func (m mockLocalIncoming) ConsumePublishMessage(_ context.Context, _ gregor1.Message) error {
	return errors.New("unimplemented")
}
func (m mockLocalIncoming) Ping(_ context.Context) (string, error) {
	return "pong", nil
}

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

	cli := mockLocalIncoming{sm.server}
	c := NewClient(user, device, sm, &LevelDBStorageEngine{db}, time.Minute, rpc.SimpleLogOutput{})

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

	if err := c.syncFromTime(cli, c.sm.LatestCTime(c.user, c.device)); err != nil {
		t.Fatal(err)
	}

	test.AddStateMachinePerDevice(sm.client, user, nil)

	err = c.syncFromTime(cli, c.sm.LatestCTime(c.user, c.device))
	if _, ok := err.(errHashMismatch); !ok {
		t.Fatal(err)
	}

	if err := c.Sync(cli); err != nil {
		t.Fatal(err)
	}
}
