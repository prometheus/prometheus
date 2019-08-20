package dns

import "testing"

func TestUpdateDomainGroup(t *testing.T) {
	client := NewTestClientNew()

	addGroupArgs := AddDomainGroupArgs{
		GroupName: TestDomainGroupName,
	}
	addGroupRes, _ := client.AddDomainGroup(&addGroupArgs)

	updateArgs := UpdateDomainGroupArgs{
		GroupId:   addGroupRes.GroupId,
		GroupName: TestChanegGroupName,
	}
	_, err := client.UpdateDomainGroup(&updateArgs)
	if err == nil {
		t.Logf("UpdateDomainGroup success")
	} else {
		t.Errorf("Failed to UpdateDomainGroup, %v", err)
	}
}
