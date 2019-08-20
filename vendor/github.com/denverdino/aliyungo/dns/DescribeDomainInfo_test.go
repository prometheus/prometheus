package dns

import "testing"

func TestDescribeDomainInfo(t *testing.T) {
	client := NewTestClientNew()
	describeArgs := DescribeDomainInfoArgs{
		DomainName: TestDomainName,
	}

	_, err := client.DescribeDomainInfo(&describeArgs)
	if err == nil {
		t.Logf("DescribeDomainInfo success")
	} else {
		t.Errorf("Failed to DescribeDomainInfo: %v", err)
	}
}
