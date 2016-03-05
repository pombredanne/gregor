package message_broker

import (
	"time"
)

type UID interface {
	GetBytes() []byte
}

type MsgID interface {
	GetBytes() []byte
}

type DeviceID interface {
	GetBytes() []byte
}

type System interface {
	GetString() string
}

type Category interface {
	GetString() string
}

type Body interface {
	GetBytes() []byte
}

type Metadata interface {
	GetUID() UID
	GetMsgID() MsgID
	GetCTime() TimeOrOffset
	GetDeviceID() DeviceID
}

type InbandMessage interface {
	GetMetadata() Metadata
	GetCreation() Item
	GetDismissal() Dismissal
}

type OutOfBandMesasge interface {
	GetSystem() System
	GetUID() UID
	GetBody() Body
}

type TimeOrOffset interface {
	GetTime() *time.Time
	GetDuration() *time.Duration
}

type Item interface {
	GetMetadata() Metadata
	GetDTime() TimeOrOffset
	GetNotifyTimes() []TimeOrOffset
	GetBody() Body
	GetCategory() Category
}

type MsgRange interface {
	GetMetadata() Metadata
	GetEndTime() TimeOrOffset
	GetCategory() Category
}

type Dismissal interface {
	GetMetadata() Metadata
	GetMsgIDsToDismiss() []MsgID
	GetRangesToDismiss() []MsgRange
}

type State interface {
	GetItems() ([]Item, error)
	GetItemsInCategory(c Category) ([]Item, error)
}

type StateMachine interface {
	ConsumeInbandMessage(m InbandMessage) error
	ConsumeOutOfBandMessage(m OutOfBandMesasge) error
	GetState(u UID, d DeviceID, t TimeOrOffset) (State, error)
	GetInbandMessagesSince(u UID, d DeviceID, t TimeOrOffset) ([]InbandMessage, error)
}

type ObjFactory interface {
	MakeUID(b []byte) (UID, error)
	MakeMsgID(b []byte) (MsgID, error)
	MakeDeviceID(b []byte) (DeviceID, error)
	MakeBody(b []byte) (Body, error)
	MakeItem(msgid MsgID, category string, deviceid DeviceID, ctime time.Time, dtime *time.Time, body Body) (Item, error)
	MakeState(i []Item) (State, error)
}

type Server interface {
	BrodcastInbandMessage(m InbandMessage) error
	BrodcastOutOfBandMessage(m OutOfBandMesasge) error
	TriggerNotification(m InbandMessage) error
}
