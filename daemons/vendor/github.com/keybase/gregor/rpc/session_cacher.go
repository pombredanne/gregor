package rpc

import (
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

type authResp struct {
	res    gregor1.AuthResult
	expiry time.Time
}

type expirySt struct {
	sid    gregor1.SessionID
	expiry time.Time
}

// SessionCacher implements gregor1.AuthInterface with a cache of authenticated sessions.
type SessionCacher struct {
	parent      gregor1.AuthInterface
	cl          clockwork.Clock
	timeout     time.Duration
	sessions    map[gregor1.SessionToken]*gregor1.AuthResult
	sessionIDs  map[gregor1.SessionID]gregor1.SessionToken
	reqCh       chan request
	expiryQueue []expirySt
}

// NewSessionCacher creates a new AuthInterface that caches sessions for the given timeout.
func NewSessionCacher(a gregor1.AuthInterface, cl clockwork.Clock, timeout time.Duration) *SessionCacher {
	sc := &SessionCacher{
		parent:     a,
		cl:         cl,
		timeout:    timeout,
		sessions:   make(map[gregor1.SessionToken]*gregor1.AuthResult),
		sessionIDs: make(map[gregor1.SessionID]gregor1.SessionToken),
		reqCh:      make(chan request),
	}
	go sc.requestHandler()
	return sc
}

type request interface{}

type setResReq struct {
	tok gregor1.SessionToken
	res *gregor1.AuthResult
}

func (sc *SessionCacher) setRes(tok gregor1.SessionToken, res *gregor1.AuthResult) {
	sc.sessions[tok] = res
	sc.sessionIDs[res.Sid] = tok
}

type deleteSIDReq struct {
	sid gregor1.SessionID
}

func (sc *SessionCacher) deleteSID(sid gregor1.SessionID) {
	if tok, ok := sc.sessionIDs[sid]; ok {
		if _, ok := sc.sessions[tok]; ok {
			delete(sc.sessions, sc.sessionIDs[sid])
		}
		delete(sc.sessionIDs, sid)
	}
}

type readTokReq struct {
	tok  gregor1.SessionToken
	resp chan *gregor1.AuthResult
}

type sizeReq struct {
	resp chan int
}

// clearExpiryQueue removes all currently expired sessions.
func (sc *SessionCacher) clearExpiryQueue() {
	now := sc.cl.Now()
	for _, e := range sc.expiryQueue {
		if now.Before(e.expiry) {
			return
		}

		sc.deleteSID(sc.expiryQueue[0].sid)
		sc.expiryQueue = sc.expiryQueue[1:]
	}
}

// requestHandler runs in a single goroutine, handling all access requests and
// periodic expiry of sessions.
func (sc *SessionCacher) requestHandler() {
	for req := range sc.reqCh {
		sc.clearExpiryQueue()
		switch req := req.(type) {
		case readTokReq:
			req.resp <- sc.sessions[req.tok]
		case setResReq:
			expiry := sc.cl.Now().Add(sc.timeout)
			sc.expiryQueue = append(sc.expiryQueue, expirySt{req.res.Sid, expiry})
			sc.setRes(req.tok, req.res)
		case deleteSIDReq:
			sc.deleteSID(req.sid)
		case sizeReq:
			req.resp <- len(sc.sessions)
		}
	}
}

// Close allows the SessionCacher to be garbage collected.
func (sc *SessionCacher) Close() {
	close(sc.reqCh)
}

// Size returns the number of sessions currently in the cache.
func (sc *SessionCacher) Size() int {
	respCh := make(chan int)
	sc.reqCh <- sizeReq{respCh}
	return <-respCh
}

// AuthenticateSessionToken authenticates a given session token, first against
// the cache and the, if that fails, against the parent AuthInterface.
func (sc *SessionCacher) AuthenticateSessionToken(ctx context.Context, tok gregor1.SessionToken) (res gregor1.AuthResult, err error) {
	respCh := make(chan *gregor1.AuthResult)
	select {
	case sc.reqCh <- readTokReq{tok, respCh}:
	case <-ctx.Done():
		err = ctx.Err()
		return
	}

	select {
	case resp := <-respCh:
		if resp != nil {
			res = *resp
			return
		}
	case <-ctx.Done():
		err = ctx.Err()
		return
	}

	if res, err = sc.parent.AuthenticateSessionToken(ctx, tok); err == nil {
		select {
		case sc.reqCh <- setResReq{tok, &res}:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	return
}

// RevokeSessionIDs revokes the given session IDs in the cache and the parent.
func (sc *SessionCacher) RevokeSessionIDs(ctx context.Context, sessionIDs []gregor1.SessionID) error {
	for _, sid := range sessionIDs {
		select {
		case sc.reqCh <- deleteSIDReq{sid}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return sc.parent.RevokeSessionIDs(ctx, sessionIDs)
}

var _ gregor1.AuthInterface = (*SessionCacher)(nil)
