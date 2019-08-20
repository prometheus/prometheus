package ecs

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func testVRouter(t *testing.T, client *Client, regionId common.Region, vpcId string, vrouterId string, instanceId string) {

	newName := "My_Aliyun_test_VRouter"

	modifyArgs := ModifyVRouterAttributeArgs{
		VRouterId:   vrouterId,
		VRouterName: newName,
		Description: newName,
	}

	err := client.ModifyVRouterAttribute(&modifyArgs)
	if err != nil {
		t.Errorf("Failed to modify VRouters: %v", err)
	}

	args := DescribeVRoutersArgs{
		VRouterId: vrouterId,
		RegionId:  regionId,
	}

	vrouters, _, err := client.DescribeVRouters(&args)
	if err != nil {
		t.Errorf("Failed to describe VRouters: %v", err)
	}
	t.Logf("VRouters: %++v", vrouters)
	if vrouters[0].VRouterName != newName {
		t.Errorf("Failed to modify VRouters with new name: %s", newName)
	}

	testRouteTable(t, client, regionId, vpcId, vrouterId, vrouters[0].RouteTableIds.RouteTableId[0], instanceId)

}
