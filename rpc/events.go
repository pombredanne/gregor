package rpc

import (
	"github.com/keybase/gregor/protocol/gregor1"
)

type eventHandler interface {
	connectionCreated(gregor1.UID)
	connectionDestroyed(gregor1.UID)
	uidServerCreated(gregor1.UID)
	uidServerDestroyed(gregor1.UID)
	broadcastSent(gregor1.Message)
}
