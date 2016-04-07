package rpc

import (
	protocol "github.com/keybase/gregor/protocol/go"
)

// events is a type used to track various events in the gregor rpc
// system.  Currently for testing purposes only.
//
// The addEvent* functions below are meant to be noops except when
// newEvents() is called at the beginning of a test.
type events struct {
	connCreated     chan protocol.UID
	connDestroyed   chan protocol.UID
	perUIDCreated   chan protocol.UID
	perUIDDestroyed chan protocol.UID
	bcastSent       chan protocol.Message
}

func newEvents() *events {
	return &events{
		connCreated:     make(chan protocol.UID, 100),
		connDestroyed:   make(chan protocol.UID, 100),
		perUIDCreated:   make(chan protocol.UID, 100),
		perUIDDestroyed: make(chan protocol.UID, 100),
		bcastSent:       make(chan protocol.Message, 100),
	}
}

func (e *events) connectionCreated(uid protocol.UID) {
	e.connCreated <- uid
}

func (e *events) connectionDestroyed(uid protocol.UID) {
	e.connDestroyed <- uid
}

func (e *events) uidServerCreated(uid protocol.UID) {
	e.perUIDCreated <- uid
}

func (e *events) uidServerDestroyed(uid protocol.UID) {
	e.perUIDDestroyed <- uid
}

func (e *events) broadcastSent(m protocol.Message) {
	e.bcastSent <- m
}
