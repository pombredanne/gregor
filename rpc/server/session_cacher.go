package rpc

import (
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/stats"
	"golang.org/x/net/context"
)

type expirySt struct {
	sid    gregor1.SessionID
	expiry time.Time
}

// SessionCacher implements gregor1.AuthInterface with a cache of authenticated sessions.
type SessionCacher struct {
	sync.RWMutex
	parent       gregor1.AuthInterface
	cl           clockwork.Clock
	timeout      time.Duration
	sessions     map[gregor1.SessionToken]*gregor1.AuthResult
	sessionIDs   map[gregor1.SessionID]gregor1.SessionToken
	shutdownCh   chan struct{}
	expiryQueue  []expirySt
	authdHandler *authdHandler
	stats        stats.Registry
}

// NewSessionCacher creates a new AuthInterface that caches sessions for the given timeout.
func NewSessionCacher(a gregor1.AuthInterface, stats stats.Registry, cl clockwork.Clock,
	timeout time.Duration) *SessionCacher {
	sc := &SessionCacher{
		parent:     a,
		cl:         cl,
		timeout:    timeout,
		sessions:   make(map[gregor1.SessionToken]*gregor1.AuthResult),
		sessionIDs: make(map[gregor1.SessionID]gregor1.SessionToken),
		shutdownCh: make(chan struct{}),
		stats:      stats.SetPrefix("session_cacher"),
	}

	go sc.updateStatsLoop()
	go sc.clearExpiryQueueLoop()

	return sc
}

func NewSessionCacherFromURI(uri *rpc.FMPURI, stats stats.Registry, cl clockwork.Clock,
	timeout time.Duration, log rpc.LogOutput, opts rpc.LogOptions,
	refreshInterval time.Duration) *SessionCacher {

	sc := NewSessionCacher(nil, stats, cl, timeout)
	transport := rpc.NewConnectionTransport(uri, rpc.NewSimpleLogFactory(log, opts), keybase1.WrapError)
	sc.authdHandler = NewAuthdHandler(sc, log, refreshInterval)

	log.Debug("Connecting to session server %s", uri.String())
	conn := rpc.NewConnectionWithTransport(sc.authdHandler, transport, keybase1.ErrorUnwrapper{},
		true, keybase1.WrapError, log, nil)
	sc.parent = gregor1.AuthClient{Cli: conn.GetClient()}
	return sc
}

func (sc *SessionCacher) GetSuperToken() gregor1.SessionToken {
	return sc.authdHandler.GetSuperToken()
}

func (sc *SessionCacher) updateStats() {
	sc.stats.ValueInt("cache size", sc.Size())
}

func (sc *SessionCacher) updateStatsLoop() {
	for {
		select {
		case <-sc.shutdownCh:
			return
		case <-time.After(time.Second):
			sc.updateStats()
		}
	}
}

func (sc *SessionCacher) setRes(tok gregor1.SessionToken, res *gregor1.AuthResult) {
	sc.sessions[tok] = res
	sc.sessionIDs[res.Sid] = tok
}

func (sc *SessionCacher) deleteSID(sid gregor1.SessionID) {
	if tok, ok := sc.sessionIDs[sid]; ok {
		if _, ok := sc.sessions[tok]; ok {
			delete(sc.sessions, sc.sessionIDs[sid])
		}
		delete(sc.sessionIDs, sid)
	}
}

func (sc *SessionCacher) clearExpiryQueueLoop() {
	for {
		select {
		case <-sc.cl.After(10 * time.Second):
			sc.clearExpiryQueue()
		case <-sc.shutdownCh:
			return
		}
	}
}

// clearExpiryQueue removes all currently expired sessions.
func (sc *SessionCacher) clearExpiryQueue() {
	sc.Lock()
	defer sc.Unlock()

	now := sc.cl.Now()
	for _, e := range sc.expiryQueue {
		if now.Before(e.expiry) {
			return
		}

		sc.deleteSID(sc.expiryQueue[0].sid)
		sc.expiryQueue = sc.expiryQueue[1:]
	}
}

// Close allows the SessionCacher to be garbage collected.
func (sc *SessionCacher) Close() {
	close(sc.shutdownCh)
	if sc.authdHandler != nil {
		sc.authdHandler.Close()
	}
}

// Size returns the number of sessions currently in the cache.
func (sc *SessionCacher) Size() int {
	sc.RLock()
	defer sc.RUnlock()
	return len(sc.sessions)
}

// AuthenticateSessionToken authenticates a given session token, first against
// the cache and the, if that fails, against the parent AuthInterface.
func (sc *SessionCacher) AuthenticateSessionToken(ctx context.Context, tok gregor1.SessionToken) (res gregor1.AuthResult, err error) {

	// Fire off request for a cached session
	sc.stats.Count("AuthenticateSessionToken")
	sc.RLock()
	pres := sc.sessions[tok]
	sc.RUnlock()

	// Return res if we hit the cache
	if pres != nil {
		res = *pres
		sc.stats.Count("AuthenticateSessionToken - cache hit")
		return
	}

	// Call out to authd and store the result
	if res, err = sc.parent.AuthenticateSessionToken(ctx, tok); err == nil {
		sc.Lock()
		expiry := sc.cl.Now().Add(sc.timeout)
		sc.expiryQueue = append(sc.expiryQueue, expirySt{res.Sid, expiry})
		sc.setRes(tok, &res)
		sc.Unlock()
	}

	return
}

// RevokeSessionIDs revokes the given session IDs in the cache.
func (sc *SessionCacher) RevokeSessionIDs(ctx context.Context, sessionIDs []gregor1.SessionID) error {
	sc.Lock()
	defer sc.Unlock()
	for _, sid := range sessionIDs {
		sc.deleteSID(sid)
	}
	return nil
}

var _ gregor1.AuthInterface = (*SessionCacher)(nil)
var _ gregor1.AuthUpdateInterface = (*SessionCacher)(nil)
