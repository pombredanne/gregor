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
)

// Main entry point or gregord
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
	srv := server.NewServer(log, opts.BroadcastTimeout)

	if opts.MockAuth {
		srv.SetAuthenticator(newMockAuth())
	} else {
		transport := NewConnTransport(log, rpcopts, opts.SessionServer)
		handler := NewAuthdHandler(log)

		log.Debug("Connecting to session server %s", opts.SessionServer.String())
		rpc.NewConnectionWithTransport(&handler, transport, keybase1.ErrorUnwrapper{},
			true, keybase1.WrapError, log, nil)

		cli := <-handler.connectCh
		sc := server.NewSessionCacher(gregor1.AuthClient{cli}, clockwork.NewRealClock(), 10*time.Minute)
		defer sc.Close()
		go func() {
			for {
				log.Debug("Setting authenticator")
				srv.SetAuthenticator(sc)

				cli = <-handler.connectCh
				sc.ResetAuthInterface(gregor1.AuthClient{cli})
			}
		}()
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
