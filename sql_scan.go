package gregor

import (
	"encoding/hex"
	"errors"
)

var ErrBadScan = errors.New("bad scan of data type")

type uidScanner struct {
	o   ObjFactory
	uid UID
}

func (u uidScanner) UID() UID { return u.uid }

func scanHexToBytes(src interface{}) ([]byte, error) {
	if s, ok := src.(string); ok {
		b, err := hex.DecodeString(s)
		return b, err
	}
	return nil, ErrBadScan
}

func (u uidScanner) Scan(src interface{}) error {
	b, err := scanHexToBytes(src)
	if err != nil {
		return err
	}
	u.uid, err = u.o.MakeUID(b)
	return err
}

type deviceIDScanner struct {
	o        ObjFactory
	deviceID DeviceID
}

func (d deviceIDScanner) DeviceID() DeviceID { return d.deviceID }

func (d deviceIDScanner) Scan(src interface{}) error {
	b, err := scanHexToBytes(src)
	if err != nil {
		return err
	}
	d.deviceID, err = d.o.MakeDeviceID(b)
	return err
}

type msgIDScanner struct {
	o     ObjFactory
	msgID MsgID
}

func (m msgIDScanner) MsgID() MsgID { return m.msgID }

func (m msgIDScanner) Scan(src interface{}) error {
	b, err := scanHexToBytes(src)
	if err != nil {
		return err
	}
	m.msgID, err = m.o.MakeMsgID(b)
	return err
}

type inbandMsgTypeScanner struct {
	t InbandMsgType
}

func (i inbandMsgTypeScanner) InbandMsgType() InbandMsgType { return i.t }

func (i inbandMsgTypeScanner) Scan(src interface{}) error {
	if raw, ok := src.(int); ok {
		t := InbandMsgType(raw)
		switch t {
		case InbandMsgTypeUpdate, InbandMsgTypeSync:
			i.t = t
		default:
			return ErrBadScan
		}
	}
	return ErrBadScan

}
