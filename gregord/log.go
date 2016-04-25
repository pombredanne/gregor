package main

import (
	"fmt"
	"log/syslog"
	"os"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	logging "github.com/keybase/go-logging"
	"golang.org/x/crypto/ssh/terminal"
)

// The gregor libraries don't depend on any particular logging implementation.
// Rather they depend on the go-framed-msgpack-rpc library's LogOutput
// interface, and they take implementation of that provided by the caller. This
// allows client code to pass in the client's Logger object, for gregor code
// running inside the client. Here in the server we implement the LogOutput
// interface using the go-logging library.

type GoLoggingWrapperForRPC struct {
	inner *logging.Logger
}

var _ rpc.LogOutput = (*GoLoggingWrapperForRPC)(nil)

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
