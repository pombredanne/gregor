// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import (
	"os"
	"testing"
)

func connectMysql(t *testing.T) *StorageMysql {
	dsn := os.Getenv("MYSQL_DSN")
	if len(dsn) == 0 {
		t.Skip("No MYSQL_DSN env variable, skipping test")
	}
	s, err := NewStorageMysql(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		t.Fatal("nil storage object")
	}
	t.Logf("connected to mysql: %s", dsn)
	if err := s.resetSchema(); err != nil {
		t.Fatal(err)
	}

	return s
}

func TestConnect(t *testing.T) {
	s := connectMysql(t)
	_ = s
}
