package api

import (
	"testing"

	"github.com/hashicorp/consul/sdk/testutil"
)

func TestAPI_OperatorKeyringInstallListPutRemove(t *testing.T) {
	t.Parallel()
	oldKey := "d8wu8CSUrqgtjVsvcBPmhQ=="
	newKey := "qxycTi/SsePj/TZzCBmNXw=="
	c, s := makeClientWithConfig(t, nil, func(c *testutil.TestServerConfig) {
		c.Encrypt = oldKey
	})
	defer s.Stop()

	operator := c.Operator()
	if err := operator.KeyringInstall(newKey, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	listResponses, err := operator.KeyringList(nil)
	if err != nil {
		t.Fatalf("err %v", err)
	}

	// Make sure the new key is installed
	if len(listResponses) != 2 {
		t.Fatalf("bad: %v", len(listResponses))
	}
	for _, response := range listResponses {
		if len(response.Keys) != 2 {
			t.Fatalf("bad: %v", len(response.Keys))
		}
		if _, ok := response.Keys[oldKey]; !ok {
			t.Fatalf("bad: %v", ok)
		}
		if _, ok := response.Keys[newKey]; !ok {
			t.Fatalf("bad: %v", ok)
		}
	}

	// Switch the primary to the new key
	if err := operator.KeyringUse(newKey, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	if err := operator.KeyringRemove(oldKey, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	listResponses, err = operator.KeyringList(nil)
	if err != nil {
		t.Fatalf("err %v", err)
	}

	// Make sure the old key is removed
	if len(listResponses) != 2 {
		t.Fatalf("bad: %v", len(listResponses))
	}
	for _, response := range listResponses {
		if len(response.Keys) != 1 {
			t.Fatalf("bad: %v", len(response.Keys))
		}
		if _, ok := response.Keys[oldKey]; ok {
			t.Fatalf("bad: %v", ok)
		}
		if _, ok := response.Keys[newKey]; !ok {
			t.Fatalf("bad: %v", ok)
		}
	}
}
