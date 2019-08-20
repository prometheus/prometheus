package ecs

import "testing"

func TestDescribeNatGateway(t *testing.T) {

	client := NewTestClient()
	args := DescribeBandwidthPackagesArgs{
		RegionId:           "cn-beijing",
		BandwidthPackageId: "bwp-2zes6svn910zjqhcyqnxm",
		NatGatewayId:       "ngw-2zex6oklf8901t76yut6c",
	}
	packages, err := client.DescribeBandwidthPackages(&args)
	if err != nil {
		t.Fatalf("Failed to DescribeBandwidthPackages: %v", err)
	}
	for _, pack := range packages.BandwidthPackages.BandwidthPackage {
		t.Logf("pack.IpCount: %++v", pack.IpCount)
		t.Logf("pack.Bandwidth: %++v", pack.Bandwidth)
		t.Logf("pack.ZoneId: %++v", pack.ZoneId)
		t.Logf("pack.ipAddress: %++v", len(pack.PublicIpAddresses.PublicIpAddresse))
	}
}

func TestClient_DescribeNatGateways(t *testing.T) {
	client := NewVpcTestClientForDebug()
	args := &DescribeNatGatewaysArgs{
		RegionId: TestRegionID,
		VpcId:    TestVpcId,
	}

	natGateways, _, err := client.DescribeNatGateways(args)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		for index, item := range natGateways {
			t.Logf("natGateways[%d]=%++v", index, item)
		}
	}
}
