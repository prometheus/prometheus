package slb

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestLoadBalancer(t *testing.T) {

	client := NewTestClientForDebug()

	creationArgs := CreateLoadBalancerArgs{
		RegionId:         common.Beijing,
		LoadBalancerName: "test-slb",
		AddressType:      InternetAddressType,
		ClientToken:      client.GenerateClientToken(),
	}

	response, err := client.CreateLoadBalancer(&creationArgs)
	if err != nil {
		t.Fatalf("Failed to CreateLoadBalancer: %v", err)
	}

	t.Logf("CreateLoadBalancer result: %v", *response)
	lbId := response.LoadBalancerId

	testBackendServers(t, client, lbId)
	testListeners(t, client, lbId)

	describeLoadBalancersArgs := DescribeLoadBalancersArgs{
		RegionId: common.Beijing,
	}

	loadBalancers, err := client.DescribeLoadBalancers(&describeLoadBalancersArgs)

	if err != nil {
		t.Fatalf("Failed to DescribeLoadBalancers: %v", err)
	}
	t.Logf("DescribeLoadBalancers result: %++v", loadBalancers)

	err = client.SetLoadBalancerStatus(lbId, InactiveStatus)
	if err != nil {
		t.Fatalf("Failed to SetLoadBalancerStatus: %v", err)
	}
	err = client.SetLoadBalancerName(lbId, "test-slb2")
	if err != nil {
		t.Fatalf("Failed to SetLoadBalancerName: %v", err)
	}
	loadBalancer, err := client.DescribeLoadBalancerAttribute(lbId)

	if err != nil {
		t.Fatalf("Failed to DescribeLoadBalancerAttribute: %v", err)
	}
	t.Logf("DescribeLoadBalancerAttribute result: %++v", loadBalancer)

	err = client.DeleteLoadBalancer(lbId)
	if err != nil {
		t.Errorf("Failed to DeleteLoadBalancer: %v", err)
	}

	t.Logf("DeleteLoadBalancer successfully: %s", lbId)

}

func TestLoadBalancerIPv6(t *testing.T) {

	client := NewTestClientForDebug()

	creationArgs := CreateLoadBalancerArgs{
		RegionId:         common.Hangzhou,
		LoadBalancerName: "test-slb-ipv6",
		AddressType:      InternetAddressType,
		MasterZoneId:     "cn-hangzhou-e",
		SlaveZoneId:      "cn-hangzhou-f",
		ClientToken:      client.GenerateClientToken(),
		AddressIPVersion: IPv6,
	}

	response, err := client.CreateLoadBalancer(&creationArgs)
	if err != nil {
		t.Fatalf("Failed to CreateLoadBalancer: %v", err)
	}

	t.Logf("CreateLoadBalancer result: %v", *response)
	lbId := response.LoadBalancerId

	describeLoadBalancersArgs := DescribeLoadBalancersArgs{
		RegionId: common.Hangzhou,
	}

	loadBalancers, err := client.DescribeLoadBalancers(&describeLoadBalancersArgs)

	if err != nil {
		t.Fatalf("Failed to DescribeLoadBalancers: %v", err)
	}
	t.Logf("DescribeLoadBalancers result: %++v", loadBalancers)

	err = client.SetLoadBalancerStatus(lbId, InactiveStatus)
	if err != nil {
		t.Fatalf("Failed to SetLoadBalancerStatus: %v", err)
	}
	err = client.SetLoadBalancerName(lbId, "test-slb2")
	if err != nil {
		t.Fatalf("Failed to SetLoadBalancerName: %v", err)
	}
	loadBalancer, err := client.DescribeLoadBalancerAttribute(lbId)

	if err != nil {
		t.Fatalf("Failed to DescribeLoadBalancerAttribute: %v", err)
	}
	t.Logf("DescribeLoadBalancerAttribute result: %++v", loadBalancer)

	err = client.DeleteLoadBalancer(lbId)
	if err != nil {
		t.Errorf("Failed to DeleteLoadBalancer: %v", err)
	}

	t.Logf("DeleteLoadBalancer successfully: %s", lbId)

}

func TestClient_DescribeLoadBalancers(t *testing.T) {
	client := NewTestNewSLBClientForDebug()
	//client.SetSecurityToken(TestSecurityToken)

	args := &DescribeLoadBalancersArgs{
		RegionId: TestRegionID,
		//SecurityToken: TestSecurityToken,
	}

	slbs, err := client.DescribeLoadBalancers(args)
	if err != nil {
		t.Fatalf("Failed %++v", err)
	} else {
		t.Logf("Result = %++v", slbs)
	}
}
