package dns

import (
	"testing"
)

func TestGetMainDomainName(t *testing.T) {
	//prepare
	client := NewTestClient()
	args := GetMainDomainNameArgs{
		InputString: "go." + TestDomainName,
	}

	resp, err := client.GetMainDomainName(&args)
	if err == nil {
		t.Logf("GetMainDomainName success: %v ", resp)
	} else {
		t.Errorf("Failed to GetMainDomainName: %s", args.InputString)
	}
}
