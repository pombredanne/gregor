// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

// StorageMysql is an implementation of srvup.Storage that uses
// mysql to store the server status information.
type StorageMysql struct {
	db *sql.DB
}

// NewStorageMysql creates a StorageMysql object.
func NewStorageMysql(dsn string) (*StorageMysql, error) {
	s := &StorageMysql{}
	if err := s.initialize(dsn); err != nil {
		return nil, err
	}
	return s, nil
}

// Shutdown releases any resources that this StorageMysql object is
// using. This object is not usable after Shutdown.
func (s *StorageMysql) Shutdown() {
	if s.db == nil {
		return
	}
	s.db.Close()
}

func (s *StorageMysql) initialize(dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	s.db = db

	return s.createSchema()
}

func (s *StorageMysql) createSchema() error {
	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// resetSchema will drop all tables and recreate them.  Use with
// care, probably only in tests.
func (s *StorageMysql) resetSchema() error {
	for _, stmt := range reset {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return s.createSchema()
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS server_status (
		groupname VARCHAR(32) NOT NULL,
		hostname VARCHAR(128) NOT NULL,
		hbtime DATETIME(6) NOT NULL,
		ctime DATETIME NOT NULL,
		PRIMARY KEY (groupname, hostname),
		INDEX (groupname, hbtime)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8`,
}

var reset = []string{
	`DROP TABLE IF EXISTS server_status`,
}
