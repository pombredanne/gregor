package rpc

import (
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

// sessionCacher implements gregor1.AuthInterface with a cache of authenticated sessions.
type sessionCacher struct {
	a          gregor1.AuthInterface
	sessions   map[gregor1.SessionToken]gregor1.AuthResult
	sessionIDs map[gregor1.SessionID]gregor1.SessionToken
}

func NewSessionCacher(a gregor1.AuthInterface) gregor1.AuthInterface {
	return &sessionCacher{
		a:          a,
		sessions:   make(map[gregor1.SessionToken]gregor1.AuthResult),
		sessionIDs: make(map[gregor1.SessionID]gregor1.SessionToken),
	}
}

func (sc *sessionCacher) AuthenticateSessionToken(ctx context.Context, tok gregor1.SessionToken) (res gregor1.AuthResult, err error) {
	var ok bool
	if res, ok = sc.sessions[tok]; ok {
		return
	}

	if res, err = sc.a.AuthenticateSessionToken(ctx, tok); err == nil {
		sc.sessions[tok] = res
		sc.sessionIDs[res.Sid] = tok
	}
	return
}

func (sc *sessionCacher) RevokeSessionIDs(ctx context.Context, sessionIDs []gregor1.SessionID) error {
	for _, sid := range sessionIDs {
		if tok, ok := sc.sessionIDs[sid]; ok {
			if _, ok := sc.sessions[tok]; ok {
				delete(sc.sessions, sc.sessionIDs[sid])
			}
			delete(sc.sessionIDs, sid)
		}
	}
	return sc.a.RevokeSessionIDs(ctx, sessionIDs)
}

var _ gregor1.AuthInterface = (*sessionCacher)(nil)
