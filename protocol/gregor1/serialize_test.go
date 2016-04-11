package gregor1

import (
	"reflect"
	"testing"
	"time"

	gregor "github.com/keybase/gregor"
)

func MakeTestState(o gregor.ObjFactory) (gregor.State, error) {
	u, err := o.MakeUID([]byte{0x01, 0x02, 0x03})
	if err != nil {
		return nil, err
	}

	deviceid, err := o.MakeDeviceID([]byte{0x0})
	if err != nil {
		return nil, err
	}

	c, err := o.MakeCategory("foo")
	if err != nil {
		return nil, err
	}

	body, err := o.MakeBody([]byte{0x01, 0x02, 0x03})
	if err != nil {
		return nil, err
	}

	items := make([]gregor.Item, 12)
	for i := range items {
		msgid, err := o.MakeMsgID([]byte{byte(i)})
		if err != nil {
			return nil, err
		}
		items[i], err = o.MakeItem(u, msgid, deviceid, time.Now(), c, nil, body)
		if err != nil {
			return nil, err
		}
	}
	return o.MakeState(items)
}

func TestMarshalState(t *testing.T) {
	var o ObjFactory
	state, err := MakeTestState(o)
	if err != nil {
		t.Fatal(err)
	}

	b, err := state.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	state2, err := o.UnmarshalState(b)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(state, state2) {
		t.Fatal("states not equal")
	}
}
