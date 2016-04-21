package main

import (
	"database/sql"
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var dsn string
var help bool
var interval int
var partitions int
var sleepBetweenPartitions time.Duration
var runOnce bool
var sleepBetweenRuns time.Duration
var done chan struct{}
var format string

func init() {
	flag.StringVar(&dsn, "dsn", "gregor:@/gregor_test", "mysql dsn for gregor database")
	flag.BoolVar(&help, "h", false, "show help")
	flag.BoolVar(&help, "help", false, "show help")
	flag.IntVar(&interval, "interval", 30, "clean messages older than interval `days`")
	flag.IntVar(&partitions, "partitions", 256, "number of uid partitions for the database")
	flag.DurationVar(&sleepBetweenPartitions, "sleep", 100*time.Millisecond, "sleep duration between uid partition cleans")
	flag.BoolVar(&runOnce, "once", true, "run one time, then exit")
	flag.DurationVar(&sleepBetweenRuns, "runsleep", 1*time.Minute, "sleep duration between full runs (when -once is false)")
}

func main() {
	flag.Parse()
	if help {
		flag.Usage()
		os.Exit(0)
	}

	partitionFormat()

	log.Printf("cleaning up dismissed messages older than %d days", interval)
	done = make(chan struct{})
	go signals()
	db := database()
	defer db.Close()
	run(db)
	log.Printf("done")
}

func database() *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("error opening database connection to %s: %s", dsn, err)
	}
	return db
}

func run(db *sql.DB) {
	if runOnce {
		cleanup(db)
		return
	}

	for {
		cleanup(db)
		select {
		case <-done:
			log.Printf("aborting run loop")
			return
		case <-time.After(sleepBetweenRuns):
		}
	}
}

func cleanup(db *sql.DB) {
	for i := 0; i < partitions; i++ {
		if i > 0 {
			select {
			case <-done:
				log.Printf("aborting cleanup loop")
				return
			case <-time.After(sleepBetweenPartitions):
			}
		}
		partition(db, fmt.Sprintf(format, i))
	}
}

// Note that this query relies on the following foreign key reference
// FOREIGN KEY(uid, msgid) REFERENCES messages (uid, msgid) ON DELETE CASCADE
// on child tables of messages.
const deleteQuery = `DELETE FROM messages USING 
	messages LEFT JOIN items ON (messages.uid=items.uid AND messages.msgid=items.msgid) 
	WHERE items.uid LIKE ? AND items.dtime < DATE_SUB(NOW(), INTERVAL ? DAY)`

func partition(db *sql.DB, prefix string) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("error starting transaction: %s", err)
	}
	defer tx.Commit()

	res, err := tx.Exec(deleteQuery, prefix+"%", interval)
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

func signals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
	select {
	case x := <-c:
		log.Printf("caught signal %v", x)
		close(done)
	}
}

// partitionFormat generates a fmt format string to transform an
// int into a uid partition prefix.  The number of partitions must
// be a power of 16 so that the entire uid space is partitioned.
// e.g., '0%' => 'f%', '00%' => 'ff%', '000%' => 'fff%'.
func partitionFormat() {
	// convert partitions to hex and make sure it is a power of 16.
	maxHex := fmt.Sprintf("%x", partitions-1)
	for _, x := range maxHex {
		if x != 'f' {
			log.Fatalf("number of partitions: %d, must be a power of 16", partitions)
		}
	}

	// The uid partition format should be of the form "%02x" where 16 ^ 2 == partitions.
	// The individual parts of the format below that generates this are:
	// %% => single percent sign
	// 0  => zero pad
	// %d => Use the length of maxHex to specify the length of the string.
	// x  => format as hex
	format = fmt.Sprintf("%%0%dx", len(maxHex))
}
