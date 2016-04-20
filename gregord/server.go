package main

import (
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

func (m *mainServer) listenAndServe() error {
	logger.Debug("listening at", m.opts.BindAddress)
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
