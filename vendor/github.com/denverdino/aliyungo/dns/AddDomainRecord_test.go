package dns

import (
	"testing"
)

func TestAddDomainRecord(t *testing.T) {
	client := NewTestClient()
	addDomainRecordArgs := AddDomainRecordArgs{
		DomainName: TestDomainName,
		RR:         "testaddrecord",
		Type:       ARecord,
		Value:      "8.8.8.8",
	}
	response, err := client.AddDomainRecord(&addDomainRecordArgs)
	if err == nil {
		t.Logf("AddDomainRecord: testaddr for domain: %s Success, %v",
			TestDomainName, response)

		deleteDomainRecordArgs := DeleteDomainRecordArgs{
			RecordId: response.RecordId,
		}
		client.DeleteDomainRecord(&deleteDomainRecordArgs)
	} else {
		t.Errorf("Failed to AddDomainRecord: testaddr for domain: %s", TestDomainName)
	}
}
