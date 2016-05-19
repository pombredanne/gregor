package test

import (
	"database/sql"
	"log"
	"os"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/keybase/gregor"
)

var (
	db     *sql.DB
	dbLock sync.Mutex
)

// AcquireTestDB returns a MySQL DB and acquires a lock be released with ReleaseTestDB.
func AcquireTestDB(t *testing.T) *sql.DB {
	var dsn string
	s := os.Getenv("MYSQL_DSN")
	if s != "" {
		var err error
		dsn, err = gregor.URLAddParseTime(s)
		if err != nil {
			t.Skip("Error parsing MYSQL_DSN")
		}
	}

	if dsn == "" {
		t.Skip("Error parsing MYSQL_DSN")
	}

	dbLock.Lock()
	log.Println("Acquiring Test DB")
	if db == nil {
		log.Println("Connecting to Test DB")
		var err error
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			dbLock.Unlock()
			t.Fatal(err)
		}
	} else {
		log.Println("Reusing connection to Test DB")
	}
	return db
}

// ReleaseTestDB releases a lock acquired by AcquireTestDB.
func ReleaseTestDB() {
	log.Println("Releasing Test DB")
	dbLock.Unlock()
}
