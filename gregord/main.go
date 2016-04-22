package main

import (
	"fmt"
	"log/syslog"
	"os"
	"time"

	"github.com/jonboulle/clockwork"
	keybase1 "github.com/keybase/client/go/protocol"
	"golang.org/x/net/context"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	grpc "github.com/keybase/gregor/rpc"

	logging "github.com/keybase/go-logging"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	logger := newLogger()
	rpcLogger := &GoLoggingWrapperForRPC{logger}

	opts, err := ParseOptions(os.Args)
	if err != nil {
		logger.Fatal(err)
	}

	srv := grpc.NewServer(rpcLogger)

	if opts.MockAuth {
		srv.SetAuthenticator(mockAuth{})
	} else {
		conn, err := opts.SessionServer.Dial()
		if err != nil {
			logger.Fatal(err)
		}
		Cli := rpc.NewClient(rpc.NewTransport(conn, rpc.NewSimpleLogFactory(rpcLogger, nil), keybase1.WrapError), keybase1.ErrorUnwrapper{})
		sc := grpc.NewSessionCacher(gregor1.AuthClient{Cli}, clockwork.NewRealClock(), 10*time.Minute)
		srv.SetAuthenticator(sc)
		defer sc.Close()
	}

	// create a message consumer state machine
	consumer, err := newConsumer(opts.MysqlDSN, rpcLogger)
	if err != nil {
		logger.Fatal(err)
	}
	defer consumer.shutdown()
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

type GoLoggingWrapperForRPC struct {
	inner *logging.Logger
}

func (g *GoLoggingWrapperForRPC) Error(s string, args ...interface{}) {
	g.inner.Errorf(s, args...)
}
func (g *GoLoggingWrapperForRPC) Warning(s string, args ...interface{}) {
	g.inner.Warningf(s, args...)
}
func (g *GoLoggingWrapperForRPC) Info(s string, args ...interface{}) {
	g.inner.Infof(s, args...)
}
func (g *GoLoggingWrapperForRPC) Debug(s string, args ...interface{}) {
	g.inner.Debugf(s, args...)
}
func (g *GoLoggingWrapperForRPC) Profile(s string, args ...interface{}) {
	g.inner.Debugf(s, args...)
}

var noColorFormat = `%{level:.4s} %{time:15:04:05.000} %{shortfile} %{message}`

var colorFormat = "%{color}" + noColorFormat + "%{color:reset}"

func newLogger() *logging.Logger {
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	var format string
	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		format = colorFormat
	} else {
		format = noColorFormat
	}
	formattedBackend := logging.NewBackendFormatter(backend, logging.MustStringFormatter(format))
	backends := []logging.Backend{formattedBackend}

	// Add a syslog backend if the right env vars are present.
	// TODO: *Remove* the stdout backend in this case?
	syslogIP := os.Getenv("LOGGLY_PORT_514_UDP_ADDR")
	syslogPort := os.Getenv("LOGGLY_PORT_514_UDP_PORT")
	if syslogIP != "" && syslogPort != "" {
		syslogURL := syslogIP + ":" + syslogPort
		syslogWriter, err := syslog.Dial("udp", syslogURL, 0, "")
		if err != nil {
			fmt.Println(err)
			fmt.Printf("ERROR failed to connect to syslog at %s\n", syslogURL)
			os.Exit(1)
		}
		fmt.Printf("connected to syslog at %s\n", syslogURL)
		syslogBackend := logging.SyslogBackend{Writer: syslogWriter}
		formattedSyslog := logging.NewBackendFormatter(&syslogBackend, logging.MustStringFormatter(format))
		backends = append(backends, formattedSyslog)
	}

	logger := logging.MustGetLogger("gregord")
	logger.SetBackend(logging.MultiLogger(backends...))
	return logger
}
