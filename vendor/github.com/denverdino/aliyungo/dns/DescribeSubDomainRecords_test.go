package dns

import (
	"testing"
)

func TestDescribeSubDomainRecords(t *testing.T) {
	//prepare
	client := NewTestClient()
	describeArgs := DescribeSubDomainRecordsArgs{
		SubDomain: "go." + TestDomainName,
	}

	describeResponse, err := client.DescribeSubDomainRecords(&describeArgs)
	if err == nil {
		t.Logf("DescribeSubDomainRecords success: %v ", describeResponse)
	} else {
		t.Errorf("Failed to DescribeSubDomainRecords: %s", describeArgs.SubDomain)
	}
}
