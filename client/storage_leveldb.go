package client

import (
	"github.com/keybase/gregor"
	"github.com/syndtr/goleveldb/leveldb"
)

type LevelDBStorageEngine *leveldb.DB

func (db LevelDBStorageEngine) Store(user gregor.UID, value []byte) error {
	return db.Put(user.Bytes(), value, nil)
}

func (db LevelDBStorageEngine) Load(user gregor.UID) ([]byte, error) {
	return db.Get(user.Bytes(), nil)
}

var _ StorageEngine = LevelDBStorageEngine(nil)
