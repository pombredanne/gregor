// Copyright 2016 Keybase Inc. All rights reserved.
// Use of this source code is governed by a BSD
// license that can be found in the LICENSE file.

package stats

import (
	"errors"
	stathat "github.com/stathat/go"
	"time"
)

type StathatConfig struct {
	ezkey string
}

func NewStathatConfig(ezkey string) StathatConfig {
	return StathatConfig{ezkey: ezkey}
}

type stathatBackend struct {
	config   StathatConfig
	reporter stathat.Reporter
}

var _ Backend = stathatBackend{}

func (s stathatBackend) Count(name string) error {
	return s.reporter.PostEZCountOne(name, s.config.ezkey)
}

func (s stathatBackend) CountMult(name string, count int) error {
	return s.reporter.PostEZCount(name, s.config.ezkey, count)
}

func (s stathatBackend) Value(name string, value float64) error {
	return s.reporter.PostEZValue(name, s.config.ezkey, value)
}

func newStathatRegistry(iconfig interface{}) (Backend, error) {
	config, ok := iconfig.(StathatConfig)
	if ok {
		reporter := stathat.NewBatchReporter(stathat.DefaultReporter, 200*time.Millisecond)
		return stathatBackend{config: config, reporter: reporter}, nil
	} else {
		return nil, errors.New("invalid stathat config")
	}
}
