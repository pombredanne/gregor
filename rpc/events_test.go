package rpc

import (
	"github.com/keybase/gregor/protocol/gregor1"
)

// events is a type used to track various events in the gregor rpc
// system.  Currently for testing purposes only.
//
// The addEvent* functions below are meant to be noops except when
// newEvents() is called at the beginning of a test.
type events struct {
	connCreated     chan gregor1.UID
	connDestroyed   chan gregor1.UID
	perUIDCreated   chan gregor1.UID
	perUIDDestroyed chan gregor1.UID
	bcastSent       chan gregor1.Message
}

func newEvents() *events {
	return &events{
		connCreated:     make(chan gregor1.UID, 100),
		connDestroyed:   make(chan gregor1.UID, 100),
		perUIDCreated:   make(chan gregor1.UID, 100),
		perUIDDestroyed: make(chan gregor1.UID, 100),
		bcastSent:       make(chan gregor1.Message, 100),
	}
}

func (e *events) connectionCreated(uid gregor1.UID) {
	e.connCreated <- uid
}

func (e *events) connectionDestroyed(uid gregor1.UID) {
	e.connDestroyed <- uid
}

func (e *events) uidServerCreated(uid gregor1.UID) {
	e.perUIDCreated <- uid
}

func (e *events) uidServerDestroyed(uid gregor1.UID) {
	e.perUIDDestroyed <- uid
}

func (e *events) broadcastSent(m gregor1.Message) {
	e.bcastSent <- m
}
