package dns

import (
	"testing"
)

func TestUpdateDomainRecord(t *testing.T) {
	//prepare
	client := NewTestClient()
	addDomainRecordArgs := AddDomainRecordArgs{
		DomainName: TestDomainName,
		RR:         "testupdaterecordid",
		Value:      "8.8.8.8",
		Type:       ARecord,
	}

	addResponse, err := client.AddDomainRecord(&addDomainRecordArgs)

	// test update record
	updateArgs := UpdateDomainRecordArgs{
		RecordId: addResponse.RecordId,
		RR:       addDomainRecordArgs.RR,
		Value:    "4.4.4.4",
		Type:     ARecord,
	}

	_, err = client.UpdateDomainRecord(&updateArgs)
	if err == nil {
		t.Logf("UpdateDomainRecord success: RR:%s Value:%s", updateArgs.RR, updateArgs.Value)
	} else {
		t.Errorf("Failed to UpdateDomainRecord: %s", updateArgs.RecordId)
	}

	//clearup
	deleteDomainRecordArgs := DeleteDomainRecordArgs{
		RecordId: addResponse.RecordId,
	}
	_, err = client.DeleteDomainRecord(&deleteDomainRecordArgs)
	if err != nil {
		t.Errorf("Failed to DeleteDomainRecord: %s", deleteDomainRecordArgs.RecordId)
	}

}
