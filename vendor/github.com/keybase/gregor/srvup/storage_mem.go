// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import (
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
)

// StorageMem implements srvup.Storage.
type StorageMem struct {
	groups map[string]groupMap
	clock  clockwork.Clock
	sync.Mutex
}

type groupMap map[string]time.Time

func NewStorageMem(c clockwork.Clock) *StorageMem {
	return &StorageMem{
		groups: make(map[string]groupMap),
		clock:  c,
	}
}

func (m *StorageMem) UpdateServerStatus(group, hostname string) error {
	m.Lock()
	defer m.Unlock()
	g, ok := m.groups[group]
	if !ok {
		g = make(groupMap)
		m.groups[group] = g
	}
	g[hostname] = m.clock.Now()
	return nil
}

func (m *StorageMem) AliveServers(group string, threshold time.Duration) ([]string, error) {
	m.Lock()
	defer m.Unlock()
	g, ok := m.groups[group]
	if !ok {
		return nil, nil
	}
	var alive []string
	for host, ptime := range g {
		if m.clock.Now().Sub(ptime) >= threshold {
			continue
		}
		alive = append(alive, host)
	}
	return alive, nil
}
