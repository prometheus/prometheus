package slb

import "testing"

func TestDescribeRegions(t *testing.T) {

	client := NewTestNewSLBClientForDebug()

	regions, err := client.DescribeRegions()

	if err == nil {
		t.Logf("regions: %v", regions)
	} else {
		t.Errorf("Failed to DescribeRegions: %v", err)
	}

}
