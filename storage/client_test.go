package storage

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/test"
	"github.com/syndtr/goleveldb/leveldb"
)

var of gregor1.ObjFactory

func TestLevelDBClient(t *testing.T) {
	fname, err := ioutil.TempDir("", "gregor")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fname)

	fc := clockwork.NewFakeClock()
	sm := NewMemEngine(of, fc)
	user, device := test.TestStateMachinePerDevice(t, sm)
	db, err := leveldb.OpenFile(fname, nil)
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(user, device, sm, &LevelDBStorageEngine{db})

	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	if err := c.Restore(); err != nil {
		t.Fatal(err)
	}
}
