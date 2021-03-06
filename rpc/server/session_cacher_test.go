package rpc

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/stats"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func checkBad(t *testing.T, a gregor1.AuthInterface, tok gregor1.SessionToken) {
	_, err := a.AuthenticateSessionToken(context.TODO(), tok)
	require.NotNil(t, err, "badToken authenticated")
}

func checkGood(t *testing.T, a gregor1.AuthInterface, tok gregor1.SessionToken, uid gregor1.UID) {
	res, err := a.AuthenticateSessionToken(context.TODO(), tok)
	require.Nil(t, err, "no error")
	require.Equal(t, res.Uid, uid, "UIDs equal")
}

func assertCacheSize(t *testing.T, sc *SessionCacher, expected int) {
	require.Equal(t, expected, sc.Size())
}

func TestSessionCacher(t *testing.T) {
	a := mockAuth{
		sessions: map[gregor1.SessionToken]gregor1.AuthResult{
			goodToken: goodResult,
		},
		sessionIDs: map[gregor1.SessionID]gregor1.SessionToken{
			goodSID: goodToken,
		},
	}
	checkBad(t, a, badToken)
	checkGood(t, a, goodToken, goodUID)
	d := 2 * time.Minute
	fc := clockwork.NewFakeClock()
	sc := NewSessionCacher(a, stats.DummyRegistry{}, fc, d)
	defer sc.Close()

	checkBad(t, sc, badToken)
	assertCacheSize(t, sc, 0)
	checkGood(t, sc, goodToken, goodUID)
	assertCacheSize(t, sc, 1)

	// Revoke goodToken
	a.RevokeSessionIDs(context.TODO(), []gregor1.SessionID{goodSID})
	checkBad(t, a, badToken)
	checkBad(t, a, goodToken)

	// cached results linger until timeout
	checkBad(t, sc, badToken)
	checkGood(t, sc, goodToken, goodUID)
	assertCacheSize(t, sc, 1)

	// Advance past timeout, cached results gone
	fc.BlockUntil(1)
	fc.Advance(d)
	fc.BlockUntil(1)
	assertCacheSize(t, sc, 0)
	checkBad(t, sc, goodToken)
}
