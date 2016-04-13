package gregor1

import (
	"bytes"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	gregor "github.com/keybase/gregor"
)

func MakeTestItems(o gregor.ObjFactory, fc clockwork.FakeClock) ([]gregor.Item, error) {
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
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}

		msgid, err := o.MakeMsgID(b[:])
		if err != nil {
			return nil, err
		}

		if i%3 == 0 {
			fc.Advance(time.Second)
		}
		items[i], err = o.MakeItem(u, msgid, deviceid, fc.Now(), c, nil, body)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func TestMarshalState(t *testing.T) {
	var o ObjFactory
	fc := clockwork.NewFakeClock()
	items, err := MakeTestItems(o, fc)
	if err != nil {
		t.Fatal(err)
	}

	state, err := o.MakeState(items)
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

func TestHashState(t *testing.T) {
	var o ObjFactory
	fc := clockwork.NewFakeClock()
	items, err := MakeTestItems(o, fc)
	if err != nil {
		t.Fatal(err)
	}

	perm := rand.Perm(len(items))
	shuffled := make([]gregor.Item, len(items))
	for i := range shuffled {
		shuffled[i] = items[perm[i]]
	}

	state1, err := o.MakeState(items)
	if err != nil {
		t.Fatal(err)
	}

	state2, err := o.MakeState(shuffled)
	if err != nil {
		t.Fatal(err)
	}

	h1, err := state1.Hash()
	if err != nil {
		t.Fatal(err)
	}

	h2, err := state2.Hash()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(h1, h2) {
		t.Fatal("shuffled state hash not equal to original")
	}
}
