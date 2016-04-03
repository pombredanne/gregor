package test

import (
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	gregor "github.com/keybase/gregor"
	"github.com/stretchr/testify/require"
)

type testUID []byte
type testMsgID []byte
type testDeviceID []byte
type testSystem string
type testCategory string

type testItem struct {
	m      *testMetadata
	dtime  gregor.TimeOrOffset
	nTimes []gregor.TimeOrOffset
	cat    gregor.Category
	body   gregor.Body
}

type testTimeOrOffset struct {
	t *time.Time
	d *time.Duration
}

type testMsgRange struct {
	e gregor.TimeOrOffset
	c gregor.Category
}

type testDismissal struct {
	ids    []gregor.MsgID
	ranges []testMsgRange
}

type testMetadata struct {
	u gregor.UID
	m gregor.MsgID
	d gregor.DeviceID
	t time.Time
}

type testSyncMessage testMetadata

type testInBandMessage struct {
	m *testMetadata
	i *testItem
	d *testDismissal
	s *testSyncMessage
}

type testMessage struct {
	i *testInBandMessage
}

type TestObjFactory struct{}
type testState []gregor.Item

func (f TestObjFactory) MakeUID(b []byte) (gregor.UID, error) {
	return testUID(b), nil
}
func (f TestObjFactory) MakeMsgID(b []byte) (gregor.MsgID, error) {
	return testMsgID(b), nil
}
func (f TestObjFactory) MakeDeviceID(b []byte) (gregor.DeviceID, error) {
	return testDeviceID(b), nil
}
func (f TestObjFactory) MakeBody(b []byte) (gregor.Body, error) {
	return testBody(b), nil
}
func (f TestObjFactory) MakeCategory(s string) (gregor.Category, error) {
	return testCategory(s), nil
}
func (f TestObjFactory) MakeItem(u gregor.UID, msgid gregor.MsgID, deviceid gregor.DeviceID, ctime time.Time, c gregor.Category, dtime *time.Time, body gregor.Body) (gregor.Item, error) {
	ret := &testItem{
		m:    newTestMetadata(u, msgid, deviceid, ctime),
		cat:  c,
		body: body,
	}
	if dtime != nil {
		ret.dtime = timeToTimeOrOffset(*dtime)
	}
	return ret, nil
}
func (f TestObjFactory) MakeDismissalByRange(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time, c gregor.Category, d time.Time) (gregor.InBandMessage, error) {
	md := newTestMetadata(uid, msgid, devid, ctime)
	td := &testDismissal{
		ranges: []testMsgRange{
			testMsgRange{
				e: timeToTimeOrOffset(d),
				c: c,
			},
		},
	}
	return &testInBandMessage{m: md, d: td}, nil
}

func (f TestObjFactory) MakeDismissalByID(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time, d gregor.MsgID) (gregor.InBandMessage, error) {
	md := newTestMetadata(uid, msgid, devid, ctime)
	td := &testDismissal{
		ids: []gregor.MsgID{d},
	}
	return &testInBandMessage{m: md, d: td}, nil
}

func (f TestObjFactory) MakeStateSyncMessage(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time) (gregor.InBandMessage, error) {
	md := newTestMetadata(uid, msgid, devid, ctime)
	return &testInBandMessage{m: md, s: (*testSyncMessage)(md)}, nil
}

func (f TestObjFactory) MakeState(i []gregor.Item) (gregor.State, error) {
	return testState(i), nil
}
func (f TestObjFactory) MakeMetadata(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time, i gregor.InBandMsgType) (gregor.Metadata, error) {
	return newTestMetadata(uid, msgid, devid, ctime), nil
}

var errBadType = errors.New("bad type in cast")

func (f TestObjFactory) MakeInBandMessageFromItem(i gregor.Item) (gregor.InBandMessage, error) {
	ti, ok := i.(*testItem)
	if !ok {
		return nil, errBadType
	}
	return &testInBandMessage{m: ti.m, i: ti}, nil
}

func (f TestObjFactory) UnmarshalState(b []byte) (gregor.State, error) {
	panic("UnmarshalState unimplemented for TestObjFactory")
}

func newTestMetadata(u gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time) *testMetadata {
	return &testMetadata{
		u: u, m: msgid, d: devid, t: ctime,
	}
}

var _ gregor.ObjFactory = TestObjFactory{}

type testBody string

func (t testBody) Bytes() []byte { return []byte(t) }

func (t *testMetadata) MsgID() gregor.MsgID                 { return t.m }
func (t *testMetadata) CTime() time.Time                    { return t.t }
func (t *testMetadata) SetCTime(x time.Time)                { t.t = x }
func (t *testMetadata) DeviceID() gregor.DeviceID           { return t.d }
func (t *testMetadata) UID() gregor.UID                     { return t.u }
func (t *testMetadata) InBandMsgType() gregor.InBandMsgType { return gregor.InBandMsgTypeUpdate }

func (t *testItem) DTime() gregor.TimeOrOffset         { return t.dtime }
func (t *testItem) NotifyTimes() []gregor.TimeOrOffset { return t.nTimes }
func (t *testItem) Body() gregor.Body                  { return t.body }
func (t *testItem) Category() gregor.Category          { return t.cat }
func (t *testItem) Metadata() gregor.Metadata          { return t.m }

func (t testMsgRange) EndTime() gregor.TimeOrOffset { return t.e }
func (t testMsgRange) Category() gregor.Category    { return t.c }

func (t *testSyncMessage) Metadata() gregor.Metadata { return (*testMetadata)(t) }

func (t testInBandMessage) Metadata() gregor.Metadata                       { return t.m }
func (t testInBandMessage) ToStateSyncMessage() gregor.StateSyncMessage     { return t.s }
func (t testInBandMessage) ToStateUpdateMessage() gregor.StateUpdateMessage { return t }

func (t testInBandMessage) Dismissal() gregor.Dismissal {
	if t.d == nil {
		return nil
	}
	return t.d
}
func (t testInBandMessage) Creation() gregor.Item {
	if t.i == nil {
		return nil
	}
	return t.i
}

func (t testInBandMessage) Merge(m2 gregor.InBandMessage) error {
	t2, ok := m2.(testInBandMessage)
	if !ok {
		return fmt.Errorf("bad merge; wrong type: %T", m2)
	}
	if t.i != nil && t2.i != nil {
		return errors.New("clash of creations")
	}
	if t.i == nil {
		t.i = t2.i
	}
	if t.d == nil {
		t.d = t2.d
	} else if t2.d != nil {
		t.d.ids = append(t.d.ids, t2.d.ids...)
		t.d.ranges = append(t.d.ranges, t2.d.ranges...)
	}
	return nil
}

func (t *testDismissal) MsgIDsToDismiss() []gregor.MsgID { return t.ids }

func (t testTimeOrOffset) Time() *time.Time       { return t.t }
func (t testTimeOrOffset) Offset() *time.Duration { return t.d }
func (t testTimeOrOffset) Before(t2 time.Time) bool {
	return t.Time() != nil && t.Time().Before(t2)
}
func (t testUID) Bytes() []byte       { return t }
func (t testMsgID) Bytes() []byte     { return t }
func (t testDeviceID) Bytes() []byte  { return t }
func (t testCategory) String() string { return string(t) }
func (t testSystem) String() string   { return string(t) }

func (m testMessage) ToInBandMessage() gregor.InBandMessage       { return m.i }
func (m testMessage) ToOutOfBandMessage() gregor.OutOfBandMessage { return nil }

func (t *testDismissal) RangesToDismiss() []gregor.MsgRange {
	var ret []gregor.MsgRange
	for _, r := range t.ranges {
		ret = append(ret, gregor.MsgRange(r))
	}
	return ret
}

func (t testState) Items() ([]gregor.Item, error) {
	return []gregor.Item(t), nil
}

func (t testState) ItemsInCategory(c gregor.Category) ([]gregor.Item, error) {
	var ret []gregor.Item
	for _, item := range t {
		if item.Category().String() == c.String() {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func (t testState) Marshal() ([]byte, error) {
	panic("Unimplemented Marshal for testState")
}

var _ gregor.State = testState(nil)
var _ gregor.Item = (*testItem)(nil)
var _ gregor.InBandMessage = testInBandMessage{}

func assertNItems(t *testing.T, sm gregor.StateMachine, u gregor.UID, d gregor.DeviceID, too gregor.TimeOrOffset, n int) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.Items()
	require.Nil(t, err, "no error from Items()")
	require.Equal(t, n, len(it), "wrong number of items")
}

func assertNItemsInCategory(t *testing.T, sm gregor.StateMachine, u gregor.UID, d gregor.DeviceID, too gregor.TimeOrOffset, c gregor.Category, n int) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.ItemsInCategory(c)
	require.Nil(t, err, "no error from ItemsInCategory()")
	require.Equal(t, n, len(it), "wrong number of items")
}
func assertBodiesInCategory(t *testing.T, sm gregor.StateMachine, u gregor.UID, d gregor.DeviceID, too gregor.TimeOrOffset, c gregor.Category, expected []string) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.ItemsInCategory(c)
	require.Nil(t, err, "no error from ItemsInCategory()")
	require.Len(t, expected, len(it), "wrong number of items")
	actual := make([]string, 0)
	for _, a := range it {
		actual = append(actual, string(a.Body().Bytes()))
	}
	require.Equal(t, expected, actual, "the right values in the store")
}

func randBytes(n int) []byte {
	ret := make([]byte, n)
	rand.Read(ret)
	return ret
}

func makeUID() gregor.UID           { return testUID(randBytes(8)) }
func makeMsgID() gregor.MsgID       { return testMsgID(randBytes(8)) }
func makeDeviceID() gregor.DeviceID { return testDeviceID(randBytes(8)) }
func makeOffset(i int) gregor.TimeOrOffset {
	d := time.Second * time.Duration(i)
	return testTimeOrOffset{d: &d}
}
func timeToTimeOrOffset(t time.Time) gregor.TimeOrOffset {
	return testTimeOrOffset{t: &t}
}

func newCreation(u gregor.UID, m gregor.MsgID, d gregor.DeviceID, c gregor.Category, data string, dtime gregor.TimeOrOffset) gregor.Message {
	md := &testMetadata{u: u, m: m, d: d}
	item := &testItem{m: md, dtime: dtime, body: testBody(data), cat: c}
	return testMessage{i: &testInBandMessage{m: md, i: item}}
}

func newDismissalByIDs(u gregor.UID, m gregor.MsgID, d gregor.DeviceID, ids []gregor.MsgID) gregor.Message {
	md := &testMetadata{u: u, m: m, d: d}
	dismissal := &testDismissal{ids: ids}
	ret := testMessage{i: &testInBandMessage{m: md, d: dismissal}}
	return ret
}

func newDismissalByCategory(u gregor.UID, m gregor.MsgID, d gregor.DeviceID, c gregor.Category, e gregor.TimeOrOffset) gregor.Message {
	md := &testMetadata{u: u, m: m, d: d}
	dismissal := &testDismissal{ranges: []testMsgRange{{c: c, e: e}}}
	ret := testMessage{i: &testInBandMessage{m: md, d: dismissal}}
	return ret
}

func consumeMessage(t *testing.T, which string, sm gregor.StateMachine, m gregor.Message) {
	err := sm.ConsumeMessage(m)
	if err != nil {
		t.Fatalf("In inserting msg %s: %v", which, err)
	}
}

func TestStateMachineAllDevices(t *testing.T, sm gregor.StateMachine, fc clockwork.FakeClock) {
	t0 := fc.Now()
	u1 := makeUID()
	c1 := testCategory("foos")
	c2 := testCategory("bars")

	// Make an assertion: that there are no items total in the StateMachine,
	// and no items at in the category 'foos'
	assert1 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}

	// Do the assertion for "now" = nil
	assert1(nil)

	// Produce a new mesasge, with payload "f1"
	m1 := makeMsgID()
	consumeMessage(t, "m1", sm,
		newCreation(u1, m1, nil, c1, "f1", nil),
	)

	// Make an assertion: that there is 1 item in the StateMachine,
	// and 1 item for the category "foos"
	assert2 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 1)
	}

	// Do the assertion for "now" = nil
	assert2(nil)

	// Produce a new *dismissal* message that dismisses the first message
	// we added (m1)
	consumeMessage(t, "d1", sm, newDismissalByIDs(u1, makeMsgID(), nil, []gregor.MsgID{m1}))

	// Make an assertion: that there are no items left in the StateMachine
	assert3 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	tm3 := fc.Now()

	// Do the assertion for "now" = nil
	assert3(nil)
	fc.Advance(time.Second)

	// Make and consume 3 new messages.
	consumeMessage(t, "m2", sm,
		newCreation(u1, makeMsgID(), nil, c1, "f2", nil),
	)
	consumeMessage(t, "m3", sm,
		newCreation(u1, makeMsgID(), nil, c1, "f3", makeOffset(3)),
	)
	consumeMessage(t, "m4", sm,
		newCreation(u1, makeMsgID(), nil, c2, "b1", nil),
	)

	// Make an assertion: that the items wound up and in the right
	// categories.
	assert4 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 3)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c2, 1)
	}
	assert4(nil)
	tm4 := fc.Now()
	fc.Advance(time.Duration(4) * time.Second)

	// Make an assertion: that the dismissals worked as planned
	assert5 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 1)
		assertBodiesInCategory(t, sm, u1, nil, too, c1, []string{"f2"})
		assertBodiesInCategory(t, sm, u1, nil, too, c2, []string{"b1"})
	}
	tm5 := fc.Now()
	assert5(nil)

	// Assert our previous checkpoint still works
	assert3(timeToTimeOrOffset(tm3))
	assert4(timeToTimeOrOffset(tm4))

	consumeMessage(t, "m5", sm,
		newCreation(u1, makeMsgID(), nil, c2, "b3", nil),
	)
	fc.Advance(time.Duration(4) * time.Second)
	consumeMessage(t, "m6", sm,
		newCreation(u1, makeMsgID(), nil, c2, "b4", nil),
	)
	fc.Advance(time.Duration(1) * time.Second)
	consumeMessage(t, "d2", sm,
		newDismissalByCategory(u1, makeMsgID(), nil, c2, timeToTimeOrOffset(tm5)),
	)
	assert6 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertBodiesInCategory(t, sm, u1, nil, too, c1, []string{"f2"})
		assertBodiesInCategory(t, sm, u1, nil, too, c2, []string{"b4"})
	}
	assert6(nil)

	// Now ask for a dump of all messages sent since the beginning of time.
	// We make an optimization that we don't bother to return messages
	// that have since expired or been dismissed. So we expect the
	// two undismissed/unexpired messages above, and the two dismissals themselves.
	msgs, err := sm.InBandMessagesSince(u1, nil, timeToTimeOrOffset(t0))
	require.Nil(t, err, "no error from InBandMessagesSince")
	require.Equal(t, 4, len(msgs), "expected 4 messages")
	msgIDsToDismiss := msgs[0].ToStateUpdateMessage().Dismissal().MsgIDsToDismiss()
	require.Equal(t, 1, len(msgIDsToDismiss), "only 1 msgID to dismiss")
	require.Equal(t, m1, msgIDsToDismiss[0], "msg m1 update")
	require.Equal(t, []byte("f2"), msgs[1].ToStateUpdateMessage().Creation().Body().Bytes(), "body 1")
	require.Equal(t, []byte("b4"), msgs[2].ToStateUpdateMessage().Creation().Body().Bytes(), "body 1")
	rangesToDismiss := msgs[3].ToStateUpdateMessage().Dismissal().RangesToDismiss()
	require.Equal(t, 1, len(rangesToDismiss), "only 1 msg range to dismiss")
	require.Equal(t, c2.String(), rangesToDismiss[0].Category().String(), "the right category")
	require.Equal(t, tm5.UnixNano(), rangesToDismiss[0].EndTime().Time().UnixNano(), "the right dismissal time")
}

func TestStateMachinePerDevice(t *testing.T, sm gregor.StateMachine, fc clockwork.FakeClock) {
	u1 := makeUID()
	c1 := testCategory("foos")
	d1 := makeDeviceID()
	assert1 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	assert1(nil)
	m1 := makeMsgID()
	sm.ConsumeMessage(
		newCreation(u1, m1, d1, c1, "f1", nil),
	)
	assert2 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, d1, too, 1)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
	}
	assert2(nil)
	m2 := makeMsgID()
	d2 := makeDeviceID()
	sm.ConsumeMessage(
		newCreation(u1, m2, d2, c1, "f2", nil),
	)
	assert3 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert3(nil)
	sm.ConsumeMessage(
		newDismissalByIDs(u1, makeMsgID(), nil, []gregor.MsgID{m1}),
	)
	assert4 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert4(nil)

	// Make sure that "global" notifications are all picked-up if querying per-device;
	// Advance the clock so that they will be ordered consistently.
	m3 := makeMsgID()
	fc.Advance(time.Duration(1) * time.Second)
	sm.ConsumeMessage(
		newCreation(u1, m3, nil, c1, "f3", nil),
	)
	assert5 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f3"})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2", "f3"})
	}
	assert5(nil)
}
