package gregor

import (
	"database/sql"
	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"testing"
)

func createSqliteDb() (*sql.DB, error) {
	name := "./gregor.db"
	os.Remove(name)
	db, err := sql.Open("sqlite3", name)
	if err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return db, err
	}
	if _, err = tx.Exec(SqliteSchema()); err != nil {
		return db, err
	}

	if err = tx.Commit(); err != nil {
		return db, err
	}
	return db, nil
}

func TestSqliteEngine(t *testing.T) {
	db, err := createSqliteDb()
	if db != nil {
		defer db.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	cl := clockwork.NewFakeClock()
	eng := NewSQLEngine(db, testObjFactory{}, sqliteTimeWriter{}, cl)
	testStateMachineAllDevices(t, eng, cl)
	testStateMachinePerDevice(t, eng, cl)
}
