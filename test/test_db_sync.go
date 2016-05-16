package test

import (
	"database/sql"
	"log"
	"net/url"
	"os"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

var (
	db     *sql.DB
	dbLock sync.Mutex
)

// AcquireTestDB returns a MySQL DB and acquires a lock be released with ReleaseTestDB.
func AcquireTestDB(t *testing.T) *sql.DB {
	s := os.Getenv("MYSQL_DSN")
	dsn := ""
	if s != "" {
		udsn, err := url.Parse(s)
		if err != nil {
			t.Skip("Error parsing MYSQL_DSN")
		}
		query := udsn.Query()
		query.Set("parseTime", "true")
		udsn.RawQuery = query.Encode()

		dsn = udsn.String()
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
