package gregor1

import (
	"encoding/hex"
	"fmt"
	keybase1 "github.com/keybase/client/go/protocol"
	"github.com/keybase/gregor"
	"time"
)

type ObjFactory struct{}

func (o ObjFactory) MakeUID(b []byte) (gregor.UID, error)           { return UID(hex.EncodeToString(b)), nil }
func (o ObjFactory) MakeMsgID(b []byte) (gregor.MsgID, error)       { return MsgID(b), nil }
func (o ObjFactory) MakeDeviceID(b []byte) (gregor.DeviceID, error) { return DeviceID(b), nil }
func (o ObjFactory) MakeBody(b []byte) (gregor.Body, error)         { return Body(b), nil }
func (o ObjFactory) MakeCategory(s string) (gregor.Category, error) { return Category(s), nil }

func castUID(uid gregor.UID) (UID, error) {
	ret, ok := uid.(UID)
	if !ok {
		return UID(""), fmt.Errorf("Bad UID; wrong type")
	}
	return ret, nil
}

func castItem(i gregor.Item) (ItemAndMetadata, error) {
	ret, ok := i.(ItemAndMetadata)
	if !ok {
		return ItemAndMetadata{}, fmt.Errorf("Bad Item; wrong type")
	}
	return ret, nil
}

func castMsgID(msgid gregor.MsgID, e *error) MsgID {
	ret, ok := msgid.(MsgID)
	if !ok {
		*e = fmt.Errorf("Bad MsgID; wrong type")
	}
	return ret
}

func castDeviceID(d gregor.DeviceID, e *error) DeviceID {
	ret, ok := d.(DeviceID)
	if !ok {
		*e = fmt.Errorf("Bad MsgID; wrong type")
	}
	return ret
}

func timeToTimeOrOffset(timeIn *time.Time) TimeOrOffset {
	var timeOut keybase1.Time
	if timeIn != nil && !timeIn.IsZero() {
		timeOut = keybase1.ToTime(*timeIn)
	}
	return TimeOrOffset{
		Offset_: 0,
		Time_:   timeOut,
	}
}

func (o ObjFactory) makeMetadata(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time, i gregor.InbandMsgType) (Metadata, error) {
	uid2, e := castUID(uid)
	if e != nil {
		return Metadata{}, e
	}
	return Metadata{
		Uid:            uid2,
		MsgID_:         MsgID(msgid.Bytes()),
		Ctime:          timeToTimeOrOffset(&ctime),
		DeviceID_:      DeviceID(devid.Bytes()),
		InbandMsgType_: int(i),
	}, nil
}

func (o ObjFactory) makeItem(c gregor.Category, d *time.Time, b gregor.Body) (Item, error) {
	return Item{
		Dtime:    timeToTimeOrOffset(d),
		Category: Category(c.String()),
		Body:     Body(b.Bytes()),
	}, nil
}

func (o ObjFactory) MakeItem(u gregor.UID, msgid gregor.MsgID, deviceid gregor.DeviceID, ctime time.Time, c gregor.Category, dtime *time.Time, body gregor.Body) (gregor.Item, error) {
	md, err := o.makeMetadata(u, msgid, deviceid, ctime, gregor.InbandMsgTypeUpdate)
	if err != nil {
		return nil, err
	}
	item, err := o.makeItem(c, dtime, body)
	if err != nil {
		return nil, err
	}
	return ItemAndMetadata{
		md: &md,
		i:  &item,
	}, nil
}

func (o ObjFactory) MakeDismissalByRange(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time, c gregor.Category, d time.Time) (gregor.InbandMessage, error) {
	md, err := o.makeMetadata(uid, msgid, devid, ctime, gregor.InbandMsgTypeUpdate)
	if err != nil {
		return nil, err
	}
	return InbandMessage{
		StateUpdate: &StateUpdateMessage{
			Md: md,
			Dismissal_: &Dismissal{
				Ranges: []MsgRange{{
					EndTime_:  timeToTimeOrOffset(&d),
					Category_: Category(c.String()),
				}},
			},
		},
	}, nil
}

func (o ObjFactory) MakeDismissalByID(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time, d gregor.MsgID) (gregor.InbandMessage, error) {
	md, err := o.makeMetadata(uid, msgid, devid, ctime, gregor.InbandMsgTypeUpdate)
	if err != nil {
		return nil, err
	}
	return InbandMessage{
		StateUpdate: &StateUpdateMessage{
			Md: md,
			Dismissal_: &Dismissal{
				MsgIDs: []MsgID{MsgID(d.Bytes())},
			},
		},
	}, nil
}

func (o ObjFactory) MakeStateSyncMessage(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time) (gregor.InbandMessage, error) {
	md, err := o.makeMetadata(uid, msgid, devid, ctime, gregor.InbandMsgTypeUpdate)
	if err != nil {
		return nil, err
	}
	return InbandMessage{
		StateSync: &StateSyncMessage{
			Md: md,
		},
	}, nil
}

func (o ObjFactory) MakeState(items []gregor.Item) (gregor.State, error) {
	var ourItems []ItemAndMetadata
	for _, item := range items {
		ourItem, err := castItem(item)
		if err != nil {
			return nil, err
		}
		ourItems = append(ourItems, ourItem)
	}
	return State{items: ourItems}, nil
}

func (o ObjFactory) MakeMetadata(uid gregor.UID, msgid gregor.MsgID, devid gregor.DeviceID, ctime time.Time, i gregor.InbandMsgType) (gregor.Metadata, error) {
	return o.makeMetadata(uid, msgid, devid, ctime, gregor.InbandMsgTypeUpdate)
}

func (o ObjFactory) MakeInbandMessageFromItem(i gregor.Item) (gregor.InbandMessage, error) {
	ourItem, err := castItem(i)
	if err != nil {
		return nil, err
	}
	return InbandMessage{
		StateUpdate: &StateUpdateMessage{
			Md:        *ourItem.md,
			Creation_: ourItem.i,
		},
	}, nil
}

var _ gregor.ObjFactory = ObjFactory{}
