package rpc

import (
	protocol "github.com/keybase/gregor/protocol/go"
)

type eventHandler interface {
	connectionCreated(protocol.UID)
	connectionDestroyed(protocol.UID)
	uidServerCreated(protocol.UID)
	uidServerDestroyed(protocol.UID)
	broadcastSent(protocol.Message)
}
