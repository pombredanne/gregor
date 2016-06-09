package rpc

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	keybase1 "github.com/keybase/client/go/protocol"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/protocol/gregor1"
	"github.com/keybase/gregor/stats"
	"golang.org/x/net/context"
)

// ErrBadUID occurs when there is a bad UID on the auth channel.
var ErrBadUID = errors.New("bad UID on channel")

type authInfo struct {
	// Protects all variables below.
	authLock sync.RWMutex
	tok      gregor1.SessionToken
	res      gregor1.AuthResult
	authTime time.Time
}

func (i *authInfo) get() (
	tok gregor1.SessionToken, res gregor1.AuthResult, authTime time.Time) {
	i.authLock.RLock()
	defer i.authLock.RUnlock()
	return i.tok, i.res, i.authTime
}

func (i *authInfo) set(
	tok gregor1.SessionToken, res gregor1.AuthResult, authTime time.Time) {
	i.authLock.Lock()
	defer i.authLock.Unlock()
	i.tok = tok
	i.res = res
	i.authTime = authTime
}

func (i *authInfo) clear(sid gregor1.SessionID) bool {
	i.authLock.Lock()
	defer i.authLock.Unlock()
	if i.res.Sid != sid {
		return false
	}
	i.tok = ""
	i.res = gregor1.AuthResult{}
	i.authTime = time.Time{}
	return true
}

type connection struct {
	c      net.Conn
	xprt   rpc.Transporter
	parent *Server

	authCh   chan error
	authInfo authInfo

	server *rpc.Server

	// Suitable for receiving externally after
	// startAuthentication() finishes successfully.
	serverDoneCh <-chan struct{}

	log   rpc.LogOutput
	stats stats.Registry
}

// instrumentedConnection times all of the RPC calls served for stats purposes
type instrumentedConnection struct {
	conn *connection
}

func newConnection(c net.Conn, parent *Server) (*connection, error) {
	xprt := rpc.NewTransport(c, rpc.NewSimpleLogFactory(parent.log, nil), keybase1.WrapError)

	conn := &connection{
		c:      c,
		xprt:   xprt,
		parent: parent,
		authCh: make(chan error, 1),
		log:    parent.log,
		stats:  parent.stats.SetPrefix("connection"),
	}

	if err := conn.startRPCServer(); err != nil {
		return nil, err
	}

	return conn, nil
}

var superUID = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00}

func SuperUID() []byte { return superUID }

func matchUIDorSuperuser(uid, expected []byte) bool {
	return bytes.Equal(uid, superUID) || bytes.Equal(uid, expected)
}

func (c *connection) checkUIDAuth(ctx context.Context, uid gregor1.UID) error {
	tok, res, _ := c.authInfo.get()
	if _, err := c.AuthenticateSessionToken(ctx, tok); err != nil {
		return err
	}

	if !matchUIDorSuperuser(res.Uid, uid.Bytes()) {
		return AuthError(fmt.Sprintf("mismatched UIDs: %v != %v", uid, res.Uid))
	}
	return nil
}

func (c *connection) checkMessageAuth(ctx context.Context, m gregor1.Message) error {
	if ibm := m.ToInBandMessage(); ibm != nil {
		if ibm.Metadata() == nil || ibm.Metadata().UID() == nil {
			return errors.New("no valid UID in message")
		}
		if err := c.checkUIDAuth(ctx, ibm.Metadata().UID().Bytes()); err != nil {
			return err
		}
	}
	if oobm := m.ToOutOfBandMessage(); oobm != nil {
		if oobm.UID() == nil {
			return errors.New("no valid UID in message")
		}
		if err := c.checkUIDAuth(ctx, oobm.UID().Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (c *connection) AuthenticateSessionToken(ctx context.Context, tok gregor1.SessionToken) (gregor1.AuthResult, error) {

	c.stats.Count("AuthenticateSessionToken")
	c.log.Debug("Authenticate: %+v", tok)
	if tok == "" {
		var UnauthenticatedSessionError = keybase1.Status{
			Name: "BAD_SESSION",
			Code: int(keybase1.StatusCode_SCBadSession),
			Desc: "unauthed session"}
		c.log.Error("Authenticate: blank session token for connection!")
		c.stats.Count("AuthenticateSessionToken - blank")
		return gregor1.AuthResult{}, UnauthenticatedSessionError
	}

	res, err := c.parent.auth.AuthenticateSessionToken(ctx, tok)
	if err == nil {
		c.authInfo.set(tok, res, c.parent.clock.Now())
	}
	select {
	case c.authCh <- err:
		// First auth call, or first auth call after authCh is
		// read from.
	default:
		// Subsequent auth calls -- just drop.
	}
	return res, err
}

func (c *connection) Sync(ctx context.Context, arg gregor1.SyncArg) (gregor1.SyncResult, error) {
	c.stats.Count("Sync")
	if err := c.checkUIDAuth(ctx, arg.Uid); err != nil {
		return gregor1.SyncResult{}, err
	}

	return c.parent.startSync(ctx, arg)
}

func validateConsumeMessage(m gregor1.Message) error {
	errfunc := func(msg string) error {
		s := fmt.Sprintf("invalid consume call: %s", msg)
		return errors.New(s)
	}
	ibm := m.ToInBandMessage()
	obm := m.ToOutOfBandMessage()
	if ibm == nil && obm == nil {
		return errfunc("unknown message type")
	}
	if ibm != nil {
		upd := ibm.ToStateUpdateMessage()
		if upd != nil {
			if upd.Metadata() == nil {
				return errfunc("missing metadata fields")
			} else {
				if upd.Metadata().MsgID() == nil {
					return errfunc("missing msg ID")
				}
				if upd.Metadata().UID() == nil {
					return errfunc("missing UID")
				}
			}

			if upd.Creation() == nil && upd.Dismissal() == nil {
				return errfunc("unknown state update message type")
			} else {
				if upd.Creation() != nil {
					if upd.Creation().Category() == nil {
						return errfunc("missing category")
					}
					if upd.Creation().Body() == nil {
						return errfunc("missing body")
					}
				}
				if upd.Dismissal() != nil {
					if upd.Dismissal().MsgIDsToDismiss() == nil {
						return errfunc("missing msg IDs to dismiss")
					}
					if upd.Dismissal().RangesToDismiss() == nil {
						return errfunc("missing ranges to dismiss")
					}
				}
			}
		}
	}

	if obm != nil {
		if obm.UID() == nil {
			return errfunc("missing UID")
		}
		if obm.System() == nil {
			return errfunc("missing system")
		}
		if obm.Body() == nil {
			return errfunc("missing body")
		}
	}

	return nil
}

func (c *connection) ConsumeMessage(ctx context.Context, m gregor1.Message) error {

	// Check the validity of the message first
	if err := validateConsumeMessage(m); err != nil {
		c.stats.Count("validateConsumeMessage failed")
		return err
	}

	c.stats.Count("ConsumeMessage")
	// Debugging
	ibm := m.ToInBandMessage()
	if ibm != nil {
		c.stats.Count("ConsumeMessage - ibm")
		c.log.Debug("ConsumeMessage: in-band message: msgID: %s Ctime: %s",
			m.ToInBandMessage().Metadata().MsgID(), m.ToInBandMessage().Metadata().CTime())
	} else {
		c.stats.Count("ConsumeMessage - oobm")
		c.log.Debug("ConsumeMessage: out-of-band message: uid: %s", m.ToOutOfBandMessage().UID())
	}

	// Check authorization
	if err := c.checkMessageAuth(ctx, m); err != nil {
		return err
	}

	// Start up the main processing procedure
	return c.parent.runConsumeMessageMainSequence(ctx, m)
}

func (c *connection) ConsumePublishMessage(ctx context.Context, m gregor1.Message) error {

	// Check the validity of the message first
	if err := validateConsumeMessage(m); err != nil {
		return err
	}

	// Debugging
	ibm := m.ToInBandMessage()
	if ibm != nil {
		c.stats.Count("ConsumePublishMessage - ibm")
		c.log.Debug("ConsumeMessage: in-band message: msgID: %s Ctime: %s",
			m.ToInBandMessage().Metadata().MsgID(), m.ToInBandMessage().Metadata().CTime())
	} else {
		c.stats.Count("ConsumePublishMessage - oobm")
		c.log.Debug("ConsumeMessage: out-of-band message: uid: %s", m.ToOutOfBandMessage().UID())
	}

	// Check authorization
	if err := c.checkMessageAuth(ctx, m); err != nil {
		c.close()
		return err
	}

	return c.parent.consumePublish(ctx, m)
}

func (c *connection) StateByCategoryPrefix(ctx context.Context, arg gregor1.StateByCategoryPrefixArg) (gregor1.State, error) {
	c.stats.Count("StateByCategoryPrefix")
	if err := c.checkUIDAuth(ctx, arg.Uid); err != nil {
		return gregor1.State{}, err
	}
	return c.parent.stateByCategoryPrefix(ctx, arg)
}

func (c *connection) Ping(ctx context.Context) (string, error) {
	c.stats.Count("Ping")
	return "pong", nil
}

func (c *connection) GetReminders(ctx context.Context, maxReminders int) (ret gregor1.ReminderSet, err error) {
	c.stats.Count("GetReminders")
	// Need to have super user for GetReminders
	if err = c.checkUIDAuth(ctx, superUID); err != nil {
		return ret, err
	}

	return c.parent.getReminders(ctx, maxReminders)
}

func (c *connection) DeleteReminders(ctx context.Context, rids []gregor1.ReminderID) error {

	c.stats.Count("DeleteReminders")

	// Need to have super user for DeleteReminders
	if err := c.checkUIDAuth(ctx, superUID); err != nil {
		return err
	}
	return c.parent.deleteReminders(ctx, rids)
}

func (c *connection) startRPCServer() error {
	c.server = rpc.NewServer(c.xprt, keybase1.WrapError)

	iconn := instrumentedConnection{conn: c}
	prots := []rpc.Protocol{
		gregor1.AuthProtocol(iconn),
		gregor1.IncomingProtocol(iconn),
		gregor1.RemindProtocol(iconn),
	}
	for _, prot := range prots {
		c.log.Info("registering protocol %s", prot.Name)
		if err := c.server.Register(prot); err != nil {
			return err
		}
	}

	c.serverDoneCh = c.server.Run()
	return nil
}

func (c *connection) startAuthentication() error {
	select {
	case <-c.serverDoneCh:
		return c.server.Err()

	case err := <-c.authCh:
		return err
	}
}

func (c *connection) serverDoneChan() <-chan struct{} {
	return c.serverDoneCh
}

// serverDoneErr returns a non-nil error only after serverDoneChan() is
// closed.
func (c *connection) serverDoneErr() error {
	return c.server.Err()
}

func (c *connection) close() {
	// Should trigger the c.serverDoneCh case in
	// startAuthentication.
	c.c.Close()
}

func (c instrumentedConnection) instrument(name string) func() {
	now := time.Now()
	return func() {
		dur := time.Since(now)
		durms := int(dur.Nanoseconds() / 1000000)
		if durms > 10000 {
			c.conn.stats.Count("slow response - " + name)
			c.conn.log.Error("slow response time: rpc: %s time: %dms", name, durms)
		}
		c.conn.stats.ValueInt("speed - "+name, durms)
	}
}

func (c instrumentedConnection) AuthenticateSessionToken(ctx context.Context, arg gregor1.SessionToken) (gregor1.AuthResult, error) {
	f := c.instrument("AuthenticateSessionToken")
	defer f()
	return c.conn.AuthenticateSessionToken(ctx, arg)
}

func (c instrumentedConnection) Sync(ctx context.Context, arg gregor1.SyncArg) (res gregor1.SyncResult, err error) {
	f := c.instrument("Sync")
	defer f()
	return c.conn.Sync(ctx, arg)
}

func (c instrumentedConnection) ConsumeMessage(ctx context.Context, arg gregor1.Message) error {
	f := c.instrument("ConsumeMessage")
	defer f()
	return c.conn.ConsumeMessage(ctx, arg)
}

func (c instrumentedConnection) ConsumePublishMessage(ctx context.Context, arg gregor1.Message) error {
	f := c.instrument("ConsumePublishMessage")
	defer f()
	return c.conn.ConsumePublishMessage(ctx, arg)
}

func (c instrumentedConnection) Ping(ctx context.Context) (string, error) {
	f := c.instrument("Ping")
	defer f()
	return c.conn.Ping(ctx)
}

func (c instrumentedConnection) StateByCategoryPrefix(ctx context.Context,
	arg gregor1.StateByCategoryPrefixArg) (gregor1.State, error) {
	f := c.instrument("StateByCategoryPrefix")
	defer f()
	return c.conn.StateByCategoryPrefix(ctx, arg)
}

func (c instrumentedConnection) GetReminders(ctx context.Context, arg int) (gregor1.ReminderSet, error) {
	f := c.instrument("GetReminders")
	defer f()
	return c.conn.GetReminders(ctx, arg)
}

func (c instrumentedConnection) DeleteReminders(ctx context.Context, arg []gregor1.ReminderID) error {
	f := c.instrument("DeleteReminders")
	defer f()
	return c.conn.DeleteReminders(ctx, arg)
}

var _ gregor1.AuthInterface = (*connection)(nil)
var _ gregor1.IncomingInterface = (*connection)(nil)
var _ gregor1.AuthInterface = instrumentedConnection{}
var _ gregor1.IncomingInterface = instrumentedConnection{}
var _ gregor1.RemindInterface = instrumentedConnection{}
