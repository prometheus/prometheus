package ecs

import "testing"

func TestDescribeSnatTableEntry(t *testing.T) {

	client := NewTestClient()
	args := DescribeSnatTableEntriesArgs{
		RegionId:    "cn-beijing",
		SnatTableId: "stb-abc",
	}
	_, _, err := client.DescribeSnatTableEntries(&args)
	if err != nil {
		t.Fatalf("Failed to DescribeBandwidthPackages: %v", err)
	}
}
