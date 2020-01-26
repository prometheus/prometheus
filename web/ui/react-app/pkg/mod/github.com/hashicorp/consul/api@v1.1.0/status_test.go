package api

import (
	"testing"
)

func TestAPI_StatusLeader(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()
	s.WaitForSerfCheck(t)

	status := c.Status()

	leader, err := status.Leader()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if leader == "" {
		t.Fatalf("Expected leader")
	}
}

func TestAPI_StatusPeers(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()
	s.WaitForSerfCheck(t)

	status := c.Status()

	peers, err := status.Peers()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(peers) == 0 {
		t.Fatalf("Expected peers ")
	}
}
