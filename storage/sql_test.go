package storage

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/protocol/gregor1"
	test "github.com/keybase/gregor/test"
	_ "github.com/mattn/go-sqlite3"
)

func createDb(engine string, name string) (*sql.DB, error) {
	db, err := sql.Open(engine, name)
	if err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return db, err
	}
	schema := Schema(engine)
	for _, stmt := range schema {
		if _, err = tx.Exec(stmt); err != nil {
			return db, err
		}
	}

	if err = tx.Commit(); err != nil {
		return db, err
	}
	return db, nil
}

func testEngine(t *testing.T, engine string, name string, w sqlTimeWriter) {
	db, err := createDb(engine, name)
	if db != nil {
		defer db.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	cl := clockwork.NewFakeClock()
	var of gregor1.ObjFactory
	eng := NewSQLEngine(db, of, w, cl)
	test.TestStateMachineAllDevices(t, eng)
	test.TestStateMachinePerDevice(t, eng)
}

func TestSqliteEngine(t *testing.T) {
	name := "./gregor.db"
	os.Remove(name)
	testEngine(t, "sqlite3", "./gregor.db", sqliteTimeWriter{})
}

// Test with: TEST_MYSQL_DSN=gregor:@/gregor_test?parseTime=true go test
func TestMySQLEngine(t *testing.T) {
	name := os.Getenv("TEST_MYSQL_DSN")
	if name == "" {
		t.Skip("TEST_MYSQL_DSN not set")
	}

	// We need to have parseTime=true as one of our DSN paramenters,
	// so if it's not there, add it on. This way, in sql_scan, we'll get
	// time.Time values back from MySQL.
	u, err := url.Parse(name)
	if err != nil {
		t.Fatalf("couldn't parse TEST_MYSQL_DSN: %v", err)
	}
	u = ForceParseTime(u)
	testEngine(t, "mysql", u.String(), mysqlTimeWriter{})
}
