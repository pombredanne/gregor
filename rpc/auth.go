package rpc

import (
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	protocol "github.com/keybase/gregor/protocol/go"
	"golang.org/x/net/context"
)

// Authenticator is an interface for handling authentication.
type Authenticator interface {
	Authenticate(ctx context.Context, tok string) (protocol.UID, string, error)
}

type AuthClient struct {
	keybase1.QuotaClient
}

func NewAuthClient(Cli rpc.GenericClient) *AuthClient {
	return &AuthClient{keybase1.QuotaClient{Cli}}
}

func (ac *AuthClient) Authenticate(ctx context.Context, tok string) (uid protocol.UID, sid string, err error) {
	res, err := ac.VerifySession(ctx, tok)
	if err != nil {
		return
	}
	uid, sid = res.Uid.ToBytes(), res.Sid
	return
}
