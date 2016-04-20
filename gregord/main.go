package main

import (
	"log/syslog"
	"net"
	"os"
	"time"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/go-logging"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"
	"golang.org/x/net/context"
)

var logger *logging.Logger

func init() {
	logger = logging.MustGetLogger("gregord")
	syslogIP, syslogPort := os.Getenv("LOGGLY_PORT_514_UDP_ADDR"), os.Getenv("LOGGLY_PORT_514_UDP_PORT")
	if syslogIP != "" && syslogPort != "" {
		w, err := syslog.Dial("udp", net.JoinHostPort(syslogIP, syslogPort), syslog.LOG_DEBUG, "[gregord] ")
		if err != nil {
			logger.Errorf("unable to dial sysloger: %v", err)
		}

		logger.SetBackend(logging.AddModuleLevel(&logging.SyslogBackend{w}))
		logger.Debug("syslog logging enabled")
	}

}

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		logger.Fatal(err)
	}

	srv := grpc.NewServer()

	if opts.MockAuth {
		srv.SetAuthenticator(mockAuth{})
	} else {
		logger.Debugf("dialing authd at %s", opts.SessionServer.String())
		conn, err := opts.SessionServer.Dial()
		if err != nil {
			logger.Fatal(err)
		}
		Cli := rpc.NewClient(rpc.NewTransport(conn, nil, keybase1.WrapError), keybase1.ErrorUnwrapper{})
		sc := grpc.NewSessionCacher(gregor1.AuthClient{Cli}, clockwork.NewRealClock(), 10*time.Minute)
		srv.SetAuthenticator(sc)
		defer sc.Close()
	}

	// create a message consumer state machine
	consumer, err := newConsumer(opts.MysqlDSN)
	if err != nil {
		logger.Fatal(err)
	}
	defer consumer.shutdown()
	logger.Debug("serving consumer")
	go srv.Serve(consumer)

	logger.Fatal(newMainServer(opts, srv).listenAndServe())
}

type mockAuth struct{}

func (m mockAuth) AuthenticateSessionToken(_ context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {
	return gregor1.AuthResult{Uid: gregor1.UID("gooduid"), Sid: gregor1.SessionID("1")}, nil
}
func (m mockAuth) RevokeSessionIDs(_ context.Context, sessionIDs []gregor1.SessionID) error {
	return nil
}

var _ gregor1.AuthInterface = mockAuth{}
