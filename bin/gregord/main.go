package main

import (
	"database/sql"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jonboulle/clockwork"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/bin"
	"github.com/keybase/gregor/protocol/gregor1"
	server "github.com/keybase/gregor/rpc/server"
	"github.com/keybase/gregor/srvup"
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
	srv := server.NewServer(log, opts.BroadcastTimeout, opts.StorageHandlers,
		opts.StorageQueueSize)

	if opts.MockAuth {
		srv.SetAuthenticator(newMockAuth())
	} else {
		sc := server.NewSessionCacherFromURI(opts.SessionServer, clockwork.NewRealClock(),
			10*time.Minute, log, rpcopts)
		defer sc.Close()

		log.Debug("Setting authenticator")
		srv.SetAuthenticator(sc)
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

	mstore, err := srvup.NewStorageMysql(opts.MysqlDSN, log)
	if err != nil {
		log.Error("%#v", err)
		os.Exit(3)
	}
	status := srvup.New("gregord", opts.HeartbeatInterval, opts.AliveThreshold, mstore)
	defer status.Shutdown()
	status.HeartbeatLoop(opts.BindAddress)

	sm := storage.NewMySQLEngine(db, gregor1.ObjFactory{})
	srv.SetStorageStateMachine(sm)
	go srv.Serve()

	log.Debug("Calling mainServer.listenAndServe()")
	log.Error("%#v", newMainServer(opts, srv).listenAndServe())
	os.Exit(4)
}
