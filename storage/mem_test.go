package storage

import (
	"testing"

	"github.com/jonboulle/clockwork"
	protocol "github.com/keybase/gregor/protocol/go"
	test "github.com/keybase/gregor/test"
)

func TestMemEngine(t *testing.T) {
	cl := clockwork.NewFakeClock()
	eng := NewMemEngine(protocol.ObjFactory{}, cl)
	test.TestStateMachineAllDevices(t, eng, cl)
	test.TestStateMachinePerDevice(t, eng, cl)
}
