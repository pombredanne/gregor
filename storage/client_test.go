package storage

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/jonboulle/clockwork"
	protocol "github.com/keybase/gregor/protocol/go"
	"github.com/keybase/gregor/test"
	"github.com/syndtr/goleveldb/leveldb"
)

var objFactory protocol.ObjFactory

func TestLevelDBClient(t *testing.T) {
	fname, err := ioutil.TempDir("", "gregor")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fname)

	fc := clockwork.NewFakeClock()
	sm := NewMemEngine(objFactory, fc)
	user, device := test.TestStateMachinePerDevice(t, objFactory, sm, fc)
	db, err := leveldb.OpenFile(fname, nil)
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(user, device, objFactory, sm, &LevelDBStorageEngine{db})

	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	if err := c.Restore(); err != nil {
		t.Fatal(err)
	}
}
