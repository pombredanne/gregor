package rpc

import (
	protocol "github.com/keybase/gregor/keybase/protocol/go"
	context "golang.org/x/net/context"
)

type Authenticator interface {
	Authenticate(ctx context.Context, tok protocol.AuthToken) (protocol.UID, protocol.SessionID, error)
}
