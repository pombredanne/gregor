package test

import (
	"github.com/keybase/gregor/protocol/gregor1"
)

// Events is a type used to track various events in the gregor rpc
// system.  Currently for testing purposes only.
type Events struct {
	ConnCreated     chan gregor1.UID
	ConnDestroyed   chan gregor1.UID
	PerUIDCreated   chan gregor1.UID
	PerUIDDestroyed chan gregor1.UID
	BcastSent       chan gregor1.Message
	PubSent         chan gregor1.Message
}

func NewEvents() *Events {
	return &Events{
		ConnCreated:     make(chan gregor1.UID, 100),
		ConnDestroyed:   make(chan gregor1.UID, 100),
		PerUIDCreated:   make(chan gregor1.UID, 100),
		PerUIDDestroyed: make(chan gregor1.UID, 100),
		BcastSent:       make(chan gregor1.Message, 100),
		PubSent:         make(chan gregor1.Message, 100),
	}
}

func (e *Events) ConnectionCreated(uid gregor1.UID) {
	e.ConnCreated <- uid
}

func (e *Events) ConnectionDestroyed(uid gregor1.UID) {
	e.ConnDestroyed <- uid
}

func (e *Events) UIDServerCreated(uid gregor1.UID) {
	e.PerUIDCreated <- uid
}

func (e *Events) UIDServerDestroyed(uid gregor1.UID) {
	e.PerUIDDestroyed <- uid
}

func (e *Events) BroadcastSent(m gregor1.Message) {
	e.BcastSent <- m
}

func (e *Events) PublishSent(m gregor1.Message) {
	e.PubSent <- m
}
