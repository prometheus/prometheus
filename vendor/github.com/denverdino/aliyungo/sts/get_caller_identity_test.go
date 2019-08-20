package sts

import "testing"

func TestGetCallerIdentity(t *testing.T) {

	stsClient := NewTestClient()
	resp, err := stsClient.GetCallerIdentity()
	if err != nil {
		t.Errorf("Failed to GetCallerIdentity %v", err)
		return
	}

	t.Logf("pass GetCallerIdentity %v", resp)
}
