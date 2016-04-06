package storage

import (
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/keybase/gregor/protocol/gregor1"
	test "github.com/keybase/gregor/test"
)

func TestMemEngine(t *testing.T) {
	cl := clockwork.NewFakeClock()
	of := gregor1.ObjFactory{}
	eng := NewMemEngine(of, cl)
	test.TestStateMachineAllDevices(t, eng)
	test.TestStateMachinePerDevice(t, eng)
}
