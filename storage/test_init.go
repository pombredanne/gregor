package storage

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
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

func InitMySQLEngine(t *testing.T) *sql.DB {
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
	db, err := createDb("mysql", u.String())
	if err != nil {
		t.Fatal(err)
	}
	return db
}
