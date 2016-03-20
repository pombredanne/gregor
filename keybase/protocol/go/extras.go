package gregor1

import (
	"encoding/hex"
	"errors"
	"fmt"
	keybase1 "github.com/keybase/client/go/protocol"
	"github.com/keybase/gregor"
	"time"
)

func (u UID) Bytes() []byte {
	b, err := hex.DecodeString(string(u))
	if err != nil {
		return nil
	}
	return b
}

func (m MsgID) Bytes() []byte          { return []byte(m) }
func (d DeviceID) Bytes() []byte       { return []byte(d) }
func (s System) String() string        { return string(s) }
func (c Category) String() string      { return string(c) }
func (b Body) Bytes() []byte           { return []byte(b) }
func (c Category) Eq(c2 Category) bool { return string(c) == string(c2) }

func (t TimeOrOffset) Time() *time.Time {
	if t.Time_.IsZero() {
		return nil
	}
	ret := keybase1.FromTime(t.Time_)
	return &ret
}

func (t TimeOrOffset) Offset() *time.Duration {
	if t.Offset_ == 0 {
		return nil
	}
	d := time.Duration(t.Offset_) * time.Millisecond
	return &d
}

func (s StateSyncMessage) Metadata() gregor.Metadata {
	return s.Md
}

func (m MsgRange) EndTime() gregor.TimeOrOffset {
	return m.EndTime_
}

func (m MsgRange) Category() gregor.Category {
	return m.Category_
}

func (d Dismissal) RangesToDismiss() []gregor.MsgRange {
	var ret []gregor.MsgRange
	for _, r := range d.Ranges {
		ret = append(ret, r)
	}
	return ret
}

func (d Dismissal) MsgIDsToDismiss() []gregor.MsgID {
	var ret []gregor.MsgID
	for _, m := range d.MsgIDs {
		ret = append(ret, m)
	}
	return ret
}

type ItemAndMetadata struct {
	md *Metadata
	i  *Item
}

func (m Metadata) UID() gregor.UID                   { return m.Uid }
func (i ItemAndMetadata) Metadata() gregor.Metadata  { return *i.md }
func (i ItemAndMetadata) Body() gregor.Body          { return i.i.Body }
func (i ItemAndMetadata) Category() gregor.Category  { return i.i.Category }
func (i ItemAndMetadata) DTime() gregor.TimeOrOffset { return i.i.Dtime }
func (i ItemAndMetadata) NotifyTimes() []gregor.TimeOrOffset {
	var ret []gregor.TimeOrOffset
	for _, t := range i.i.NotifyTimes {
		ret = append(ret, t)
	}
	return ret
}

func (s StateUpdateMessage) Metadata() gregor.Metadata { return s.Md }
func (s StateUpdateMessage) Creation() gregor.Item {
	if s.Creation_ != nil {
		return nil
	}
	return ItemAndMetadata{md: &s.Md, i: s.Creation_}
}
func (s StateUpdateMessage) Dismissal() gregor.Dismissal {
	if s.Dismissal_ != nil {
		return nil
	}
	return s.Dismissal_
}

func (i InbandMessage) Merge(i2 gregor.InbandMessage) error {
	t2, ok := i2.(InbandMessage)
	if !ok {
		return fmt.Errorf("bad merge; wrong type: %T", i2)
	}
	if i.StateSync != nil || t2.StateSync != nil {
		return errors.New("Cannot merge sync messages")
	}
	return i.StateUpdate.Merge(t2.StateUpdate)
}

func (s StateUpdateMessage) Merge(s2 *StateUpdateMessage) error {
	if s.Creation_ != nil && s2.Creation_ != nil {
		return errors.New("clash of creations")
	}
	if s.Creation_ == nil {
		s.Creation_ = s2.Creation_
	}
	if s.Dismissal_ == nil {
		s.Dismissal_ = s2.Dismissal_
	} else if s.Dismissal_ != nil {
		s.Dismissal_.MsgIDs = append(s.Dismissal_.MsgIDs, s2.Dismissal_.MsgIDs...)
		s.Dismissal_.Ranges = append(s.Dismissal_.Ranges, s2.Dismissal_.Ranges...)
	}
	return nil
}

func (i InbandMessage) Metadata() gregor.Metadata {
	if i.StateUpdate != nil {
		return i.StateUpdate.Md
	}
	if i.StateSync != nil {
		return i.StateSync.Md
	}
	return nil
}

func (i InbandMessage) ToStateSyncMessage() gregor.StateSyncMessage {
	if i.StateSync == nil {
		return nil
	}
	return i.StateSync
}

func (i InbandMessage) ToStateUpdateMessage() gregor.StateUpdateMessage {
	if i.StateUpdate == nil {
		return nil
	}
	return i.StateUpdate
}

func (m Metadata) MsgID() gregor.MsgID                 { return m.MsgID_ }
func (m Metadata) CTime() gregor.TimeOrOffset          { return m.Ctime }
func (m Metadata) DeviceID() gregor.DeviceID           { return m.DeviceID_ }
func (m Metadata) InbandMsgType() gregor.InbandMsgType { return gregor.InbandMsgType(m.InbandMsgType_) }

func (o OutOfBandMessage) Body() gregor.Body     { return o.Body_ }
func (o OutOfBandMessage) System() gregor.System { return o.System_ }
func (o OutOfBandMessage) UID() gregor.UID       { return o.Uid }

func (m Message) ToInbandMessage() gregor.InbandMessage {
	if m.Ibm == nil {
		return nil
	}
	return m.Ibm
}

func (m Message) ToOutOfBandMessage() gregor.OutOfBandMessage {
	if m.Oobm != nil {
		return nil
	}
	return m.Oobm
}

type State struct {
	items []ItemAndMetadata
}

func (s State) Items() ([]gregor.Item, error) {
	var ret []gregor.Item
	for _, i := range s.items {
		ret = append(ret, i)
	}
	return ret, nil
}

func (i ItemAndMetadata) InCategory(c Category) bool {
	return i.i.Category.Eq(c)
}

func (s State) ItemsInCategory(gc gregor.Category) ([]gregor.Item, error) {
	var ret []gregor.Item
	c := Category(gc.String())
	for _, i := range s.items {
		if i.InCategory(c) {
			ret = append(ret, i)
		}
	}
	return ret, nil
}

var _ gregor.UID = UID("")
var _ gregor.MsgID = MsgID{}
var _ gregor.DeviceID = DeviceID{}
var _ gregor.System = System("")
var _ gregor.Body = Body{}
var _ gregor.Category = Category("")
var _ gregor.TimeOrOffset = TimeOrOffset{}
var _ gregor.Metadata = Metadata{}
var _ gregor.StateSyncMessage = StateSyncMessage{}
var _ gregor.MsgRange = MsgRange{}
var _ gregor.Dismissal = Dismissal{}
var _ gregor.Item = ItemAndMetadata{}
var _ gregor.StateUpdateMessage = StateUpdateMessage{}
var _ gregor.InbandMessage = InbandMessage{}
var _ gregor.OutOfBandMessage = OutOfBandMessage{}
var _ gregor.Message = Message{}
var _ gregor.State = State{}
