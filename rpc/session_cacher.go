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

// SessionCacher implements gregor1.AuthInterface with a cache of authenticated sessions.
type SessionCacher struct {
	parent     gregor1.AuthInterface
	cl         clockwork.Clock
	timeout    time.Duration
	sessions   map[gregor1.SessionToken]authResp
	sessionIDs map[gregor1.SessionID]gregor1.SessionToken
	reqCh      chan request
}

// NewSessionCacher creates a new AuthInterface that caches sessions for the given timeout.
func NewSessionCacher(a gregor1.AuthInterface, cl clockwork.Clock, timeout time.Duration) *SessionCacher {
	sc := &SessionCacher{
		parent:     a,
		cl:         cl,
		timeout:    timeout,
		sessions:   make(map[gregor1.SessionToken]authResp),
		sessionIDs: make(map[gregor1.SessionID]gregor1.SessionToken),
		reqCh:      make(chan request),
	}
	go sc.requestHandler()
	return sc
}

type request interface{}

type readTokReq struct {
	tok  gregor1.SessionToken
	resp chan *gregor1.AuthResult
}

func (sc *SessionCacher) readTok(tok gregor1.SessionToken) *gregor1.AuthResult {
	resp, ok := sc.sessions[tok]
	if ok && !resp.expiry.Before(sc.cl.Now()) {
		return &resp.res
	}
	return nil
}

type setResReq struct {
	tok gregor1.SessionToken
	res gregor1.AuthResult
}

func (sc *SessionCacher) setRes(tok gregor1.SessionToken, res gregor1.AuthResult, expiry time.Time) {
	sc.sessions[tok] = authResp{res, expiry}
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

type expirySt struct {
	sid    gregor1.SessionID
	expiry time.Time
}

// requestHandler runs in a single goroutine, handling all access requests and
// periodic expiry of sessions.
func (sc *SessionCacher) requestHandler() {
	var expiryCh <-chan time.Time
	var expiryQueue []expirySt
	for {
		select {
		case <-expiryCh:
			sc.deleteSID(expiryQueue[0].sid)
			expiryQueue = expiryQueue[1:]
			if len(expiryQueue) > 0 {
				expiryCh = sc.cl.After(expiryQueue[0].expiry.Sub(sc.cl.Now()))
			}
		case req := <-sc.reqCh:
			switch req := req.(type) {
			case readTokReq:
				req.resp <- sc.readTok(req.tok)
			case setResReq:
				if len(expiryQueue) == 0 {
					expiryCh = sc.cl.After(sc.timeout)
				}
				e := expirySt{
					sid:    req.res.Sid,
					expiry: sc.cl.Now().Add(sc.timeout),
				}
				expiryQueue = append(expiryQueue, e)
				sc.setRes(req.tok, req.res, e.expiry)
			case deleteSIDReq:
				sc.deleteSID(req.sid)
			default:
				// req = nil if sc.Close() has been called.
				return
			}
		}
	}
}

func (sc *SessionCacher) Close() {
	close(sc.reqCh)
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
		case sc.reqCh <- setResReq{tok, res}:
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
