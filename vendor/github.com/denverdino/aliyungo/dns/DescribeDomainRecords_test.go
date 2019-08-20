package dns

import (
	"testing"
)

func TestDescribeDomainRecords(t *testing.T) {
	//prepare
	client := NewTestClient()
	describeArgs := DescribeDomainRecordsArgs{
		DomainName: TestDomainName,
	}
	describeArgs.PageSize = 100

	describeResponse, err := client.DescribeDomainRecords(&describeArgs)
	if err == nil {
		t.Logf("DescribeDomainRecords success: TotalCount:%d ", describeResponse.TotalCount)
	} else {
		t.Errorf("Failed to DescribeDomainRecords: %s", describeArgs.DomainName)
	}
}
