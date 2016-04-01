package main

import (
	"os"

	protocol "github.com/keybase/gregor/protocol/go"
	"github.com/keybase/gregor/rpc"
	"golang.org/x/net/context"
)

type dummyauth struct{}

func (m dummyauth) Authenticate(_ context.Context, tok protocol.AuthToken) (protocol.UID, protocol.SessionID, error) {
	return protocol.UID{}, protocol.SessionID(""), nil
}

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		errorf("%s\n", err)
		os.Exit(2)
	}
	srv := rpc.NewServer()
	srv.SetAuthenticator(&dummyauth{})
	err = newMainServer(opts, srv).listenAndServe()
	if err != nil {
		errorf("%s\n", err)
		os.Exit(2)
	}
	return
}
