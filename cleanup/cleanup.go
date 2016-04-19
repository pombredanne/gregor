package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	var interval int
	var help bool
	var dsn string

	flag.IntVar(&interval, "interval", 30, "clean messages older than interval `days`")
	flag.BoolVar(&help, "h", false, "show help")
	flag.BoolVar(&help, "help", false, "show help")
	flag.StringVar(&dsn, "dsn", "gregor:@/gregor_test", "mysql dsn for gregor database")

	flag.Parse()
	if help {
		flag.Usage()
		os.Exit(0)
	}

	log.Printf("cleaning up dismissed messages older than %d days", interval)
	db := database(dsn)
	defer db.Close()
	cleanup(db, interval)
	log.Printf("done")
}

func database(dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("error opening database connection to %s: %s", dsn, err)
	}
	return db
}

func cleanup(db *sql.DB, interval int) {
	for i := 0x00; i <= 0xff; i++ {
		partition(db, fmt.Sprintf("%02x", i), interval)
	}
}

func partition(db *sql.DB, prefix string, interval int) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("error starting transaction: %s", err)
	}
	defer tx.Commit()

	// Note that this query relies on the following foreign key reference
	// FOREIGN KEY(uid, msgid) REFERENCES messages (uid, msgid) ON DELETE CASCADE
	// on child tables of messages.
	res, err := tx.Exec("DELETE FROM messages USING messages LEFT JOIN items ON (messages.uid=items.uid AND messages.msgid=items.msgid) WHERE items.uid LIKE ? AND items.dtime < DATE_SUB(NOW(), INTERVAL ? DAY)", prefix+"%", interval)
	if err != nil {
		tx.Rollback()
		log.Fatalf("[%s] tx exec error: %s", prefix, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		log.Printf("[%s] error getting rows affected: %s", prefix, err)
		return
	}

	log.Printf("uid prefix %s, messages deleted: %d", prefix, affected)
}
