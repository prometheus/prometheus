package dns

import (
	"testing"
)

func TestDeleteDomainGroup(t *testing.T) {
	client := NewTestClientNew()
	args := AddDomainGroupArgs{
		GroupName: TestDomainGroupName,
	}
	res, _ := client.AddDomainGroup(&args)

	deleteDomainGroupArgs := DeleteDomainGroupArgs{
		GroupId: res.GroupId,
	}
	response, err := client.DeleteDomainGroup(&deleteDomainGroupArgs)
	if err == nil {
		t.Logf("DeleteDomainGroup %s success, %v", TestDomainGroupName, response)
	} else {
		t.Errorf("Failed to DeleteDomainGroup, %v", err)
	}
}
