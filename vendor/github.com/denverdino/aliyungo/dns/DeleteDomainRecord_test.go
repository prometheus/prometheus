package dns

import (
	"testing"
)

func TestDeleteDomainRecord(t *testing.T) {
	//prepare
	client := NewTestClient()
	addDomainRecordArgs := AddDomainRecordArgs{
		DomainName: TestDomainName,
		RR:         "testdeleterecordid",
		Type:       ARecord,
		Value:      "8.8.8.8",
	}

	addResponse, err := client.AddDomainRecord(&addDomainRecordArgs)

	//test delete record
	deleteDomainRecordArgs := DeleteDomainRecordArgs{
		RecordId: addResponse.RecordId,
	}
	deleteResponse, err := client.DeleteDomainRecord(&deleteDomainRecordArgs)
	if err == nil {
		t.Logf("DeleteDomainRecord: %v", deleteResponse)
	} else {
		t.Errorf("Failed to DeleteDomainRecord: %s", deleteDomainRecordArgs.RecordId)
	}
}
