package dns

import (
	"testing"
)

func TestDeleteDomain(t *testing.T) {
	client := NewTestClientNew()
	args := AddDomainArgs{
		DomainName: TestDomainName,
	}
	client.AddDomain(&args)

	deleteDomainArgs := DeleteDomainArgs{
		DomainName: TestDomainName,
	}
	response, err := client.DeleteDomain(&deleteDomainArgs)
	if err == nil {
		t.Logf("DeleteDomain %s success, %v", TestDomainName, response)
	} else {
		t.Errorf("Failed to DeleteDomain, %v", err)
	}
}
