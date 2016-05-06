// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import (
	"time"

	"github.com/jonboulle/clockwork"
)

// memstore implements srvup.Storage.
type memstore struct {
	groups map[string]groupMap
	clock  clockwork.Clock
}

type groupMap map[string]time.Time

func newMemstore(c clockwork.Clock) *memstore {
	return &memstore{
		groups: make(map[string]groupMap),
		clock:  c,
	}
}

func (m *memstore) UpdateServerStatus(group, hostname string) error {
	g, ok := m.groups[group]
	if !ok {
		g = make(groupMap)
		m.groups[group] = g
	}
	g[hostname] = m.clock.Now()
	return nil
}

func (m *memstore) AliveServers(group string, threshold time.Duration) ([]string, error) {
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
