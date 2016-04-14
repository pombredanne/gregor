package rpc

import (
	"github.com/keybase/gregor/protocol/gregor1"
)

type EventHandler interface {
	ConnectionCreated(gregor1.UID)
	ConnectionDestroyed(gregor1.UID)
	UIDServerCreated(gregor1.UID)
	UIDServerDestroyed(gregor1.UID)
	BroadcastSent(gregor1.Message)
}
