package keybase1

import (
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

func (c SessionClient) Authenticate(ctx context.Context, session string) (gregor1.UID, error) {
	res, err := c.VerifySession(ctx, session)
	if err != nil {
		return nil, err
	}
	return gregor1.UID(res.Uid), nil
}

var _ gregor1.AuthInterface = SessionClient{}
