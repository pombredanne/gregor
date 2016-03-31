package storage

import (
	"bytes"
	"encoding/hex"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	gregor "github.com/keybase/gregor"
)

// MemEngine is an implementation of a gregor StateMachine that just keeps
// all incoming messages in a hash table, with one entry per user. It doesn't
// do anything fancy w/r/t indexing Items, so just iterates over all of them
// every time a dismissal or a state dump comes in. Used mainly for testing
// when SQLite isn't available.
type MemEngine struct {
	sync.Mutex
	objFactory gregor.ObjFactory
	clock      clockwork.Clock
	users      map[string](*user)
}

// NewMemEngine makes a new MemEngine with the given object factory and the
// potentially fake clock (or a real clock if not testing).
func NewMemEngine(f gregor.ObjFactory, cl clockwork.Clock) *MemEngine {
	return &MemEngine{
		objFactory: f,
		clock:      cl,
		users:      make(map[string](*user)),
	}
}

var _ gregor.StateMachine = (*MemEngine)(nil)

// item is a wrapper around a Gregor item interface, with the ctime
// it arrived at, and the optional dtime at which it was dismissed. Note there's
// another Dtime internal to item that can be interpreted relative to the ctime
// of the wrapper object.
type item struct {
	item  gregor.Item
	ctime time.Time
	dtime *time.Time
}

// loggedMsg is a message that we've logged on arrival into this state machine
// store. When it comes in, we stamp it with the current time, and also associate
// it with an item if there's one to speak of.
type loggedMsg struct {
	m     gregor.InBandMessage
	ctime time.Time
	i     *item
}

// user consists of a list of items (some of which might be dismissed) and
// and an unpruned log of all incoming messages.
type user struct {
	items [](*item)
	log   []loggedMsg
}

func newUser() *user {
	return &user{
		items: make([](*item), 0),
	}
}

// isDismissedAt returns true if item i is dismissed at time t
func (i item) isDismissedAt(t time.Time) bool {
	if i.dtime != nil && isBeforeOrSame(*i.dtime, t) {
		return true
	}
	if dt := i.item.DTime(); dt != nil && isBeforeOrSame(toTime(i.ctime, dt), t) {
		return true
	}
	return false
}

// isDismissedAt returns true if the log message has an associated item
// and that item was dismissed at time t.
func (m loggedMsg) isDismissedAt(t time.Time) bool {
	return m.i != nil && m.i.isDismissedAt(t)
}

// export the item i to a generic gregor.Item interface. Basically just return
// the object we got, but if there was no CTime() on the incoming message,
// then use the ctime we stamped on the message when it arrived.
func (i item) export(f gregor.ObjFactory) (gregor.Item, error) {
	md := i.item.Metadata()
	return f.MakeItem(md.UID(), md.MsgID(), md.DeviceID(), i.ctime, i.item.Category(), i.dtime, i.item.Body())
}

// addItem adds an item for this user
func (u *user) addItem(now time.Time, i gregor.Item) *item {
	newItem := &item{item: i, ctime: toTime(now, i.Metadata().CTime())}
	u.items = append(u.items, newItem)
	return newItem
}

// logMessage logs a message for this user and potentially associates an item
func (u *user) logMessage(t time.Time, m gregor.InBandMessage, i *item) {
	u.log = append(u.log, loggedMsg{m, t, i})
}

func msgIDtoString(m gregor.MsgID) string {
	return hex.EncodeToString(m.Bytes())
}

func (u *user) dismissMsgIDs(now time.Time, ids []gregor.MsgID) {
	set := make(map[string]struct{})
	for _, i := range ids {
		set[msgIDtoString(i)] = struct{}{}
	}
	for _, i := range u.items {
		if _, found := set[msgIDtoString(i.item.Metadata().MsgID())]; found {
			i.dtime = &now
		}
	}
}

func toTime(now time.Time, t gregor.TimeOrOffset) time.Time {
	if t == nil {
		return now
	}
	if t.Time() != nil {
		return *t.Time()
	}
	if t.Offset() != nil {
		return now.Add(*t.Offset())
	}
	return now
}

func (u *user) dismissRanges(now time.Time, rs []gregor.MsgRange) {
	for _, i := range u.items {
		for _, r := range rs {
			if r.Category().String() == i.item.Category().String() &&
				isBeforeOrSame(i.ctime, toTime(now, r.EndTime())) {
				i.dtime = &now
				break
			}
		}
	}
}

type timeOrOffset time.Time

func (t timeOrOffset) Time() *time.Time {
	ret := time.Time(t)
	return &ret
}
func (t timeOrOffset) Offset() *time.Duration { return nil }

var _ gregor.TimeOrOffset = timeOrOffset{}

func isBeforeOrSame(a, b time.Time) bool {
	return !b.Before(a)
}

func (u *user) state(now time.Time, f gregor.ObjFactory, d gregor.DeviceID, t gregor.TimeOrOffset) (gregor.State, error) {
	var items []gregor.Item
	if t == nil {
		t = timeOrOffset(now)
	}
	for _, i := range u.items {
		md := i.item.Metadata()
		did := md.DeviceID()
		if d != nil && did != nil && !bytes.Equal(did.Bytes(), d.Bytes()) {
			continue
		}
		if toTime(now, t).Before(i.ctime) {
			continue
		}
		if i.isDismissedAt(toTime(now, t)) {
			continue
		}
		exported, err := i.export(f)
		if err != nil {
			return nil, err
		}
		items = append(items, exported)
	}
	return f.MakeState(items)
}

func isMessageForDevice(m gregor.InBandMessage, d gregor.DeviceID) bool {
	sum := m.ToStateUpdateMessage()
	if sum == nil {
		return true
	}
	if d == nil {
		return true
	}
	did := sum.Metadata().DeviceID()
	if did == nil {
		return true
	}
	if bytes.Equal(did.Bytes(), d.Bytes()) {
		return true
	}
	return false
}

func (u *user) replayLog(now time.Time, d gregor.DeviceID, t gregor.TimeOrOffset) []gregor.InBandMessage {
	var ret []gregor.InBandMessage
	for _, msg := range u.log {
		if !isMessageForDevice(msg.m, d) {
			continue
		}
		if msg.ctime.Before(toTime(now, t)) {
			continue
		}
		if msg.isDismissedAt(now) {
			continue
		}

		ret = append(ret, msg.m)
	}
	return ret
}

func (m *MemEngine) consumeInBandMessage(uid gregor.UID, msg gregor.InBandMessage) error {
	user := m.getUser(uid)
	now := m.clock.Now()
	var i *item
	var err error
	switch {
	case msg.ToStateUpdateMessage() != nil:
		i, err = m.consumeStateUpdateMessage(user, now, msg.ToStateUpdateMessage())
	default:
	}
	user.logMessage(now, msg, i)
	return err
}

func (m *MemEngine) ConsumeMessage(msg gregor.Message) error {
	m.Lock()
	defer m.Unlock()

	switch {
	case msg.ToInBandMessage() != nil:
		return m.consumeInBandMessage(gregor.UIDFromMessage(msg), msg.ToInBandMessage())
	default:
		return nil
	}
}

func uidToString(u gregor.UID) string {
	return hex.EncodeToString(u.Bytes())
}

// getUser gets or makes a new user object for the given UID.
func (m *MemEngine) getUser(uid gregor.UID) *user {
	uidHex := uidToString(uid)
	if u, ok := m.users[uidHex]; ok {
		return u
	}
	u := newUser()
	m.users[uidHex] = u
	return u
}

func (m *MemEngine) consumeCreation(u *user, now time.Time, i gregor.Item) (*item, error) {
	newItem := u.addItem(now, i)
	return newItem, nil
}

func (m *MemEngine) consumeDismissal(u *user, now time.Time, d gregor.Dismissal) error {
	if ids := d.MsgIDsToDismiss(); ids != nil {
		if !d.CTime().IsZero() {
			now = d.CTime()
		}
		u.dismissMsgIDs(now, ids)
	}
	if r := d.RangesToDismiss(); r != nil {
		u.dismissRanges(now, r)
	}
	return nil
}

func (m *MemEngine) consumeStateUpdateMessage(u *user, now time.Time, msg gregor.StateUpdateMessage) (*item, error) {
	var err error
	var i *item
	if msg.Creation() != nil {
		if i, err = m.consumeCreation(u, now, msg.Creation()); err != nil {
			return nil, err
		}
	}
	if msg.Dismissal() != nil {
		if err = m.consumeDismissal(u, now, msg.Dismissal()); err != nil {
			return nil, err
		}
	}
	return i, nil
}

func (m *MemEngine) State(u gregor.UID, d gregor.DeviceID, t gregor.TimeOrOffset) (gregor.State, error) {
	m.Lock()
	defer m.Unlock()
	user := m.getUser(u)
	return user.state(m.clock.Now(), m.objFactory, d, t)
}

func (m *MemEngine) InBandMessagesSince(u gregor.UID, d gregor.DeviceID, t gregor.TimeOrOffset) ([]gregor.InBandMessage, error) {
	m.Lock()
	defer m.Unlock()
	user := m.getUser(u)
	msg := user.replayLog(m.clock.Now(), d, t)
	return msg, nil
}
