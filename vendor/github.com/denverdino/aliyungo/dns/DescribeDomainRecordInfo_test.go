package dns

import (
	"testing"
)

func TestDescribeDomainRecordInfo(t *testing.T) {
	//prepare
	client := NewTestClient()
	describeArgs := DescribeDomainRecordsArgs{
		DomainName: TestDomainName,
	}
	describeArgs.PageSize = 100

	describeResponse, err := client.DescribeDomainRecords(&describeArgs)
	if err == nil {
		record := describeResponse.DomainRecords.Record[0]
		arg := DescribeDomainRecordInfoArgs{
			RecordId: record.RecordId,
		}
		response, err := client.DescribeDomainRecordInfo(&arg)
		if err == nil {
			t.Logf("DescribeDomainRecordInfo success: %v", response)
		} else {
			t.Errorf("Failed to DescribeDomainRecordInfo: %s", describeArgs.DomainName)
		}
	} else {
		t.Errorf("Failed to DescribeDomainRecords: %s", describeArgs.DomainName)
	}
}
