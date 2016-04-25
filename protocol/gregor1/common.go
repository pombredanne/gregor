// Auto-generated by avdl-compiler v1.3.3 (https://github.com/keybase/node-avdl-compiler)
//   Input file: gregor1/avdl/common.avdl

package gregor1

import (
	rpc "github.com/keybase/go-framed-msgpack-rpc"
)

type TimeOrOffset struct {
	Time_   Time         `codec:"time" json:"time"`
	Offset_ DurationMsec `codec:"offset" json:"offset"`
}

type Metadata struct {
	Uid_           UID      `codec:"uid" json:"uid"`
	MsgID_         MsgID    `codec:"msgID" json:"msgID"`
	Ctime_         Time     `codec:"ctime" json:"ctime"`
	DeviceID_      DeviceID `codec:"deviceID" json:"deviceID"`
	InBandMsgType_ int      `codec:"inBandMsgType" json:"inBandMsgType"`
}

type InBandMessage struct {
	StateUpdate_ *StateUpdateMessage `codec:"stateUpdate,omitempty" json:"stateUpdate,omitempty"`
	StateSync_   *StateSyncMessage   `codec:"stateSync,omitempty" json:"stateSync,omitempty"`
}

type StateUpdateMessage struct {
	Md_        Metadata   `codec:"md" json:"md"`
	Creation_  *Item      `codec:"creation,omitempty" json:"creation,omitempty"`
	Dismissal_ *Dismissal `codec:"dismissal,omitempty" json:"dismissal,omitempty"`
}

type StateSyncMessage struct {
	Md_ Metadata `codec:"md" json:"md"`
}

type MsgRange struct {
	EndTime_  TimeOrOffset `codec:"endTime" json:"endTime"`
	Category_ Category     `codec:"category" json:"category"`
}

type Dismissal struct {
	MsgIDs_ []MsgID    `codec:"msgIDs" json:"msgIDs"`
	Ranges_ []MsgRange `codec:"ranges" json:"ranges"`
}

type Item struct {
	Category_    Category       `codec:"category" json:"category"`
	Dtime_       TimeOrOffset   `codec:"dtime" json:"dtime"`
	RemindTimes_ []TimeOrOffset `codec:"remindTimes" json:"remindTimes"`
	Body_        Body           `codec:"body" json:"body"`
}

type ItemAndMetadata struct {
	Md_   *Metadata `codec:"md,omitempty" json:"md,omitempty"`
	Item_ *Item     `codec:"item,omitempty" json:"item,omitempty"`
}

type Reminder struct {
	Item_  ItemAndMetadata `codec:"item" json:"item"`
	Ntime_ Time            `codec:"ntime" json:"ntime"`
}

type OutOfBandMessage struct {
	Uid_    UID    `codec:"uid" json:"uid"`
	System_ System `codec:"system" json:"system"`
	Body_   Body   `codec:"body" json:"body"`
}

type Message struct {
	Oobm_ *OutOfBandMessage `codec:"oobm,omitempty" json:"oobm,omitempty"`
	Ibm_  *InBandMessage    `codec:"ibm,omitempty" json:"ibm,omitempty"`
}

type DurationMsec int64
type Category string
type System string
type UID []byte
type MsgID []byte
type DeviceID []byte
type Body []byte
type Time int64
type CommonInterface interface {
}

func CommonProtocol(i CommonInterface) rpc.Protocol {
	return rpc.Protocol{
		Name:    "gregor.1.common",
		Methods: map[string]rpc.ServeHandlerDescription{},
	}
}

type CommonClient struct {
	Cli rpc.GenericClient
}
