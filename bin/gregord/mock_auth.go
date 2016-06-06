package main

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/keybase/gregor/protocol/gregor1"
	rpc "github.com/keybase/gregor/rpc/server"
	"golang.org/x/net/context"
)

type mockAuth struct {
	tokens map[gregor1.SessionToken]gregor1.AuthResult
	n      int
}

func newMockAuth() *mockAuth {
	ma := &mockAuth{tokens: make(map[gregor1.SessionToken]gregor1.AuthResult), n: 10}
	ma.tokens[gregor1.SessionToken("anything")] = gregor1.AuthResult{Uid: gregor1.UID("gooduid"), Sid: gregor1.SessionID("1")}
	return ma
}

func (m *mockAuth) AuthenticateSessionToken(_ context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {
	if res, ok := m.tokens[tok]; ok {
		return res, nil
	} else {
		return gregor1.AuthResult{}, fmt.Errorf("No session for token: %s", tok)
	}
}

func (m *mockAuth) newUser() (tok gregor1.SessionToken, auth gregor1.AuthResult, err error) {
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return tok, auth, err
	}
	return m.newUserWithUID(id)
}
func (m *mockAuth) newUserWithUID(id []byte) (tok gregor1.SessionToken, auth gregor1.AuthResult, err error) {
	tok = gregor1.SessionToken(fmt.Sprintf("tok%x", id))
	auth = gregor1.AuthResult{
		Uid: gregor1.UID(id),
		Sid: gregor1.SessionID(fmt.Sprintf("sid%d", m.n)),
	}
	m.n++
	m.tokens[tok] = auth
	return tok, auth, nil
}

func (m *mockAuth) newSuperUser() (tok gregor1.SessionToken, auth gregor1.AuthResult, err error) {
	return m.newUserWithUID(rpc.SuperUID())
}

func (m mockAuth) RevokeSessionIDs(_ context.Context, sessionIDs []gregor1.SessionID) error {
	return nil
}

func (m mockAuth) CreateGregorSuperUserSessionToken(ctx context.Context) (gregor1.SessionToken, error) {
	return "", errors.New("mockAuth super token not implemented")
}

var _ gregor1.AuthInterface = &mockAuth{}
