package rpc

import (
	"bytes"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"golang.org/x/net/context"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/srvup"
)

func TestPublish(t *testing.T) {
	c := clockwork.NewFakeClock()
	m := srvup.NewStorageMem(c)
	log := rpc.SimpleLogOutput{}

	incoming1 := newStorageStateMachine()
	s1, l1, e1, _ := startTestServer(incoming1)
	defer s1.Shutdown()
	sg1 := srvup.New("gregord", 1*time.Second, 2*time.Second, m, log)
	defer sg1.Shutdown()
	sg1.HeartbeatLoop("fmprpc://" + l1.Addr().String())
	s1.SetStatusGroup(sg1)

	incoming2 := newStorageStateMachine()
	s2, l2, e2, _ := startTestServer(incoming2)
	defer s2.Shutdown()
	sg2 := srvup.New("gregord", 1*time.Second, 2*time.Second, m, log)
	defer sg2.Shutdown()
	sg2.HeartbeatLoop("fmprpc://" + l2.Addr().String())
	s2.SetStatusGroup(sg2)

	// connect a client to s2
	cli := newClient(l2.Addr())
	defer cli.Shutdown()
	if _, err := cli.AuthClient().AuthenticateSessionToken(context.TODO(), goodToken); err != nil {
		t.Fatal(err)
	}
	<-e2.ConnCreated

	// s1 consumes a message
	m1 := newOOBMessage(goodUID, "sys", []byte{})
	if err := s1.runConsumeMessageMainSequence(context.TODO(), m1); err != nil {
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
		t.Fatalf("client broadcasts received: %d, expected 1", len(cli.broadcasts))
	}

	// compare the message contents
	m2 := cli.broadcasts[0]
	if !bytes.Equal(m2.Oobm_.Uid_, m1.Oobm_.Uid_) {
		t.Errorf("m2 uid: %x, expected %x", m2.Oobm_.Uid_, m1.Oobm_.Uid_)
	}
	if m2.Oobm_.System_ != m1.Oobm_.System_ {
		t.Errorf("m2 system: %s, expected %s", m2.Oobm_.System_, m1.Oobm_.System_)
	}
	if !bytes.Equal(m2.Oobm_.Body_, m1.Oobm_.Body_) {
		t.Errorf("m2 body: %s, expected %s", m2.Oobm_.Body_, m2.Oobm_.Body_)
	}
}

func TestNodeIds(t *testing.T) {
	c := clockwork.NewFakeClock()
	m := srvup.NewStorageMem(c)
	log := rpc.SimpleLogOutput{}

	incoming1 := newStorageStateMachine()
	s1, l1, _, _ := startTestServer(incoming1)
	defer s1.Shutdown()
	sg1 := srvup.New("gregord", 1*time.Second, 2*time.Second, m, log)
	defer sg1.Shutdown()
	sg1.HeartbeatLoop("fmprpc://" + l1.Addr().String())
	s1.SetStatusGroup(sg1)

	incoming2 := newStorageStateMachine()
	s2, l2, _, _ := startTestServer(incoming2)
	defer s2.Shutdown()
	sg2 := srvup.New("gregord", 1*time.Second, 2*time.Second, m, log)
	defer sg2.Shutdown()
	sg2.HeartbeatLoop("fmprpc://" + l2.Addr().String())
	s2.SetStatusGroup(sg2)

	ag := newAliveGroup(sg1, s1.auth, sg1.MyID(), s1.publishTimeout, c, s1.shutdownCh,
		rpc.SimpleLogOutput{}, nil)
	if len(ag.group) != 1 {
		t.Fatalf("alive group is wrong size %d != 1", len(ag.group))
	}
}
