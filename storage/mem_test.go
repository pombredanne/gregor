package storage

import (
	"github.com/jonboulle/clockwork"
	test "github.com/keybase/gregor/test"
	"testing"
)

func TestMemEngine(t *testing.T) {
	cl := clockwork.NewFakeClock()
	eng := NewMemEngine(test.TestObjFactory{}, cl)
	test.TestStateMachineAllDevices(t, eng, cl)
	test.TestStateMachinePerDevice(t, eng, cl)
}
