package client

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/jonboulle/clockwork"
	protocol "github.com/keybase/gregor/protocol/go"
	"github.com/keybase/gregor/storage"
	"github.com/syndtr/goleveldb/leveldb"
)

var objFactory protocol.ObjFactory

func TestLevelDBClient(t *testing.T) {
	fname, err := ioutil.TempDir("", "gregor")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fname)

	user, err := objFactory.MakeUID([]byte("user"))
	if err != nil {
		t.Fatal(err)
	}
	device, err := objFactory.MakeDeviceID([]byte("device"))
	if err != nil {
		t.Fatal(err)
	}
	sm := storage.NewMemEngine(objFactory, clockwork.NewFakeClock())
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
