package main

import (
	"crypto/tls"
	"net"
)

func (m *mainServer) listen() (net.Listener, error) {
	if m.opts.TLSConfig != nil {
		return tls.Listen("tcp", m.opts.BindAddress, m.opts.TLSConfig)
	}
	return net.Listen("tcp", m.opts.BindAddress)
}
