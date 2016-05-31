// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
)

// NodeId is a random id of a node
type NodeId string

// NodeDesc identifies a node in the group
type NodeDesc struct {
	Hostname string
	Id       NodeId
}

func (n NodeDesc) String() string {
	return fmt.Sprintf("[ %s, %s ]", n.Id, n.Hostname)
}

// Storage is an interface for storing and querying server status.
type Storage interface {
	UpdateServerStatus(group string, node NodeDesc) error
	AliveServers(group string, threshold time.Duration) ([]NodeDesc, error)
}

// Status maintains a server's alive status and queries for the
// alive status of all the servers.
type Status struct {
	group              string
	myID               NodeId
	heartbeatInterval  time.Duration
	aliveThreshold     time.Duration
	storage            Storage
	log                Logger
	done               chan struct{}
	clock              clockwork.Clock
	aliveCacheMu       sync.RWMutex
	aliveCache         []NodeDesc
	aliveCacheAt       time.Time
	aliveCacheDuration time.Duration
	wg                 sync.WaitGroup
}

// New creates a new Status for a server.
func New(group string, heartbeatInterval, aliveThreshold time.Duration, s Storage) *Status {

	// Generate a random ID for ourselves
	rawid := make([]byte, 8)
	rand.Read(rawid)
	id := base64.StdEncoding.EncodeToString(rawid)

	return &Status{
		group:              group,
		myID:               NodeId(id),
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
func (s *Status) Alive() ([]NodeDesc, error) {
	s.aliveCacheMu.RLock()
	if s.aliveCacheValid() {
		defer s.aliveCacheMu.RUnlock()
		return s.aliveCache, nil
	}
	s.aliveCacheMu.RUnlock()

	// cache is stale
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

// MyID returns the unique ID of the owner of the statsu group
func (s *Status) MyID() NodeId {
	return s.myID
}

// HeartbeatLoop runs a loop in a separate goroutine that sends a
// heartbeat every s.pingInterval.
func (s *Status) HeartbeatLoop(hostname string) {

	// Put one out right away to make testing this easier
	if err := s.heartbeat(hostname); err != nil {
		s.log.Warning("heartbeat error: %s", err)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			start := s.clock.Now()
			if err := s.heartbeat(hostname); err != nil {
				s.log.Warning("heartbeat error: %s", err)
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
	return s.storage.UpdateServerStatus(s.group,
		NodeDesc{Hostname: hostname, Id: NodeId(s.MyID())})
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
