package bin

import (
	"fmt"
	"log/syslog"
	"os"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	logging "github.com/keybase/go-logging"
	"github.com/keybase/gregor/srvup"
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
	myID   *srvup.NodeId
}

var _ rpc.LogOutput = (*StandardLogger)(nil)

// appendID will add in the node ID of the current gregor to all log output.
// Note that there can be tough if g.myID has a % in it, but that should
// never happen since all IDs are hex encoded.
func (g *StandardLogger) appendID(s string) string {
	if g.myID != nil {
		return fmt.Sprintf("[%s] %s", string(*g.myID), s)
	}
	return s
}

func (g *StandardLogger) SetMyID(id srvup.NodeId) {
	g.myID = &id
}

func (g *StandardLogger) Error(s string, args ...interface{}) {
	g.inner.Errorf(g.appendID(s), args...)
}
func (g *StandardLogger) Warning(s string, args ...interface{}) {
	g.inner.Warningf(g.appendID(s), args...)
}
func (g *StandardLogger) Info(s string, args ...interface{}) {
	g.inner.Infof(g.appendID(s), args...)
}
func (g *StandardLogger) Debug(s string, args ...interface{}) {
	g.inner.Debugf(g.appendID(s), args...)
}
func (g *StandardLogger) Profile(s string, args ...interface{}) {
	g.inner.Debugf(g.appendID(s), args...)
}

func (g *StandardLogger) Configure(debug bool) {
	if debug {
		logging.SetLevel(logging.DEBUG, g.module)
	} else {
		logging.SetLevel(logging.INFO, g.module)
	}
}

var formatMetaData = `%{time:15:04:05.000} ▶ %{level:.4s} %{program} %{shortfile} %{id:03x}`
var noColorFormat = formatMetaData + " %{message}"
var colorFormat = "%{color}" + formatMetaData + "%{color:reset} %{message}"

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
	return &StandardLogger{inner: logger, module: module}
}
