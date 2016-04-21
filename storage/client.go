package storage

import (
	"bytes"
	"errors"
	log "github.com/Sirupsen/logrus"
	"time"

	"github.com/keybase/gregor"
	"github.com/keybase/gregor/protocol/gregor1"
	"golang.org/x/net/context"
)

type LocalStorageEngine interface {
	Store(gregor.UID, []byte) error
	Load(gregor.UID) ([]byte, error)
}

type Client struct {
	user     gregor.UID
	device   gregor.DeviceID
	sm       gregor.StateMachine
	storage  LocalStorageEngine
	incoming gregor1.IncomingInterface
}

func NewClient(user gregor.UID, device gregor.DeviceID, sm gregor.StateMachine, storage LocalStorageEngine, incoming gregor1.IncomingInterface) *Client {
	return &Client{
		user:     user,
		device:   device,
		sm:       sm,
		storage:  storage,
		incoming: incoming,
	}
}

func (c *Client) Save() error {
	if !c.sm.IsEphemeral() {
		return errors.New("state machine is non-ephemeral")
	}

	state, err := c.sm.State(c.user, c.device, nil)
	if err != nil {
		return err
	}

	b, err := state.Marshal()
	if err != nil {
		return err
	}

	return c.storage.Store(c.user, b)
}

func (c *Client) Restore() error {
	if !c.sm.IsEphemeral() {
		return errors.New("state machine is non-ephemeral")
	}

	value, err := c.storage.Load(c.user)
	if err != nil {
		return err
	}

	state, err := c.sm.ObjFactory().UnmarshalState(value)
	if err != nil {
		return err
	}

	return c.sm.InitState(state)
}

type errHashMismatch struct{}

func (e errHashMismatch) Error() string {
	return "local state hash != server state hash"
}

func (c *Client) syncFromTime(t *time.Time) error {
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	arg := gregor1.SyncArg{
		Uid:      gregor1.UID(c.user.Bytes()),
		Deviceid: gregor1.DeviceID(c.device.Bytes()),
	}
	if t != nil {
		arg.Ctime = gregor1.ToTime(*t)
	}
	res, err := c.incoming.Sync(ctx, arg)
	if err != nil {
		return err
	}

	for _, ibm := range res.Msgs {
		c.sm.ConsumeMessage(gregor1.Message{Ibm_: &ibm})
	}

	state, err := c.sm.State(c.user, c.device, nil)
	if err != nil {
		return err
	}

	hash, err := state.Hash()
	if err != nil {
		return err
	}

	if !bytes.Equal(res.Hash, hash) {
		return errHashMismatch{}
	}

	return nil
}
func (c *Client) Sync() error {
	if err := c.syncFromTime(c.sm.LatestCTime(c.user, c.device)); err != nil {
		if _, ok := err.(errHashMismatch); ok {
			log.Printf("Sync failure: %v\nReseting StateMachine and retrying", err)
			c.sm.Clear()
			err = c.syncFromTime(nil)
		}
		return err
	}
	return nil
}
