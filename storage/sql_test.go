package storage

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/schema"
	"github.com/keybase/gregor/test"
	_ "github.com/mattn/go-sqlite3"
)

func testEngine(t *testing.T, db *sql.DB, w sqlTimeWriter, updateLock bool) {
	cl := clockwork.NewFakeClock()
	var of gregor1.ObjFactory
	eng := NewSQLEngine(db, of, w, cl, updateLock)
	test.TestStateMachineAllDevices(t, eng)
	test.TestStateMachinePerDevice(t, eng)
	test.TestStateMachineReminders(t, eng)
}

func TestSqliteEngine(t *testing.T) {
	name := "./gregor.db"
	os.Remove(name)
	db, err := schema.CreateDB("sqlite3", "./gregor.db")
	if err != nil {
		t.Fatal(err)
	}
	testEngine(t, db, sqliteTimeWriter{}, false)
}

// Test with: MYSQL_DSN=gregor:@/gregor_test?parseTime=true go test
func TestMySQLEngine(t *testing.T) {
	testEngine(t, test.AcquireTestDB(t), mysqlTimeWriter{}, true)
	test.ReleaseTestDB()
}
