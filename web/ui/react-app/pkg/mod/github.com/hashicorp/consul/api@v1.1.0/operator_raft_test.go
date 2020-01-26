package api

import (
	"strings"
	"testing"
)

func TestAPI_OperatorRaftGetConfiguration(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	operator := c.Operator()
	out, err := operator.RaftGetConfiguration(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out.Servers) != 1 ||
		!out.Servers[0].Leader ||
		!out.Servers[0].Voter {
		t.Fatalf("bad: %v", out)
	}
}

func TestAPI_OperatorRaftRemovePeerByAddress(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	// If we get this error, it proves we sent the address all the way
	// through.
	operator := c.Operator()
	err := operator.RaftRemovePeerByAddress("nope", nil)
	if err == nil || !strings.Contains(err.Error(),
		"address \"nope\" was not found in the Raft configuration") {
		t.Fatalf("err: %v", err)
	}
}
