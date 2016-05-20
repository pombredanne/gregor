package main

import (
	"crypto/tls"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/keybase/gregor"
)

type mainServer struct {
	opts   *Options
	mls    gregor.MainLoopServer
	stopCh chan struct{}
	addr   chan net.Addr
}

func newMainServer(o *Options, m gregor.MainLoopServer) *mainServer {
	ret := &mainServer{opts: o, mls: m, addr: make(chan net.Addr, 1)}
	if o.ChildMode {
		ret.stopOnClosedStdin()
	}
	return ret
}

// waitForEOFThenClose waits for eof on the given reader, and then closes
// the given channel
func waitForEOFThenClose(r io.Reader, ch chan struct{}) {
	var buf [1024]byte

	for {
		n, err := r.Read(buf[:])
		if n == 0 || err != nil {
			close(ch)
			break
		}
	}
}

func (m *mainServer) stopOnClosedStdin() {
	m.stopCh = make(chan struct{})
	go waitForEOFThenClose(os.Stdin, m.stopCh)
}

func (m *mainServer) listen() (net.Listener, error) {
	if m.opts.TLSConfig != nil {
		return tls.Listen("tcp", m.opts.BindAddress, m.opts.TLSConfig)
	}
	return net.Listen("tcp", m.opts.BindAddress)
}

func (m *mainServer) listenAndServe() error {
	l, err := m.listen()
	if err != nil {
		return err
	}
	m.addr <- l.Addr()
	signalCh := make(chan os.Signal, 1)
	go signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, os.Kill)
	go m.mls.ListenLoop(l)
	select {
	case <-signalCh:
	case <-m.stopCh:
	}
	return l.Close()
}
