package test

import (
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	base "github.com/keybase/gregor"
)

type testUID []byte
type testMsgID []byte
type testDeviceID []byte
type testSystem string
type testCategory string

type testItem struct {
	m      *testMetadata
	dtime  base.TimeOrOffset
	nTimes []base.TimeOrOffset
	cat    base.Category
	body   base.Body
}

type testTimeOrOffset struct {
	t *time.Time
	d *time.Duration
}

type testMsgRange struct {
	e base.TimeOrOffset
	c base.Category
}

type testDismissal struct {
	ids    []base.MsgID
	ranges []testMsgRange
}

type testMetadata struct {
	u base.UID
	m base.MsgID
	d base.DeviceID
	t base.TimeOrOffset
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
type testState []base.Item

func (f TestObjFactory) MakeUID(b []byte) (base.UID, error) {
	return testUID(b), nil
}
func (f TestObjFactory) MakeMsgID(b []byte) (base.MsgID, error) {
	return testMsgID(b), nil
}
func (f TestObjFactory) MakeDeviceID(b []byte) (base.DeviceID, error) {
	return testDeviceID(b), nil
}
func (f TestObjFactory) MakeBody(b []byte) (base.Body, error) {
	return testBody(b), nil
}
func (f TestObjFactory) MakeCategory(s string) (base.Category, error) {
	return testCategory(s), nil
}
func (f TestObjFactory) MakeItem(u base.UID, msgid base.MsgID, deviceid base.DeviceID, ctime time.Time, c base.Category, dtime *time.Time, body base.Body) (base.Item, error) {
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
func (f TestObjFactory) MakeDismissalByRange(uid base.UID, msgid base.MsgID, devid base.DeviceID, ctime time.Time, c base.Category, d time.Time) (base.InBandMessage, error) {
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

func (f TestObjFactory) MakeDismissalByID(uid base.UID, msgid base.MsgID, devid base.DeviceID, ctime time.Time, d base.MsgID) (base.InBandMessage, error) {
	md := newTestMetadata(uid, msgid, devid, ctime)
	td := &testDismissal{
		ids: []base.MsgID{d},
	}
	return &testInBandMessage{m: md, d: td}, nil
}

func (f TestObjFactory) MakeStateSyncMessage(uid base.UID, msgid base.MsgID, devid base.DeviceID, ctime time.Time) (base.InBandMessage, error) {
	md := newTestMetadata(uid, msgid, devid, ctime)
	return &testInBandMessage{m: md, s: (*testSyncMessage)(md)}, nil
}

func (f TestObjFactory) MakeState(i []base.Item) (base.State, error) {
	return testState(i), nil
}
func (f TestObjFactory) MakeMetadata(uid base.UID, msgid base.MsgID, devid base.DeviceID, ctime time.Time, i base.InBandMsgType) (base.Metadata, error) {
	return newTestMetadata(uid, msgid, devid, ctime), nil
}

var errBadType = errors.New("bad type in cast")

func (f TestObjFactory) MakeInBandMessageFromItem(i base.Item) (base.InBandMessage, error) {
	ti, ok := i.(*testItem)
	if !ok {
		return nil, errBadType
	}
	return &testInBandMessage{m: ti.m, i: ti}, nil
}

func newTestMetadata(u base.UID, msgid base.MsgID, devid base.DeviceID, ctime time.Time) *testMetadata {
	return &testMetadata{
		u: u, m: msgid, d: devid, t: timeToTimeOrOffset(ctime),
	}
}

var _ base.ObjFactory = TestObjFactory{}

type testBody string

func (t testBody) Bytes() []byte { return []byte(t) }

func (t *testMetadata) MsgID() base.MsgID                 { return t.m }
func (t *testMetadata) CTime() base.TimeOrOffset          { return t.t }
func (t *testMetadata) DeviceID() base.DeviceID           { return t.d }
func (t *testMetadata) UID() base.UID                     { return t.u }
func (t *testMetadata) InBandMsgType() base.InBandMsgType { return base.InBandMsgTypeUpdate }

func (t *testItem) DTime() base.TimeOrOffset         { return t.dtime }
func (t *testItem) NotifyTimes() []base.TimeOrOffset { return t.nTimes }
func (t *testItem) Body() base.Body                  { return t.body }
func (t *testItem) Category() base.Category          { return t.cat }
func (t *testItem) Metadata() base.Metadata          { return t.m }

func (t testMsgRange) EndTime() base.TimeOrOffset { return t.e }
func (t testMsgRange) Category() base.Category    { return t.c }

func (t *testSyncMessage) Metadata() base.Metadata { return (*testMetadata)(t) }

func (t testInBandMessage) Metadata() base.Metadata                       { return t.m }
func (t testInBandMessage) ToStateSyncMessage() base.StateSyncMessage     { return t.s }
func (t testInBandMessage) ToStateUpdateMessage() base.StateUpdateMessage { return t }

func (t testInBandMessage) Dismissal() base.Dismissal {
	if t.d == nil {
		return nil
	}
	return t.d
}
func (t testInBandMessage) Creation() base.Item {
	if t.i == nil {
		return nil
	}
	return t.i
}

func (t testInBandMessage) Merge(m2 base.InBandMessage) error {
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

func (t *testDismissal) MsgIDsToDismiss() []base.MsgID { return t.ids }

func (t testTimeOrOffset) Time() *time.Time       { return t.t }
func (t testTimeOrOffset) Offset() *time.Duration { return t.d }
func (t testUID) Bytes() []byte                   { return t }
func (t testMsgID) Bytes() []byte                 { return t }
func (t testDeviceID) Bytes() []byte              { return t }
func (t testCategory) String() string             { return string(t) }
func (t testSystem) String() string               { return string(t) }

func (m testMessage) ToInBandMessage() base.InBandMessage       { return m.i }
func (m testMessage) ToOutOfBandMessage() base.OutOfBandMessage { return nil }

func (t *testDismissal) RangesToDismiss() []base.MsgRange {
	var ret []base.MsgRange
	for _, r := range t.ranges {
		ret = append(ret, base.MsgRange(r))
	}
	return ret
}

func (t testState) Items() ([]base.Item, error) {
	return []base.Item(t), nil
}

func (t testState) ItemsInCategory(c base.Category) ([]base.Item, error) {
	var ret []base.Item
	for _, item := range t {
		if item.Category().String() == c.String() {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

var _ base.Item = (*testItem)(nil)
var _ base.InBandMessage = testInBandMessage{}

func assertNItems(t *testing.T, sm base.StateMachine, u base.UID, d base.DeviceID, too base.TimeOrOffset, n int) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.Items()
	require.Nil(t, err, "no error from Items()")
	require.Equal(t, n, len(it), "wrong number of items")
}

func assertNItemsInCategory(t *testing.T, sm base.StateMachine, u base.UID, d base.DeviceID, too base.TimeOrOffset, c base.Category, n int) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.ItemsInCategory(c)
	require.Nil(t, err, "no error from ItemsInCategory()")
	require.Equal(t, n, len(it), "wrong number of items")
}
func assertBodiesInCategory(t *testing.T, sm base.StateMachine, u base.UID, d base.DeviceID, too base.TimeOrOffset, c base.Category, v []string) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.ItemsInCategory(c)
	require.Nil(t, err, "no error from ItemsInCategory()")
	require.Equal(t, len(v), len(it), "wrong number of items")
	for i, p := range it {
		require.Equal(t, []byte(v[i]), p.Body().Bytes())
	}
}

func randBytes(n int) []byte {
	ret := make([]byte, n)
	rand.Read(ret)
	return ret
}

func makeUID() base.UID           { return testUID(randBytes(8)) }
func makeMsgID() base.MsgID       { return testMsgID(randBytes(8)) }
func makeDeviceID() base.DeviceID { return testDeviceID(randBytes(8)) }
func makeOffset(i int) base.TimeOrOffset {
	d := time.Second * time.Duration(i)
	return testTimeOrOffset{d: &d}
}
func timeToTimeOrOffset(t time.Time) base.TimeOrOffset {
	return testTimeOrOffset{t: &t}
}

func newCreation(u base.UID, m base.MsgID, d base.DeviceID, c base.Category, data string, dtime base.TimeOrOffset) base.Message {
	md := &testMetadata{u: u, m: m, d: d}
	item := &testItem{m: md, dtime: dtime, body: testBody(data), cat: c}
	return testMessage{i: &testInBandMessage{m: md, i: item}}
}

func newDismissalByIDs(u base.UID, m base.MsgID, d base.DeviceID, ids []base.MsgID) base.Message {
	md := &testMetadata{u: u, m: m, d: d}
	dismissal := &testDismissal{ids: ids}
	ret := testMessage{i: &testInBandMessage{m: md, d: dismissal}}
	return ret
}

func newDismissalByCategory(u base.UID, m base.MsgID, d base.DeviceID, c base.Category, e base.TimeOrOffset) base.Message {
	md := &testMetadata{u: u, m: m, d: d}
	dismissal := &testDismissal{ranges: []testMsgRange{{c: c, e: e}}}
	ret := testMessage{i: &testInBandMessage{m: md, d: dismissal}}
	return ret
}

func consumeMessage(t *testing.T, which string, sm base.StateMachine, m base.Message) {
	err := sm.ConsumeMessage(m)
	if err != nil {
		t.Fatalf("In inserting msg %s: %v", which, err)
	}
}

func TestStateMachineAllDevices(t *testing.T, sm base.StateMachine, fc clockwork.FakeClock) {
	t0 := fc.Now()
	u1 := makeUID()
	c1 := testCategory("foos")
	c2 := testCategory("bars")

	// Make an assertion: that there are no items total in the StateMachine,
	// and no items at in the category 'foos'
	assert1 := func(too base.TimeOrOffset) {
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
	assert2 := func(too base.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 1)
	}

	// Do the assertion for "now" = nil
	assert2(nil)

	// Produce a new *dismissal* message that dismisses the first message
	// we added (m1)
	consumeMessage(t, "d1", sm, newDismissalByIDs(u1, makeMsgID(), nil, []base.MsgID{m1}))

	// Make an assertion: that there are no items left in the StateMachine
	assert3 := func(too base.TimeOrOffset) {
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
	assert4 := func(too base.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 3)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c2, 1)
	}
	assert4(nil)
	tm4 := fc.Now()
	fc.Advance(time.Duration(4) * time.Second)

	// Make an assertion: that the dismissals worked as planned
	assert5 := func(too base.TimeOrOffset) {
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
	assert6 := func(too base.TimeOrOffset) {
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

func TestStateMachinePerDevice(t *testing.T, sm base.StateMachine, fc clockwork.FakeClock) {
	u1 := makeUID()
	c1 := testCategory("foos")
	d1 := makeDeviceID()
	assert1 := func(too base.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	assert1(nil)
	m1 := makeMsgID()
	sm.ConsumeMessage(
		newCreation(u1, m1, d1, c1, "f1", nil),
	)
	assert2 := func(too base.TimeOrOffset) {
		assertNItems(t, sm, u1, d1, too, 1)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
	}
	assert2(nil)
	m2 := makeMsgID()
	d2 := makeDeviceID()
	sm.ConsumeMessage(
		newCreation(u1, m2, d2, c1, "f2", nil),
	)
	assert3 := func(too base.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert3(nil)
	sm.ConsumeMessage(
		newDismissalByIDs(u1, makeMsgID(), nil, []base.MsgID{m1}),
	)
	assert4 := func(too base.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert4(nil)
}
