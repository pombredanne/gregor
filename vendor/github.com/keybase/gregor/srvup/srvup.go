// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import (
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
)

// Storage is an interface for storing and querying server status.
type Storage interface {
	UpdateServerStatus(group, hostname string) error
	AliveServers(group string, threshold time.Duration) ([]string, error)
}

// Status maintains a server's alive status and queries for the
// alive status of all the servers.
type Status struct {
	group              string
	heartbeatInterval  time.Duration
	aliveThreshold     time.Duration
	storage            Storage
	log                Logger
	done               chan struct{}
	clock              clockwork.Clock
	aliveCacheMu       sync.RWMutex
	aliveCache         []string
	aliveCacheAt       time.Time
	aliveCacheDuration time.Duration
	wg                 sync.WaitGroup
}

// New creates a new Status for a server.
func New(group string, heartbeatInterval, aliveThreshold time.Duration, s Storage) *Status {
	return &Status{
		group:              group,
		heartbeatInterval:  heartbeatInterval,
		aliveThreshold:     aliveThreshold,
		storage:            s,
		done:               make(chan struct{}),
		clock:              clockwork.NewRealClock(),
		aliveCacheDuration: 1 * time.Second,
		log:                defaultLogger{},
	}
}

// Alive returns a list of hostnames for servers that have pinged
// within s.aliveThreshold.
func (s *Status) Alive() ([]string, error) {
	s.aliveCacheMu.RLock()
	if s.aliveCacheValid() {
		defer s.aliveCacheMu.RUnlock()
		return s.aliveCache, nil
	}

	// cache is stale
	s.aliveCacheMu.RUnlock()

	s.aliveCacheMu.Lock()
	defer s.aliveCacheMu.Unlock()

	// check cache again after getting lock
	if s.aliveCacheValid() {
		return s.aliveCache, nil
	}

	// use Storage to get alive server list
	g, err := s.storage.AliveServers(s.group, s.aliveThreshold)
	if err != nil {
		return nil, err
	}
	if len(g) == 0 && s.aliveCacheAt.IsZero() {
		// don't cache empty set if nothing cached before...
		return nil, nil
	}
	s.aliveCache = g
	s.aliveCacheAt = s.clock.Now()

	return s.aliveCache, nil
}

// HeartbeatLoop runs a loop in a separate goroutine that sends a
// heartbeat every s.pingInterval.
func (s *Status) HeartbeatLoop(hostname string) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			start := s.clock.Now()
			if err := s.heartbeat(hostname); err != nil {
				s.log.Warning("heartbat error: %s", err)
			}
			sleepDur := s.heartbeatInterval - s.since(start)
			if sleepDur < 0 {
				sleepDur = 0
			}

			select {
			case <-s.clock.After(sleepDur):
				continue
			case <-s.done:
				return
			}
		}
	}()
}

func (s *Status) heartbeat(hostname string) error {
	return s.storage.UpdateServerStatus(s.group, hostname)
}

// Shutdown stops the HeartbeatLoop and waits for it to finish.
func (s *Status) Shutdown() {
	close(s.done)
	s.wg.Wait()
}

func (s *Status) setClock(c clockwork.Clock) {
	s.clock = c
}

// SetLogger sets the logger for Status.
func (s *Status) SetLogger(l Logger) {
	s.log = l
}

func (s *Status) since(t time.Time) time.Duration {
	return s.clock.Now().Sub(t)
}

// hold s.aliveCacheMu RLock or Lock before calling this
func (s *Status) aliveCacheValid() bool {
	return s.since(s.aliveCacheAt) < s.aliveCacheDuration
}
