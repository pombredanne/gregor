// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql" // for mysql driver
	"github.com/jonboulle/clockwork"
)

// StorageMysql is an implementation of srvup.Storage that uses
// mysql to store the server status information.
type StorageMysql struct {
	db      *sql.DB
	update  *sql.Stmt
	alive   *sql.Stmt
	cleanup *sql.Stmt
	clock   clockwork.Clock
	log     Logger
	done    chan struct{}
}

// NewStorageMysql creates a StorageMysql object.
func NewStorageMysql(dsn string, log Logger) (*StorageMysql, error) {
	s := &StorageMysql{
		log:  log,
		done: make(chan struct{}),
	}
	if err := s.initialize(dsn); err != nil {
		return nil, err
	}

	go s.cleanLoop()

	return s, nil
}

// Shutdown releases any resources that this StorageMysql object is
// using. This object is not usable after Shutdown.
func (s *StorageMysql) Shutdown() error {
	close(s.done)
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *StorageMysql) initialize(dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	s.db = db

	return s.createSchema()
}

// setClock stores a clock for use in testing.  See s.now().
func (s *StorageMysql) setClock(c clockwork.Clock) {
	s.clock = c
}

// UpdateServerStatus implements Storage.UpdateServerStatus.
func (s *StorageMysql) UpdateServerStatus(group string, node NodeDesc) error {
	var err error
	if s.update == nil {
		if s.clock != nil {
			s.update, err = s.db.Prepare("INSERT INTO server_status (groupname, id, address, hbtime, ctime, uri) VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE hbtime=?")
		} else {
			s.update, err = s.db.Prepare("INSERT INTO server_status (groupname, id, address, hbtime, ctime, uri) VALUES (?, ?, ?, NOW(), NOW()) ON DUPLICATE KEY UPDATE hbtime=NOW()")
		}
		if err != nil {
			return err
		}
	}

	if s.clock != nil {
		now := s.now()
		_, err = s.update.Exec(group, string(node.Id), node.URI, now, now, now, node.URI)
	} else {
		_, err = s.update.Exec(group, string(node.Id), node.URI, node.URI)
	}
	return err
}

// AliveServers implements Storage.AliveServers.
func (s *StorageMysql) AliveServers(group string, threshold time.Duration) ([]NodeDesc, error) {
	var err error
	if s.alive == nil {
		if s.clock != nil {
			s.alive, err = s.db.Prepare("SELECT id,uri FROM server_status WHERE groupname=? AND hbtime >= DATE_SUB(?, INTERVAL ? SECOND)")
		} else {
			s.alive, err = s.db.Prepare("SELECT id,uri FROM server_status WHERE groupname=? AND hbtime >= DATE_SUB(NOW(), INTERVAL ? SECOND)")
		}
		if err != nil {
			return nil, err
		}
	}

	var rows *sql.Rows
	if s.clock != nil {
		rows, err = s.alive.Query(group, s.now(), int(threshold/time.Second))
	} else {
		rows, err = s.alive.Query(group, int(threshold/time.Second))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []NodeDesc
	for rows.Next() {
		var id, uri string
		err = rows.Scan(&id, &uri)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, NodeDesc{Id: NodeId(id), URI: uri})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return nodes, nil
}

func (s *StorageMysql) cleanLoop() {
	for {
		select {
		case <-s.done:
			return
		case <-time.After(time.Minute):
			if err := s.clean(); err != nil {
				s.log.Warning("clean error: %s", err)
			}
		}
	}
}

func (s *StorageMysql) clean() error {
	if s.cleanup == nil {
		stmt, err := s.db.Prepare("DELETE FROM server_status WHERE hbtime < DATE_SUB(NOW(), INTERVAL 1 HOUR)")
		if err != nil {
			return fmt.Errorf("prepare error: %s", err)
		}
		s.cleanup = stmt
	}
	_, err := s.cleanup.Exec()
	return err
}

func (s *StorageMysql) now() string {
	return s.clock.Now().Format("2006-01-02 15:04:05")
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
		groupname varchar(32) NOT NULL,
		id char(16) NOT NULL,
		uri varchar(128) NOT NULL,
		address varchar(128) NOT NULL,
		hbtime datetime(6) NOT NULL,
		ctime datetime NOT NULL,
		PRIMARY KEY (groupname,id),
		KEY groupname (groupname,hbtime)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8`,
}

var reset = []string{
	`DROP TABLE IF EXISTS server_status`,
}
