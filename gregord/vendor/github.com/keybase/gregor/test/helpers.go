package test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	gregor "github.com/keybase/gregor"
	"github.com/stretchr/testify/require"
)

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
	require.Len(t, it, len(expected), "wrong number of items")
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

func makeUID(of gregor.ObjFactory) gregor.UID {
	uid, err := of.MakeUID(randBytes(16))
	if err != nil {
		panic(err)
	}
	return uid
}
func makeMsgID(of gregor.ObjFactory) gregor.MsgID {
	msgid, err := of.MakeMsgID(randBytes(16))
	if err != nil {
		panic(err)
	}
	return msgid
}
func makeDeviceID(of gregor.ObjFactory) gregor.DeviceID {
	deviceid, err := of.MakeDeviceID(randBytes(16))
	if err != nil {
		panic(err)
	}
	return deviceid
}
func makeCategory(of gregor.ObjFactory, s string) gregor.Category {
	c, err := of.MakeCategory(s)
	if err != nil {
		panic(err)
	}
	return c
}

func timeToTimeOrOffset(of gregor.ObjFactory, t time.Time) gregor.TimeOrOffset {
	ret, err := of.MakeTimeOrOffsetFromTime(t)
	if err != nil {
		panic(err)
	}
	return ret
}

func newCreation(of gregor.ObjFactory, u gregor.UID, m gregor.MsgID, d gregor.DeviceID, ctime time.Time, c gregor.Category, data string, dtime *time.Time) gregor.Message {
	b, err := of.MakeBody([]byte(data))
	if err != nil {
		panic(err)
	}
	i, err := of.MakeItem(u, m, d, ctime, c, dtime, b)
	if err != nil {
		panic(err)
	}
	ibmsg, err := of.MakeInBandMessageFromItem(i)
	if err != nil {
		panic(err)
	}
	msg, err := of.MakeMessageFromInBandMessage(ibmsg)
	if err != nil {
		panic(err)
	}
	return msg
}

func newDismissalByIDs(of gregor.ObjFactory, u gregor.UID, m gregor.MsgID, d gregor.DeviceID, ctime time.Time, ids []gregor.MsgID) gregor.Message {
	ret, err := of.MakeDismissalByIDs(u, m, d, ctime, ids)
	if err != nil {
		panic(err)
	}
	msg, err := of.MakeMessageFromInBandMessage(ret)
	if err != nil {
		panic(err)
	}
	return msg
}

func newDismissalByCategory(of gregor.ObjFactory, u gregor.UID, m gregor.MsgID, d gregor.DeviceID, ctime time.Time, c gregor.Category, dt time.Time) gregor.Message {
	ret, err := of.MakeDismissalByRange(u, m, d, ctime, c, dt)
	if err != nil {
		panic(err)
	}
	msg, err := of.MakeMessageFromInBandMessage(ret)
	if err != nil {
		panic(err)
	}
	return msg
}

func consumeMessage(t *testing.T, which string, sm gregor.StateMachine, m gregor.Message) {
	err := sm.ConsumeMessage(m)
	if err != nil {
		t.Fatalf("In inserting msg %s: %v", which, err)
	}
}

// advanceClock advances the given clock if it's a FakeClock.
func advanceClock(cl clockwork.Clock, d time.Duration) {
	if fc, ok := cl.(clockwork.FakeClock); ok {
		fc.Advance(d)
	}
}

func TestStateMachineAllDevices(t *testing.T, sm gregor.StateMachine) gregor.UID {
	of := sm.ObjFactory()
	cl := sm.Clock()
	t0 := cl.Now()
	u1 := makeUID(of)
	c1 := makeCategory(of, "foos")
	c2 := makeCategory(of, "bars")

	// Make an assertion: that there are no items total in the StateMachine,
	// and no items at in the category 'foos'
	assert1 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}

	// Do the assertion for "now" = nil
	assert1(nil)

	// Produce a new mesasge, with payload "f1"
	m1 := makeMsgID(of)
	consumeMessage(t, "m1", sm,
		newCreation(of, u1, m1, nil, cl.Now(), c1, "f1", nil),
	)
	advanceClock(cl, time.Second)
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
	consumeMessage(t, "d1", sm, newDismissalByIDs(of, u1, makeMsgID(of), nil, cl.Now(), []gregor.MsgID{m1}))

	// Make an assertion: that there are no items left in the StateMachine
	assert3 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	tm3 := cl.Now()

	// Do the assertion for "now" = nil
	assert3(nil)
	advanceClock(cl, time.Second)

	// Make and consume 3 new messages.
	consumeMessage(t, "m2", sm,
		newCreation(of, u1, makeMsgID(of), nil, cl.Now(), c1, "f2", nil),
	)
	dt4 := cl.Now().Add(3 * time.Second)
	consumeMessage(t, "m3", sm,
		newCreation(of, u1, makeMsgID(of), nil, cl.Now(), c1, "f3", &dt4),
	)
	consumeMessage(t, "m4", sm,
		newCreation(of, u1, makeMsgID(of), nil, cl.Now(), c2, "b1", nil),
	)

	// Make an assertion: that the items wound up and in the right
	// categories.
	assert4 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 3)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c2, 1)
	}
	assert4(nil)
	tm4 := cl.Now()
	advanceClock(cl, 4*time.Second)

	// Make an assertion: that the dismissals worked as planned
	assert5 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 1)
		assertBodiesInCategory(t, sm, u1, nil, too, c1, []string{"f2"})
		assertBodiesInCategory(t, sm, u1, nil, too, c2, []string{"b1"})
	}
	tm5 := cl.Now()
	assert5(nil)

	// Assert our previous checkpoint still works
	assert3(timeToTimeOrOffset(of, tm3))
	assert4(timeToTimeOrOffset(of, tm4))

	consumeMessage(t, "m5", sm,
		newCreation(of, u1, makeMsgID(of), nil, cl.Now(), c2, "b3", nil),
	)
	advanceClock(cl, 4*time.Second)
	consumeMessage(t, "m6", sm,
		newCreation(of, u1, makeMsgID(of), nil, cl.Now(), c2, "b4", nil),
	)
	advanceClock(cl, time.Second)
	consumeMessage(t, "d2", sm,
		newDismissalByCategory(of, u1, makeMsgID(of), nil, cl.Now(), c2, tm5),
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
	msgs, err := sm.InBandMessagesSince(u1, nil, t0)
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

	return u1
}

func TestStateMachinePerDevice(t *testing.T, sm gregor.StateMachine) (gregor.UID, gregor.DeviceID) {
	of := sm.ObjFactory()
	cl := sm.Clock()
	u1 := makeUID(of)
	c1 := makeCategory(of, "foos")
	d1 := makeDeviceID(of)
	assert1 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 0)
		assertNItemsInCategory(t, sm, u1, nil, too, c1, 0)
	}
	assert1(nil)
	m1 := makeMsgID(of)
	sm.ConsumeMessage(
		newCreation(of, u1, m1, d1, cl.Now(), c1, "f1", nil),
	)
	assert2 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, d1, too, 1)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
	}
	assert2(nil)
	m2 := makeMsgID(of)
	d2 := makeDeviceID(of)
	sm.ConsumeMessage(
		newCreation(of, u1, m2, d2, cl.Now(), c1, "f2", nil),
	)
	assert3 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f1"})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert3(nil)
	sm.ConsumeMessage(
		newDismissalByIDs(of, u1, makeMsgID(of), nil, cl.Now(), []gregor.MsgID{m1}),
	)
	assert4 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 1)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2"})
	}
	assert4(nil)

	// Make sure that "global" notifications are all picked-up if querying per-device;
	// Advance the clock so that they will be ordered consistently.
	m3 := makeMsgID(of)
	advanceClock(cl, time.Second)
	sm.ConsumeMessage(
		newCreation(of, u1, m3, nil, cl.Now(), c1, "f3", nil),
	)
	assert5 := func(too gregor.TimeOrOffset) {
		assertNItems(t, sm, u1, nil, too, 2)
		assertBodiesInCategory(t, sm, u1, d1, too, c1, []string{"f3"})
		assertBodiesInCategory(t, sm, u1, d2, too, c1, []string{"f2", "f3"})
	}
	assert5(nil)

	return u1, d1
}

func AddStateMachinePerDevice(sm gregor.StateMachine, u gregor.UID, d gregor.DeviceID) {
	of := sm.ObjFactory()
	cl := sm.Clock()
	c1 := makeCategory(of, "bars")
	sm.ConsumeMessage(
		newCreation(of, u, makeMsgID(of), d, cl.Now(), c1, "b1", nil),
	)
	advanceClock(cl, time.Second)
	sm.ConsumeMessage(
		newCreation(of, u, makeMsgID(of), d, cl.Now(), c1, "b2", nil),
	)
	advanceClock(cl, time.Second)
	sm.ConsumeMessage(
		newCreation(of, u, makeMsgID(of), d, cl.Now(), c1, "b3", nil),
	)
}
