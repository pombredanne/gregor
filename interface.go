package gregor

import (
	context "golang.org/x/net/context"
	"time"
)

type InbandMsgType int

const (
	InbandMsgTypeNone   InbandMsgType = 0
	InbandMsgTypeUpdate InbandMsgType = 1
	InbandMsgTypeSync   InbandMsgType = 2
)

type UID interface {
	Bytes() []byte
}

type MsgID interface {
	Bytes() []byte
}

type DeviceID interface {
	Bytes() []byte
}

type System interface {
	String() string
}

type Category interface {
	String() string
}

type Body interface {
	Bytes() []byte
}

type Metadata interface {
	UID() UID
	MsgID() MsgID
	CTime() TimeOrOffset
	DeviceID() DeviceID
	InbandMsgType() InbandMsgType
}

type MessageWithMetadata interface {
	Metadata() Metadata
}

type InbandMessage interface {
	MessageWithMetadata
	ToStateUpdateMessage() StateUpdateMessage
	ToStateSyncMessage() StateSyncMessage
	Merge(m1 InbandMessage) error
}

type StateUpdateMessage interface {
	MessageWithMetadata
	Creation() Item
	Dismissal() Dismissal
}

type StateSyncMessage interface {
	MessageWithMetadata
}

type OutOfBandMessage interface {
	System() System
	UID() UID
	Body() Body
}

type TimeOrOffset interface {
	Time() *time.Time
	Offset() *time.Duration
}

type Item interface {
	MessageWithMetadata
	DTime() TimeOrOffset
	NotifyTimes() []TimeOrOffset
	Body() Body
	Category() Category
}

type MsgRange interface {
	EndTime() TimeOrOffset
	Category() Category
}

type Dismissal interface {
	MsgIDsToDismiss() []MsgID
	RangesToDismiss() []MsgRange
}

type State interface {
	Items() ([]Item, error)
	ItemsInCategory(c Category) ([]Item, error)
}

type Message interface {
	ToInbandMessage() InbandMessage
	ToOutOfBandMessage() OutOfBandMessage
}

type StateMachine interface {
	ConsumeMessage(m Message) error
	State(u UID, d DeviceID, t TimeOrOffset) (State, error)
	InbandMessagesSince(u UID, d DeviceID, t TimeOrOffset) ([]InbandMessage, error)
}

type ObjFactory interface {
	MakeUID(b []byte) (UID, error)
	MakeMsgID(b []byte) (MsgID, error)
	MakeDeviceID(b []byte) (DeviceID, error)
	MakeBody(b []byte) (Body, error)
	MakeCategory(s string) (Category, error)
	MakeItem(u UID, msgid MsgID, deviceid DeviceID, ctime time.Time, c Category, dtime *time.Time, body Body) (Item, error)
	MakeDismissalByRange(uid UID, msgid MsgID, devid DeviceID, ctime time.Time, c Category, d time.Time) (InbandMessage, error)
	MakeDismissalByID(uid UID, msgid MsgID, devid DeviceID, ctime time.Time, d MsgID) (InbandMessage, error)
	MakeStateSyncMessage(uid UID, msgid MsgID, devid DeviceID, ctime time.Time) (InbandMessage, error)
	MakeState(i []Item) (State, error)
	MakeMetadata(uid UID, msgid MsgID, devid DeviceID, ctime time.Time, i InbandMsgType) (Metadata, error)
	MakeInbandMessageFromItem(i Item) (InbandMessage, error)
}

type NetworkInterfaceIncoming interface {
	ConsumeMessage(c context.Context, m Message) error
}

type NetworkInterfaceOutgoing interface {
	BroadcastMessage(c context.Context, m Message) error
}

type NetworkInterface interface {
	NetworkInterfaceOutgoing
	Serve(i NetworkInterfaceIncoming) error
}

type Server interface {
	TriggerNotification(m InbandMessage) error
}

func UIDFromMessage(m Message) UID {
	if ibm := m.ToInbandMessage(); ibm != nil {
		if md := ibm.Metadata(); md != nil {
			return md.UID()
		}
	}
	if oobm := m.ToOutOfBandMessage(); oobm != nil {
		return oobm.UID()
	}
	return nil
}
