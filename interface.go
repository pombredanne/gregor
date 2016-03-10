package gregor

import (
	"time"
)

type InbandMsgType int

const (
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
	Duration() *time.Duration
}

type Item interface {
	Metadata() Metadata
	DTime() TimeOrOffset
	NotifyTimes() []TimeOrOffset
	Body() Body
	Category() Category
}

type MsgRange interface {
	Metadata() Metadata
	EndTime() TimeOrOffset
	Category() Category
}

type Dismissal interface {
	Metadata() Metadata
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
	MakeItem(msgid MsgID, category string, deviceid DeviceID, ctime time.Time, dtime *time.Time, body Body) (Item, error)
	MakeState(i []Item) (State, error)
	MakeMetadata(uid UID, msgid MsgID, devid DeviceID, ctime time.Time, i InbandMsgType) (Metadata, error)
	MakeInbandMessageFromItem(i Item) (InbandMessage, error)
}

type Server interface {
	BrodcastInbandMessage(m InbandMessage) error
	BrodcastOutOfBandMessage(m OutOfBandMessage) error
	TriggerNotification(m InbandMessage) error
}
