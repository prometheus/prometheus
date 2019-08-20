package dns

import (
	"testing"
)

func TestDescribeDomainRecordInfoNew(t *testing.T) {
	//prepare
	client := NewTestClientNew()
	describeArgs := DescribeDomainRecordsNewArgs{
		DomainName: TestDomainName,
	}
	describeArgs.PageSize = 100

	describeResponse, err := client.DescribeDomainRecordsNew(&describeArgs)
	if err == nil {
		record := describeResponse.DomainRecords.Record[0]
		arg := DescribeDomainRecordInfoNewArgs{
			RecordId: record.RecordId,
		}
		response, err := client.DescribeDomainRecordInfoNew(&arg)
		if err == nil {
			t.Logf("DescribeDomainRecordInfo success: %v", response)
		} else {
			t.Errorf("Failed to DescribeDomainRecordInfo: %s", describeArgs.DomainName)
		}
	} else {
		t.Errorf("Failed to DescribeDomainRecords: %s", describeArgs.DomainName)
	}
}
