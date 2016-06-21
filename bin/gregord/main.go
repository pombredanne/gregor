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
	"github.com/keybase/gregor/stats"
	"github.com/keybase/gregor/storage"
)

// Main entry point or gregord
func main() {
	log := bin.NewLogger("gregord")
	rc, err := mainInner(log)
	if err != nil {
		log.Error("%s", err)
	}
	os.Exit(rc)
}

func mainInner(log *bin.StandardLogger) (int, error) {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		return 1, err
	}

	rpcopts := rpc.NewStandardLogOptions(opts.RPCDebug, log)
	log.Configure(opts.Debug)
	log.Debug("Options Parsed. Creating server...")

	// Create server
	srvopts := server.ServerOpts{
		BroadcastTimeout: opts.BroadcastTimeout,
		PublishChSize:    opts.PublishBufferSize,
		NumPublishers:    opts.NumPublishers,
		PublishTimeout:   opts.PublishTimeout,
		StorageHandlers:  opts.StorageHandlers,
		StorageQueueSize: opts.StorageQueueSize,
		TLSConfig:        opts.TLSConfig,
	}
	stats := setupStats(opts, log)
	defer stats.Shutdown()

	srv := server.NewServer(log, clockwork.NewRealClock(), stats, srvopts)

	if opts.MockAuth {
		srv.SetAuthenticator(newMockAuth())
	} else {
		sc := server.NewSessionCacherFromURI(opts.AuthServer, stats, clockwork.NewRealClock(),
			10*time.Minute, log, rpcopts, opts.SuperTokenRefreshInterval)
		defer sc.Close()

		log.Debug("Setting authenticator")
		srv.SetAuthenticator(sc)
	}

	log.Debug("Connect to MySQL DB at %s", opts.MysqlDSN)
	db, err := sql.Open("mysql", opts.MysqlDSN)

	if err != nil {
		return 3, err
	}
	defer func() {
		log.Info("DB close on clean shutdown")
		db.Close()
	}()

	statusGroup, err := setupPubSub(opts, log)
	if err != nil {
		return 3, err
	}

	defer statusGroup.Shutdown()

	srv.SetStatusGroup(statusGroup)

	sm := storage.NewMySQLEngine(db, gregor1.ObjFactory{})
	srv.SetStorageStateMachine(sm)

	log.Debug("Calling mainServer.listenAndServe()")
	err = newMainServer(opts, srv).listenAndServe()
	if err != nil {
		return 4, err
	}
	return 0, nil
}

func setupStats(opts *Options, log rpc.LogOutput) stats.Registry {
	// Create stats registry
	var statreg stats.Registry
	if opts.StatsBackend != nil {
		statreg = stats.NewSimpleRegistry(opts.StatsBackend, log).SetPrefix("gregor")
		if opts.RunMode != "" && opts.RunMode != "prod" {
			statreg = statreg.SetPrefix("[" + opts.RunMode + "]")
		}
	} else {
		statreg = stats.DummyRegistry{}
	}
	return statreg
}

func setupPubSub(opts *Options, log *bin.StandardLogger) (*srvup.Status, error) {
	mstore, err := srvup.NewStorageMysql(opts.MysqlDSN, log)
	if err != nil {
		return nil, err
	}
	statusGroup := srvup.New("gregord", opts.HeartbeatInterval, opts.AliveThreshold, mstore, log)

	alive, err := statusGroup.Alive()
	if err != nil {
		// bad enough to quit:
		return nil, err
	}

	// Tag logger with our ID
	log.SetMyID(statusGroup.MyID())

	// start sending heartbeats
	externalAddr := opts.BindAddress
	if len(opts.IncomingAddress) > 0 {
		// only use opts.IncomingAddress if it is set
		externalAddr = opts.IncomingAddress
	}

	// put a TLS URI in if we are listening using it
	var externalURI string
	if opts.TLSConfig != nil {
		externalURI = "fmprpc+tls://" + externalAddr
	} else {
		externalURI = "fmprpc://" + externalAddr
	}

	log.Debug("starting heartbeat loop for URI %s", externalURI)
	statusGroup.HeartbeatLoop(externalURI)

	if len(alive) > 0 {
		// there are other gregors up
		log.Debug("existing gregord servers: %v", alive)
		log.Debug("sleeping for alive threshold (%s) before starting server", opts.AliveThreshold)
		time.Sleep(opts.AliveThreshold)
		log.Debug("sleep complete, proceeding with server initialization")
	} else {
		log.Debug("self is first gregord server alive, proceeding directly to server initialization")
	}

	return statusGroup, nil
}
