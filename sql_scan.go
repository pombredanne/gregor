package gregor

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var ErrBadScan = errors.New("bad scan of data type")
var ErrBadString = errors.New("expected either a string or a []byte")

type uidScanner struct {
	o   ObjFactory
	uid UID
}

func (u uidScanner) UID() UID { return u.uid }

func toString(src interface{}) (string, error) {
	switch s := src.(type) {
	case string:
		return s, nil
	case []byte:
		return string(s), nil
	default:
		return "", ErrBadString
	}
}

func scanHexToBytes(src interface{}) ([]byte, error) {
	s, err := toString(src)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(s)
}

func (u uidScanner) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
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
	if src == nil {
		return nil
	}
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
	fmt.Printf("scanning -> %v\n", src)
	if src == nil {
		return nil
	}
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
	if src == nil {
		return nil
	}
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

type categoryScanner struct {
	o     ObjFactory
	c     Category
	isSet bool
}

func (c categoryScanner) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	s, err := toString(src)
	if err != nil {
		return err
	}
	c.c, err = c.o.MakeCategory(s)
	if err == nil {
		c.isSet = true
	}
	return err
}

func (c categoryScanner) Category() Category { return c.c }

func (c categoryScanner) IsSet() bool { return c.isSet }

type bodyScanner struct {
	o ObjFactory
	b Body
}

func (b bodyScanner) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	if raw, ok := src.([]byte); ok {
		var err error
		b.b, err = b.o.MakeBody(raw)
		return err
	}
	return ErrBadScan
}

func (b bodyScanner) Body() Body { return b.b }

type timeOrNilScanner struct {
	t time.Time
}

func (t timeOrNilScanner) Scan(src interface{}) error {
	fmt.Printf("scan %T %v %s\n", src, src, src)
	if src == nil {
		return nil
	}
	if raw, ok := src.(time.Time); ok {
		t.t = raw
		return nil
	}
	return ErrBadScan
}

func (t timeOrNilScanner) Time() *time.Time {
	if t.t.IsZero() {
		return nil
	}
	return &t.t
}
