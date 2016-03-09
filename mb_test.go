package gregor

import (
	"crypto/rand"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type testUID []byte
type testMsgID []byte
type testDeviceID []byte
type testSystem string
type testCategory string

type testItem struct {
	m       *testMetadata
	dtime   TimeOrOffset
	nTimes  []TimeOrOffset
	cat     Category
	payload testBody
}

type testTimeOrOffset struct {
	t *time.Time
	d *time.Duration
}

type testMsgRange struct {
	m *testMetadata
	e testTimeOrOffset
	c Category
}

type testDismissal struct {
	m      *testMetadata
	ids    []MsgID
	ranges []testMsgRange
}

type testMetadata struct {
	u UID
	m MsgID
	d DeviceID
	t TimeOrOffset
}

type testInbandMessage struct {
	m *testMetadata
	i *testItem
	d *testDismissal
}

type testMessage struct {
	i *testInbandMessage
}

type testBody string

func (t testBody) Bytes() []byte { return []byte(t) }

func (t *testMetadata) MsgID() MsgID        { return t.m }
func (t *testMetadata) CTime() TimeOrOffset { return t.t }
func (t *testMetadata) DeviceID() DeviceID  { return t.d }
func (t *testMetadata) UID() UID            { return t.u }

func (t *testItem) DTime() TimeOrOffset         { return t.dtime }
func (t *testItem) NotifyTimes() []TimeOrOffset { return t.nTimes }
func (t *testItem) Body() Body                  { return t.payload }
func (t *testItem) Category() Category          { return t.cat }
func (t *testItem) Metadata() Metadata          { return t.m }

func (t testMsgRange) Metadata() Metadata    { return t.m }
func (t testMsgRange) EndTime() TimeOrOffset { return t.e }
func (t testMsgRange) Category() Category    { return t.c }

func (t testInbandMessage) Metadata() Metadata                       { return t.m }
func (t testInbandMessage) Creation() Item                           { return t.i }
func (t testInbandMessage) Dismissal() Dismissal                     { return t.d }
func (t testInbandMessage) ToStateSyncMessage() StateSyncMessage     { return nil }
func (t testInbandMessage) ToStateUpdateMessage() StateUpdateMessage { return t }

func (t *testDismissal) Metadata() Metadata       { return t.m }
func (t *testDismissal) MsgIDsToDismiss() []MsgID { return t.ids }

func (t testTimeOrOffset) Time() *time.Time         { return t.t }
func (t testTimeOrOffset) Duration() *time.Duration { return t.d }
func (t testUID) Bytes() []byte                     { return t }
func (t testMsgID) Bytes() []byte                   { return t }
func (t testDeviceID) Bytes() []byte                { return t }
func (t testCategory) String() string               { return string(t) }
func (t testSystem) String() string                 { return string(t) }

func (m testMessage) ToInbandMessage() InbandMessage       { return m.i }
func (m testMessage) ToOutOfBandMessage() OutOfBandMessage { return nil }

func (t *testDismissal) RangesToDismiss() []MsgRange {
	var ret []MsgRange
	for _, r := range t.ranges {
		ret = append(ret, MsgRange(r))
	}
	return ret
}

var _ Item = (*testItem)(nil)
var _ InbandMessage = testInbandMessage{}

func assertNItems(t *testing.T, sm StateMachine, u UID, d DeviceID, too TimeOrOffset, n int) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.Items()
	require.Nil(t, err, "no error from Items()")
	require.Equal(t, len(it), n, "wrong number of items")
}

func assertNItemsInCategory(t *testing.T, sm StateMachine, u UID, d DeviceID, too TimeOrOffset, c Category, n int) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.ItemsInCategory(c)
	require.Nil(t, err, "no error from ItemsInCategory()")
	require.Equal(t, len(it), n, "wrong number of items")
}
func assertPayloadsInCategory(t *testing.T, sm StateMachine, u UID, d DeviceID, too TimeOrOffset, c Category, v []string) {
	state, err := sm.State(u, d, too)
	require.Nil(t, err, "no error from State()")
	it, err := state.ItemsInCategory(c)
	require.Nil(t, err, "no error from ItemsInCategory()")
	require.Equal(t, len(it), len(v), "wrong number of items")
	for i, p := range it {
		require.Equal(t, []byte(v[i]), p.Body().Bytes())
	}
}

func randBytes(n int) []byte {
	ret := make([]byte, n)
	rand.Read(ret)
	return ret
}

func makeUID() UID           { return testUID(randBytes(8)) }
func makeMsgID() MsgID       { return testMsgID(randBytes(8)) }
func makeDeviceID() DeviceID { return testDeviceID(randBytes(8)) }
func makeOffset(i int) TimeOrOffset {
	d := time.Second * time.Duration(i)
	return testTimeOrOffset{d: &d}
}
func timeToTimeOrOffset(t time.Time) TimeOrOffset {
	return testTimeOrOffset{t: &t}
}

func newCreation(u UID, m MsgID, d DeviceID, c Category, data string, dtime TimeOrOffset) Message {
	md := &testMetadata{u: u, m: m, d: d}
	item := &testItem{m: md, dtime: dtime, payload: testBody(data)}
	return testMessage{i: &testInbandMessage{m: md, i: item}}
}

func newDismissalByIDs(u UID, m MsgID, d DeviceID, ids []MsgID) Message {
	md := &testMetadata{u: u, m: d, d: d}
	dismissal := &testDismissal{m: md, ids: ids}
	return testMessage{i: &testInbandMessage{m: md, d: dismissal}}
}

func testStateMachineAllDevices(t *testing.T, sm StateMachine, fc clockwork.FakeClock) {
	u1 := makeUID()
	c1 := testCategory("foos")
	c2 := testCategory("bars")
	assert1 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	assert1(nil)
	m1 := makeMsgID()
	sm.ConsumeMessage(
		newCreation(u1, m1, nil, c1, "f1", nil),
	)
	assert2 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 1)
	}
	assert2(nil)
	sm.ConsumeMessage(newDismissalByIDs(u1, makeMsgID(), nil, []MsgID{m1}))
	assert3 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	assert3(nil)
	tm3 := fc.Now()
	fc.Advance(time.Second)
	sm.ConsumeMessage(
		newCreation(u1, makeMsgID(), nil, c1, "f2", nil),
	)
	sm.ConsumeMessage(
		newCreation(u1, makeMsgID(), nil, c1, "f3", makeOffset(3)),
	)
	sm.ConsumeMessage(
		newCreation(u1, makeMsgID(), nil, c2, "b1", nil),
	)
	assert4 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 3)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c2, 1)
	}
	assert4(nil)
	tm4 := fc.Now()
	fc.Advance(time.Duration(4) * time.Second)
	assert5 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 1)
		assertPayloadsInCategory(t, sm, u1, nil, too, c1, []string{"f2"})
		assertPayloadsInCategory(t, sm, u1, nil, too, c2, []string{"b1"})
	}
	assert5(nil)
	// Assert our previous checkpoint still works
	assert3(timeToTimeOrOffset(tm3))
	assert4(timeToTimeOrOffset(tm4))
}

func testStateMachinePerDevice(t *testing.T, sm StateMachine, fc clockwork.FakeClock) {
	u1 := makeUID()
	c1 := testCategory("foos")
	d1 := makeDeviceID()
	assert1 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	assert1(nil)
	m1 := makeMsgID()
	sm.ConsumeMessage(
		newCreation(u1, m1, d1, c1, "f1", nil),
	)
	assert2 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, d1, too, 1)
		assertPayloadsInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
	}
	assert2(nil)
	m2 := makeMsgID()
	d2 := makeDeviceID()
	sm.ConsumeMessage(
		newCreation(u1, m2, d2, c1, "f1", nil),
	)
	assert3 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertPayloadsInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
		assertPayloadsInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert3(nil)
	sm.ConsumeMessage(
		newDismissalByIDs(u1, makeMsgID(), nil, []MsgID{m1}),
	)
	assert4 := func(too TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertPayloadsInCategory(t, sm, u1, d1, too, c1, []string{})
		assertPayloadsInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert4(nil)
}
