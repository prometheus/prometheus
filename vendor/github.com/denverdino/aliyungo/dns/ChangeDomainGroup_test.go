package dns

import "testing"

func TestChangeDomainGroup(t *testing.T) {
	client := NewTestClientNew()

	// create origin group
	addGroupArgs := AddDomainGroupArgs{
		GroupName: TestDomainGroupName,
	}
	addGroupRes, _ := client.AddDomainGroup(&addGroupArgs)

	// add domain to origin group
	addDomainArgs := AddDomainArgs{
		DomainName: TestDomainName,
		GroupId:    addGroupRes.GroupId,
	}
	addDomainRes, _ := client.AddDomain(&addDomainArgs)

	// create new group
	addGroupArgs.GroupName = TestChanegGroupName
	addGroupRes, _ = client.AddDomainGroup(&addGroupArgs)

	// change to new group
	changeArgs := ChangeDomainGroupArgs{
		DomainName: addDomainRes.DomainName,
		GroupId:    addGroupRes.GroupId,
	}
	_, err := client.ChangeDomainGroup(&changeArgs)
	if err == nil {
		t.Logf("ChangeDomainGroup success")
	} else {
		t.Errorf("Failed to ChangeDomainGroup, %v", err)
	}
}
