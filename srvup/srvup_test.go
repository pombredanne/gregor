// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
)

func setup() (*Status, clockwork.FakeClock) {
	c := clockwork.NewFakeClock()
	s := New("gregord", 1*time.Second, 2*time.Second, newMemstore(c))
	s.setClock(c)
	return s, c
}

func aliveCheck(t *testing.T, s *Status, n int) {
	x, err := s.Alive()
	if err != nil {
		t.Fatal(err)
	}
	if len(x) != n {
		t.Errorf("alive servers: %d, expected %d", len(x), n)
	}

}

func TestBasics(t *testing.T) {
	s, c := setup()
	defer s.Shutdown()

	aliveCheck(t, s, 0)
	s.heartbeat("localhost:9911")
	aliveCheck(t, s, 1)
	c.Advance(3 * time.Second)
	aliveCheck(t, s, 0)
}

func TestPingLoop(t *testing.T) {
	s, c := setup()
	defer s.Shutdown()

	aliveCheck(t, s, 0)

	s.HeartbeatLoop("localhost:9911")
	c.BlockUntil(1)

	for i := 0; i < 10; i++ {
		c.Advance(1 * time.Second)
		c.BlockUntil(1)
		aliveCheck(t, s, 1)
	}
}

func TestAliveCache(t *testing.T) {
	s, _ := setup()
	defer s.Shutdown()

	aliveCheck(t, s, 0)
	s.heartbeat("localhost:9911")
	aliveCheck(t, s, 1)

	// remove the storage engine from Status to check that the cache works
	s.storage = nil

	aliveCheck(t, s, 1)
}
