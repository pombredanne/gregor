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

type nodeRow struct {
	heartbeat time.Time
	uri       string
}

type groupMap map[NodeId]nodeRow

func NewStorageMem(c clockwork.Clock) *StorageMem {
	return &StorageMem{
		groups: make(map[string]groupMap),
		clock:  c,
	}
}

func (m *StorageMem) UpdateServerStatus(group string, node NodeDesc) error {
	m.Lock()
	defer m.Unlock()
	g, ok := m.groups[group]
	if !ok {
		g = make(groupMap)
		m.groups[group] = g
	}
	g[node.Id] = nodeRow{heartbeat: m.clock.Now(), uri: node.URI}
	return nil
}

func (m *StorageMem) AliveServers(group string, threshold time.Duration) ([]NodeDesc, error) {
	m.Lock()
	defer m.Unlock()
	g, ok := m.groups[group]
	if !ok {
		return nil, nil
	}
	var alive []NodeDesc
	for id, row := range g {
		if m.clock.Now().Sub(row.heartbeat) >= threshold {
			continue
		}
		alive = append(alive, NodeDesc{Id: id, URI: row.uri})
	}
	return alive, nil
}
