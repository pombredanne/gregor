package rpc

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"golang.org/x/net/context"

	"github.com/keybase/gregor/srvup"
)

func TestPublish(t *testing.T) {
	c := clockwork.NewFakeClock()
	m := srvup.NewStorageMem(c)

	incoming1 := newStorageStateMachine()
	s1, l1, e1 := startTestServer(incoming1)
	defer s1.Shutdown()
	s1.authToken = superToken
	sg1 := srvup.New("gregord", 1*time.Second, 2*time.Second, m)
	defer sg1.Shutdown()
	sg1.HeartbeatLoop(l1.Addr().String())
	s1.SetStatusGroup(sg1)

	incoming2 := newStorageStateMachine()
	s2, l2, e2 := startTestServer(incoming2)
	defer s2.Shutdown()
	s2.authToken = superToken
	sg2 := srvup.New("gregord", 1*time.Second, 2*time.Second, m)
	defer sg2.Shutdown()
	sg2.HeartbeatLoop(l2.Addr().String())
	s2.SetStatusGroup(sg2)

	// connect a client to s2
	cli := newClient(l2.Addr())
	defer cli.Shutdown()
	if _, err := cli.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}
	<-e2.ConnCreated

	// s1 consumes a message
	m1 := newOOBMessage(goodUID, "sys", nil)
	if err := s1.consume(context.TODO(), m1); err != nil {
		t.Fatal(err)
	}

	// check that s1 sends a broadcast
	select {
	case <-e1.BcastSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for BcastSent on s1")
	}

	// check that s1 publishes a message
	select {
	case <-e1.PubSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for PubSent on s1")
	}

	// check that s2 sends a broadcast
	select {
	case <-e2.BcastSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for BcastSent on s2")
	}

	// check that the client connected to s2 received a broadcast
	if len(cli.broadcasts) != 1 {
		t.Errorf("client broadcasts received: %d, expected 1", len(cli.broadcasts))
	}
}
