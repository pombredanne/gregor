package bin

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

type StandardLogger struct {
	inner  *logging.Logger
	module string
}

var _ rpc.LogOutput = (*StandardLogger)(nil)

func (g *StandardLogger) Error(s string, args ...interface{}) {
	g.inner.Errorf(s, args...)
}
func (g *StandardLogger) Warning(s string, args ...interface{}) {
	g.inner.Warningf(s, args...)
}
func (g *StandardLogger) Info(s string, args ...interface{}) {
	g.inner.Infof(s, args...)
}
func (g *StandardLogger) Debug(s string, args ...interface{}) {
	g.inner.Debugf(s, args...)
}
func (g *StandardLogger) Profile(s string, args ...interface{}) {
	g.inner.Debugf(s, args...)
}

func (g *StandardLogger) Configure(debug bool) {
	if debug {
		logging.SetLevel(logging.DEBUG, g.module)
	} else {
		logging.SetLevel(logging.INFO, g.module)
	}
}

var noColorFormat = `%{level:.4s} %{time:15:04:05.000} %{shortfile} %{message}`

var colorFormat = "%{color}" + noColorFormat + "%{color:reset}"

func NewLogger(module string) *StandardLogger {
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

	logger := logging.MustGetLogger(module)
	logger.SetBackend(logging.MultiLogger(backends...))
	logger.ExtraCalldepth = 1
	return &StandardLogger{logger, module}
}

type RPCLogOptions struct {
	clientTrace    bool
	serverTrace    bool
	profile        bool
	verboseTrace   bool
	connectionInfo bool
	noAddress      bool
	log            rpc.LogOutput
}

func (r *RPCLogOptions) ShowAddress() bool    { return !r.noAddress }
func (r *RPCLogOptions) ShowArg() bool        { return r.verboseTrace }
func (r *RPCLogOptions) ShowResult() bool     { return r.verboseTrace }
func (r *RPCLogOptions) Profile() bool        { return r.profile }
func (r *RPCLogOptions) ClientTrace() bool    { return r.clientTrace }
func (r *RPCLogOptions) ServerTrace() bool    { return r.serverTrace }
func (r *RPCLogOptions) TransportStart() bool { return r.connectionInfo }

func NewRPCLogOptions(opts string, log rpc.LogOutput) rpc.LogOptions {
	var r RPCLogOptions
	r.clientTrace = false
	r.serverTrace = false
	r.profile = false
	r.verboseTrace = false
	r.connectionInfo = false
	r.noAddress = false
	r.log = log
	for _, c := range opts {
		switch c {
		case 'A':
			r.noAddress = true
		case 'c':
			r.clientTrace = true
		case 's':
			r.serverTrace = true
		case 'v':
			r.verboseTrace = true
		case 'i':
			r.connectionInfo = true
		case 'p':
			r.profile = true
		default:
			r.log.Warning("Unknown local RPC logging flag: %c", c)
		}
	}
	return &r
}
