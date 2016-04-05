package storage

import (
	"errors"

	"github.com/keybase/gregor"
)

type LocalStorageEngine interface {
	Store(gregor.UID, []byte) error
	Load(gregor.UID) ([]byte, error)
}

type Client struct {
	user       gregor.UID
	device     gregor.DeviceID
	objFactory gregor.ObjFactory
	sm         gregor.StateMachine
	storage    LocalStorageEngine
}

func NewClient(user gregor.UID, device gregor.DeviceID, objFactory gregor.ObjFactory, sm gregor.StateMachine, storage LocalStorageEngine) *Client {
	return &Client{
		user:       user,
		device:     device,
		objFactory: objFactory,
		sm:         sm,
		storage:    storage,
	}
}

func (c *Client) Save() error {
	if !c.sm.IsEphemeral() {
		return errors.New("state machine is non-ephemeral")
	}

	c.sm.RemoveDismissed(nil)
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

	state, err := c.objFactory.UnmarshalState(value)
	if err != nil {
		return err
	}

	return c.sm.InitState(state)
}
