package main

import (
	gregor "github.com/keybase/gregor"
	"os"
	"os/signal"
	"syscall"
)

type mainServer struct {
	opts   *Options
	mls    gregor.MainLoopServer
	stopCh chan struct{}
}

func newMainServer(o *Options, m gregor.MainLoopServer) *mainServer {
	return &mainServer{opts: o, mls: m}
}

func (m *mainServer) listenAndServe() error {
	l, err := m.listen()
	if err != nil {
		return err
	}
	signalCh := make(chan os.Signal, 1)
	go signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, os.Kill)
	go m.mls.Serve(l)
	select {
	case <-signalCh:
	case <-m.stopCh:
	}
	return l.Close()
}
