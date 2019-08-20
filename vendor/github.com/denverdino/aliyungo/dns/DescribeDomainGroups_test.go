package dns

import "testing"

func TestDescribeDomainGroups(t *testing.T) {
	client := NewTestClientNew()
	describeArgs := DescribeDomainGroupsArgs{}

	_, err := client.DescribeDomainGroups(&describeArgs)
	if err == nil {
		t.Logf("DescribeDomainGroups success")
	} else {
		t.Errorf("Failed to DescribeDomainGroups: %v", err)
	}
}
