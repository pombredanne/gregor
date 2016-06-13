// Copyright 2016 Keybase Inc. All rights reserved.
// Use of this source code is governed by a BSD
// license that can be found in the LICENSE file.

package stats

import (
	"errors"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
)

type Backend interface {
	Count(name string) error
	CountMult(name string, count int) error
	Value(name string, value float64) error
	Shutdown()
}

type Registry interface {
	Count(name string)
	CountMult(name string, count int)
	ValueInt(name string, value int)
	Value(name string, value float64)
	SetPrefix(prefix string) Registry
	GetPrefix() string
	Shutdown()
}

type SimpleRegistry struct {
	backend Backend
	prefix  string
	log     rpc.LogOutput
}

var _ Registry = SimpleRegistry{}

func (r SimpleRegistry) makeFname(name string) string {
	return r.prefix + name
}

func (r SimpleRegistry) errorDebug(err error, name string) {
	r.log.Debug("failed to post stat: err: %s name: %s", err, name)
}

func (r SimpleRegistry) SetPrefix(prefix string) Registry {
	newreg := SimpleRegistry{backend: r.backend, log: r.log}
	if r.prefix == "" {
		newreg.prefix = prefix + " - "
	} else {
		newreg.prefix = r.prefix + prefix + " - "
	}
	return newreg
}

func (r SimpleRegistry) GetPrefix() string {
	return r.prefix
}

func (r SimpleRegistry) Count(name string) {
	if err := r.backend.Count(r.makeFname(name)); err != nil {
		r.errorDebug(err, name)
	}
}

func (r SimpleRegistry) CountMult(name string, count int) {
	if err := r.backend.CountMult(r.makeFname(name), count); err != nil {
		r.errorDebug(err, name)
	}
}

func (r SimpleRegistry) ValueInt(name string, value int) {
	r.Value(name, float64(value))
}

func (r SimpleRegistry) Value(name string, value float64) {
	if err := r.backend.Value(r.makeFname(name), value); err != nil {
		r.errorDebug(err, name)
	}
}

func (r SimpleRegistry) Shutdown() {
	r.log.Info("shutting down stats backend")
	r.backend.Shutdown()
}

type BackendType string

const (
	STATHAT BackendType = "STATHAT"
	MOCK    BackendType = "MOCK"
)

func NewBackend(btype BackendType, config interface{}) (Backend, error) {
	switch btype {
	case STATHAT:
		backend, err := newStathatRegistry(config)
		if err != nil {
			return nil, err
		} else {
			return backend, nil
		}
	case MOCK:
		return mockBackend{}, nil
	default:
		return nil, errors.New("unknown stast registry type")
	}
}

func NewSimpleRegistry(backend Backend, log rpc.LogOutput) Registry {
	return SimpleRegistry{backend: backend, log: log}
}

func NewSimpleRegistryWithPrefix(backend Backend, prefix string, log rpc.LogOutput) Registry {
	return SimpleRegistry{backend: backend, prefix: prefix, log: log}
}

type DummyRegistry struct{}

func (r DummyRegistry) Count(name string)                {}
func (r DummyRegistry) CountMult(name string, count int) {}
func (r DummyRegistry) Value(name string, value float64) {}
func (r DummyRegistry) ValueInt(name string, value int)  {}
func (r DummyRegistry) SetPrefix(prefix string) Registry {
	return r
}
func (r DummyRegistry) Shutdown()         {}
func (r DummyRegistry) GetPrefix() string { return "" }

var _ Registry = DummyRegistry{}

type mockBackend struct{}

func (m mockBackend) Count(name string) error                { return nil }
func (m mockBackend) CountMult(name string, count int) error { return nil }
func (m mockBackend) Value(name string, value float64) error { return nil }
func (m mockBackend) Shutdown()                              {}
func (m mockBackend) GetPrefix() string                      { return "" }

var _ Backend = mockBackend{}
