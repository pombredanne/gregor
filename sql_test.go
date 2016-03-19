package gregor

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3"
	"net/url"
	"os"
	"testing"
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
	eng := NewSQLEngine(db, testObjFactory{}, w, cl)
	testStateMachineAllDevices(t, eng, cl)
	testStateMachinePerDevice(t, eng, cl)
}

func TestSqliteEngine(t *testing.T) {
	name := "./gregor.db"
	os.Remove(name)
	testEngine(t, "sqlite3", "./gregor.db", sqliteTimeWriter{})
}

// Test with: MYSQL_DSN=gregor:@/gregor_test?parseTime=true go test
func TestMySQLEngine(t *testing.T) {
	name := os.Getenv("MYSQL_DSN")
	if name == "" {
		t.Skip("MYSQL_DSN not set")
	}

	// We need to have parseTime=true as one of our DSN paramenters,
	// so if it's not there, add it on. This way, in sql_scan, we'll get
	// time.Time values back from MySQL.
	u, err := url.Parse(name)
	if err != nil {
		t.Fatalf("couldn't parse MYSQL_DSN: %v", err)
	}
	query := u.Query()
	query.Set("parseTime", "true")
	u.RawQuery = query.Encode()
	testEngine(t, "mysql", u.String(), mysqlTimeWriter{})
}
