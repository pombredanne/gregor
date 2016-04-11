package rpc

import (
	"time"

	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

// sessionCacher implements gregor1.AuthInterface with a cache of authenticated sessions.
type sessionCacher struct {
	a          gregor1.AuthInterface
	timeout    time.Duration
	sessions   map[gregor1.SessionToken]gregor1.AuthResult
	sessionIDs map[gregor1.SessionID]gregor1.SessionToken
	reqCh      chan request
}

// NewSessionCacher creates a new AuthInterface that caches sessions for the given timeout.
func NewSessionCacher(a gregor1.AuthInterface, timeout time.Duration) gregor1.AuthInterface {
	sc := &sessionCacher{
		a:          a,
		timeout:    timeout,
		sessions:   make(map[gregor1.SessionToken]gregor1.AuthResult),
		sessionIDs: make(map[gregor1.SessionID]gregor1.SessionToken),
		reqCh:      make(chan request),
	}
	go sc.requestHandler()
	return sc
}

type request interface{}

type authResp struct {
	res gregor1.AuthResult
	ok  bool
}

type readTokReq struct {
	tok  gregor1.SessionToken
	resp chan authResp
}

func (sc *sessionCacher) readTok(tok gregor1.SessionToken) (resp authResp) {
	resp.res, resp.ok = sc.sessions[tok]
	return
}

type setResReq struct {
	tok gregor1.SessionToken
	res gregor1.AuthResult
}

func (sc *sessionCacher) setRes(tok gregor1.SessionToken, res gregor1.AuthResult) {
	sc.sessions[tok] = res
	sc.sessionIDs[res.Sid] = tok
}

type deleteSIDReq struct {
	sid gregor1.SessionID
}

func (sc *sessionCacher) deleteSID(sid gregor1.SessionID) {
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
func (sc *sessionCacher) requestHandler() {
	var expiryCh <-chan time.Time
	var expiryQueue []expirySt
	for {
		select {
		case <-expiryCh:
			sc.deleteSID(expiryQueue[0].sid)
			expiryQueue = expiryQueue[1:]
			if len(expiryQueue) > 0 {
				expiryCh = time.After(expiryQueue[0].expiry.Sub(time.Now()))
			}
		case req := <-sc.reqCh:
			switch req := req.(type) {
			case readTokReq:
				req.resp <- sc.readTok(req.tok)
			case setResReq:
				sc.setRes(req.tok, req.res)
				e := expirySt{
					sid:    req.res.Sid,
					expiry: time.Now().Add(sc.timeout),
				}
				if len(expiryQueue) == 0 {
					expiryCh = time.After(e.expiry.Sub(time.Now()))
				}
				expiryQueue = append(expiryQueue, e)
			case deleteSIDReq:
				sc.deleteSID(req.sid)
			}
		}
	}
}

func (sc *sessionCacher) AuthenticateSessionToken(ctx context.Context, tok gregor1.SessionToken) (res gregor1.AuthResult, err error) {
	respCh := make(chan authResp)
	select {
	case sc.reqCh <- readTokReq{tok, respCh}:
	case <-ctx.Done():
		err = ctx.Err()
		return
	}

	select {
	case resp := <-respCh:
		if resp.ok {
			res = resp.res
			return
		}
	case <-ctx.Done():
		err = ctx.Err()
		return
	}

	if res, err = sc.a.AuthenticateSessionToken(ctx, tok); err == nil {
		select {
		case sc.reqCh <- setResReq{tok, res}:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	return
}

func (sc *sessionCacher) RevokeSessionIDs(ctx context.Context, sessionIDs []gregor1.SessionID) error {
	for _, sid := range sessionIDs {
		select {
		case sc.reqCh <- deleteSIDReq{sid}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return sc.a.RevokeSessionIDs(ctx, sessionIDs)
}

var _ gregor1.AuthInterface = (*sessionCacher)(nil)
