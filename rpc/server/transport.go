package rpc

import (
	"net"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"golang.org/x/net/context"
)

// connTransport implements rpc.ConnectionTransport
type connTransport struct {
	log             rpc.LogOutput
	opts            rpc.LogOptions
	uri             *rpc.FMPURI
	conn            net.Conn
	transport       rpc.Transporter
	stagedTransport rpc.Transporter
}

var _ rpc.ConnectionTransport = (*connTransport)(nil)

func NewConnTransport(log rpc.LogOutput, opts rpc.LogOptions, uri *rpc.FMPURI) *connTransport {
	return &connTransport{
		log:  log,
		opts: opts,
		uri:  uri,
	}
}

func (t *connTransport) Dial(context.Context) (rpc.Transporter, error) {
	var err error
	t.conn, err = t.uri.Dial()
	if err != nil {
		return nil, err
	}
	t.stagedTransport = rpc.NewTransport(t.conn, rpc.NewSimpleLogFactory(t.log, t.opts), keybase1.WrapError)
	return t.stagedTransport, nil
}

func (t *connTransport) IsConnected() bool {
	return t.transport != nil && t.transport.IsConnected()
}

func (t *connTransport) Finalize() {
	t.transport = t.stagedTransport
	t.stagedTransport = nil
}

func (t *connTransport) Close() {
	t.conn.Close()
}
