package main

import (
	"database/sql"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/bin"
	"github.com/keybase/gregor/protocol/gregor1"
	server "github.com/keybase/gregor/rpc/server"
	"github.com/keybase/gregor/storage"
	"golang.org/x/net/context"
)

func main() {
	log := bin.NewLogger("gregord")

	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(1)
	}

	rpcopts := rpc.NewStandardLogOptions(opts.RPCDebug, log)
	log.Configure(opts.Debug)
	log.Debug("Options Parsed. Creating server...")
	srv := server.NewServer(log)

	if opts.MockAuth {
		srv.SetAuthenticator(mockAuth{})
	} else {
		log.Debug("Dialing session server %s", opts.SessionServer.String())
		conn, err := opts.SessionServer.Dial()
		if err != nil {
			log.Error("%#v", err)
			os.Exit(2)
		}
		log.Debug("Setting authenticator")

		Cli := rpc.NewClient(rpc.NewTransport(conn, rpc.NewSimpleLogFactory(log, rpcopts), keybase1.WrapError), keybase1.ErrorUnwrapper{})
		sc := server.NewSessionCacher(gregor1.AuthClient{Cli}, clockwork.NewRealClock(), 10*time.Minute)
		srv.SetAuthenticator(sc)
		defer sc.Close()
	}

	log.Debug("Connect to MySQL DB at %s", opts.MysqlDSN)
	db, err := sql.Open("mysql", opts.MysqlDSN)

	if err != nil {
		log.Error("%#v", err)
		os.Exit(3)
	}
	defer func() {
		log.Info("DB close on clean shutdown")
		db.Close()
	}()

	sm := storage.NewMySQLEngine(db, gregor1.ObjFactory{})
	srv.SetStorageStateMachine(sm)
	go srv.Serve()

	log.Debug("Calling mainServer.listenAndServe()")
	log.Error("%#v", newMainServer(opts, srv).listenAndServe())
	os.Exit(4)
}

type mockAuth struct{}

func (m mockAuth) AuthenticateSessionToken(_ context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {
	return gregor1.AuthResult{Uid: gregor1.UID("gooduid"), Sid: gregor1.SessionID("1")}, nil
}
func (m mockAuth) RevokeSessionIDs(_ context.Context, sessionIDs []gregor1.SessionID) error {
	return nil
}

var _ gregor1.AuthInterface = mockAuth{}
