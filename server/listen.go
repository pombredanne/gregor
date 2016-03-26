package main

import (
	"crypto/tls"
	"net"
)

func (m *mainServer) listenTLS() (net.Listener, error) {
	cert, err := tls.X509KeyPair([]byte(m.opts.TLSOptions.Cert), []byte(m.opts.TLSOptions.Key))
	if err != nil {
		return nil, err
	}
	conf := tls.Config{Certificates: []tls.Certificate{cert}}
	return tls.Listen("tcp", m.opts.BindAddress, &conf)
}

func (m *mainServer) listen() (net.Listener, error) {
	if m.opts.TLSOptions != nil {
		return m.listenTLS()
	}
	return net.Listen("tcp", m.opts.BindAddress)
}
