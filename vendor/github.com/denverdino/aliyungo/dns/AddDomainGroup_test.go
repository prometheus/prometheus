package dns

import (
	"testing"
)

func TestAddDomainGroup(t *testing.T) {
	client := NewTestClientNew()
	args := AddDomainGroupArgs{
		GroupName: TestDomainGroupName,
	}

	response, err := client.AddDomainGroup(&args)
	if err == nil {
		t.Logf("AddDomainGroup %s success, %v", TestDomainGroupName, response)

		deleteDomainGroupArgs := DeleteDomainGroupArgs{
			GroupId: response.GroupId,
		}
		client.DeleteDomainGroup(&deleteDomainGroupArgs)

	} else {
		t.Errorf("Failed to AddDomainGroup, %v", err)
	}
}
