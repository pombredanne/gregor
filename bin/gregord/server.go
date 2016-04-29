package main

import (
	"crypto/tls"
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
	return &mainServer{opts: o, mls: m, addr: make(chan net.Addr, 1)}
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
