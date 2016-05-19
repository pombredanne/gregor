package storage

import (
	"database/sql"
	"os"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/test"
	_ "github.com/mattn/go-sqlite3"
)

func testEngine(t *testing.T, db *sql.DB, w sqlTimeWriter) {
	cl := clockwork.NewFakeClock()
	var of gregor1.ObjFactory
	eng := NewSQLEngine(db, of, w, cl, "")
	test.TestStateMachineAllDevices(t, eng)
	test.TestStateMachinePerDevice(t, eng)
}

func TestSqliteEngine(t *testing.T) {
	name := "./gregor.db"
	os.Remove(name)
	db, err := CreateDB("sqlite3", "./gregor.db")
	if err != nil {
		t.Fatal(err)
	}
	testEngine(t, db, sqliteTimeWriter{})
}

// Test with: MYSQL_DSN=gregor:@/gregor_test?parseTime=true go test
func TestMySQLEngine(t *testing.T) {
	testEngine(t, AcquireTestDB(t), mysqlTimeWriter{})
	ReleaseTestDB()
}
