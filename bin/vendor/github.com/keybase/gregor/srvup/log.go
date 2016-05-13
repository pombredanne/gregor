// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package srvup

import "log"

// Logger specifies the log interface that srvup needs to operate.
type Logger interface {
	Error(format string, args ...interface{})
	Warning(format string, args ...interface{})
	Info(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

type defaultLogger struct{}

func (d defaultLogger) Error(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (d defaultLogger) Warning(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (d defaultLogger) Info(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (d defaultLogger) Debug(format string, args ...interface{}) {
	log.Printf(format, args...)
}
