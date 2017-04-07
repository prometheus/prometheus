package testutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/consul/consul/structs"
	"github.com/pkg/errors"
)

type testFn func() (bool, error)

const (
	baseWait = 1 * time.Millisecond
	maxWait  = 100 * time.Millisecond
)

func WaitForResult(try testFn) error {
	var err error
	wait := baseWait
	for retries := 100; retries > 0; retries-- {
		var success bool
		success, err = try()
		if success {
			time.Sleep(25 * time.Millisecond)
			return nil
		}

		time.Sleep(wait)
		wait *= 2
		if wait > maxWait {
			wait = maxWait
		}
	}
	if err != nil {
		return errors.Wrap(err, "timed out with error")
	} else {
		return fmt.Errorf("timed out")
	}
}

type rpcFn func(string, interface{}, interface{}) error

func WaitForLeader(t *testing.T, rpc rpcFn, dc string) structs.IndexedNodes {
	var out structs.IndexedNodes
	if err := WaitForResult(func() (bool, error) {
		// Ensure we have a leader and a node registration.
		args := &structs.DCSpecificRequest{
			Datacenter: dc,
		}
		if err := rpc("Catalog.ListNodes", args, &out); err != nil {
			return false, fmt.Errorf("Catalog.ListNodes failed: %v", err)
		}
		if !out.QueryMeta.KnownLeader {
			return false, fmt.Errorf("No leader")
		}
		if out.Index == 0 {
			return false, fmt.Errorf("Consul index is 0")
		}
		return true, nil
	}); err != nil {
		t.Fatalf("failed to find leader: %v", err)
	}
	return out
}
