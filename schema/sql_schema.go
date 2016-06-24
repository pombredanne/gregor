package schema

import (
	"database/sql"
	"strings"
)

var schema = []string{

	`DROP TABLE IF EXISTS gregor_dismissals_by_time`,
	`DROP TABLE IF EXISTS gregor_dismissals_by_id`,
	`DROP TABLE IF EXISTS gregor_reminders`,
	`DROP TABLE IF EXISTS gregor_items`,
	`DROP TABLE IF EXISTS gregor_messages`,

	`CREATE TABLE gregor_messages (
		uid   CHAR(32) NOT NULL,
		msgid CHAR(32) NOT NULL,
		ctime DATETIME(6) NOT NULL,
		devid CHAR(32),
		mtype INTEGER UNSIGNED NOT NULL, -- "specify for 'Update' or 'Sync' types",
		PRIMARY KEY(uid, msgid)
	)`,

	`CREATE TABLE gregor_items (
		uid   CHAR(32) NOT NULL,
		msgid CHAR(32) NOT NULL,
		category VARCHAR(128) NOT NULL,
		dtime DATETIME(6),
		body BLOB,
		FOREIGN KEY(uid, msgid) REFERENCES gregor_messages (uid, msgid) ON DELETE CASCADE,
		PRIMARY KEY(uid, msgid)
	)`,

	`CREATE INDEX gregor_user_order ON gregor_items (uid, category)`,

	`CREATE INDEX gregor_cleanup_order ON gregor_items (uid, dtime)`,

	`CREATE TABLE gregor_reminders (
		uid   CHAR(32) NOT NULL,
		msgid CHAR(32) NOT NULL,
		seqno INT UNSIGNED NOT NULL,
		rtime DATETIME(6) NOT NULL,
		lock_time DATETIME(6),
		FOREIGN KEY(uid, msgid) REFERENCES gregor_messages (uid, msgid) ON DELETE CASCADE,
		PRIMARY KEY(uid, msgid, seqno)
	)`,

	`CREATE INDEX gregor_reminder_order ON gregor_reminders (rtime)`,

	`CREATE TABLE gregor_dismissals_by_id (
		uid   CHAR(32) NOT NULL,
		msgid CHAR(32) NOT NULL,
		dmsgid CHAR(32) NOT NULL, -- "the message IDs to dismiss",
		FOREIGN KEY(uid, msgid) REFERENCES gregor_messages (uid, msgid) ON DELETE CASCADE,
		FOREIGN KEY(uid, dmsgid) REFERENCES gregor_messages (uid, msgid) ON DELETE CASCADE,
		PRIMARY KEY(uid, msgid, dmsgid)
	)`,

	`CREATE TABLE gregor_dismissals_by_time (
		uid   CHAR(32) NOT NULL,
		msgid CHAR(32) NOT NULL,
		category VARCHAR(128) NOT NULL,
		dtime DATETIME(6) NOT NULL, -- "throw out matching events before dtime",
		FOREIGN KEY(uid, msgid) REFERENCES gregor_messages (uid, msgid) ON DELETE CASCADE,
		PRIMARY KEY(uid, msgid, category, dtime)
	)`,
}

func Schema(engine string) []string {
	return schema
}

func engineCustomize(engine string, stmt string) string {
	if engine == "mysql" && strings.HasPrefix(stmt, "CREATE TABLE") {
		return stmt + " ENGINE=InnoDB DEFAULT CHARSET=utf8"
	}
	return stmt
}

// CreateDB connects to a DB and initializes it with Gregor Schema.
func CreateDB(engine string, name string) (*sql.DB, error) {
	db, err := sql.Open(engine, name)
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}

	for _, stmt := range Schema(engine) {
		estmt := engineCustomize(engine, stmt)
		if _, err := tx.Exec(estmt); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return db, nil
}

func DBExists(engine string, dsn string) (bool, error) {
	db, err := sql.Open(engine, dsn)
	if err != nil {
		return false, err
	}

	q, err := db.Prepare("SHOW TABLES LIKE 'gregor%'")
	if err != nil {
		return false, err
	}
	rows, err := q.Query()
	if err != nil {
		return false, err
	}

	return rows.Next(), nil
}
