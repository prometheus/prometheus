package location

import "testing"

func TestDescribeRegions(t *testing.T) {
	client := NewTestClientForDebug()

	args := &DescribeRegionsArgs{
	//RegionId: common.Beijing,
	}

	regions, err := client.DescribeRegions(args)
	if err != nil {
		t.Fatalf("Failed to describe regions %++v", err)
	}

	t.Logf("regions is %++v", regions)
}
