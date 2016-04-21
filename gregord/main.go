package main

import (
	"os"
	"time"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	"golang.org/x/net/context"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"

	log "github.com/Sirupsen/logrus"
	logrus_syslog "github.com/Sirupsen/logrus/hooks/syslog"
	syslog "log/syslog"
)

func main() {
	initLogging()

	opts, err := ParseOptions(os.Args)
	if err != nil {
		log.Fatal(err)
	}

	srv := grpc.NewServer()

	if opts.MockAuth {
		srv.SetAuthenticator(mockAuth{})
	} else {
		conn, err := opts.SessionServer.Dial()
		if err != nil {
			log.Fatal(err)
		}
		Cli := rpc.NewClient(rpc.NewTransport(conn, grpc.NewLogrusRPCLogFactory(), keybase1.WrapError), keybase1.ErrorUnwrapper{})
		sc := grpc.NewSessionCacher(gregor1.AuthClient{Cli}, clockwork.NewRealClock(), 10*time.Minute)
		srv.SetAuthenticator(sc)
		defer sc.Close()
	}

	// create a message consumer state machine
	consumer, err := newConsumer(opts.MysqlDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer consumer.shutdown()
	go srv.Serve(consumer)

	log.Fatal(newMainServer(opts, srv).listenAndServe())
}

func initLogging() {
	log.SetLevel(log.DebugLevel)
	// Add a syslog hook if the right env vars are set.
	syslogIP := os.Getenv("LOGGLY_PORT_514_UDP_ADDR")
	syslogPort := os.Getenv("LOGGLY_PORT_514_UDP_PORT")
	if syslogIP != "" && syslogPort != "" {
		syslogURL := syslogIP + ":" + syslogPort
		hook, err := logrus_syslog.NewSyslogHook("udp", syslogURL, syslog.LOG_DEBUG, "")
		if err != nil {
			log.Error("Unable to connect to local syslog daemon at %s", syslogURL)
		} else {
			log.Info("Connected to syslog at %s", syslogURL)
			log.AddHook(hook)
		}
	}
}

type mockAuth struct{}

func (m mockAuth) AuthenticateSessionToken(_ context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {
	return gregor1.AuthResult{Uid: gregor1.UID("gooduid"), Sid: gregor1.SessionID("1")}, nil
}
func (m mockAuth) RevokeSessionIDs(_ context.Context, sessionIDs []gregor1.SessionID) error {
	return nil
}

var _ gregor1.AuthInterface = mockAuth{}
