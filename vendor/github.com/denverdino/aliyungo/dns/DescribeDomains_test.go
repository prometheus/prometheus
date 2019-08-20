package dns

import "testing"

func TestDescribeDomains(t *testing.T) {
	client := NewTestClientNew()
	describeArgs := DescribeDomainInfoArgs{
		DomainName: TestDomainName,
	}

	_, err := client.DescribeDomainInfo(&describeArgs)
	if err == nil {
		t.Logf("DescribeDomains success")
	} else {
		t.Errorf("Failed to DescribeDomains: %v", err)
	}
}
