package dns

import (
	"testing"
)

func TestDeleteSubDomainRecords(t *testing.T) {
	//prepare
	client := NewTestClient()
	addDomainRecordArgs := AddDomainRecordArgs{
		DomainName: TestDomainName,
		RR:         "testdeletesubdomainrecords",
		Type:       ARecord,
		Value:      "8.8.8.8",
	}

	client.AddDomainRecord(&addDomainRecordArgs)

	//test delete subdomain
	deleteDomainRecordArgs := DeleteSubDomainRecordsArgs{
		DomainName: TestDomainName,
		RR:         "testdeletesubdomainrecords",
	}
	deleteResponse, err := client.DeleteSubDomainRecords(&deleteDomainRecordArgs)
	if err == nil {
		t.Logf("DeleteDomainRecord: %v", deleteResponse)
	} else {
		t.Errorf("Failed to DeleteSubDomainRecords: %s", deleteDomainRecordArgs.RR)
	}
}
