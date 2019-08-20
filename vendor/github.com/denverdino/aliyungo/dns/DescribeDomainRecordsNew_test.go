package dns

import (
	"testing"
)

func TestDescribeDomainRecordsNew(t *testing.T) {
	//prepare
	client := NewTestClientNew()
	describeArgs := DescribeDomainRecordsNewArgs{
		DomainName: TestDomainName,
	}
	describeArgs.PageSize = 100

	describeResponse, err := client.DescribeDomainRecordsNew(&describeArgs)
	if err == nil {
		t.Logf("DescribeDomainRecords success: TotalCount:%d ", describeResponse.TotalCount)
	} else {
		t.Errorf("Failed to DescribeDomainRecords: %s", describeArgs.DomainName)
	}
}
