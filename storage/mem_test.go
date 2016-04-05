package storage

import (
	"testing"

	"github.com/jonboulle/clockwork"
	protocol "github.com/keybase/gregor/protocol/go"
	test "github.com/keybase/gregor/test"
)

func TestMemEngine(t *testing.T) {
	cl := clockwork.NewFakeClock()
	of := protocol.ObjFactory{}
	eng := NewMemEngine(of, cl)
	test.TestStateMachineAllDevices(t, eng)
	test.TestStateMachinePerDevice(t, eng)
}
