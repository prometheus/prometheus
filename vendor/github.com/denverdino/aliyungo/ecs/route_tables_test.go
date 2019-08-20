package ecs

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func testRouteTable(t *testing.T, client *Client, regionId common.Region, vpcId string, vrouterId string, routeTableId string, instanceId string) {
	cidrBlock := "0.0.0.0/0"
	createArgs := CreateRouteEntryArgs{
		RouteTableId:         routeTableId,
		DestinationCidrBlock: cidrBlock,
		NextHopType:          NextHopIntance,
		NextHopId:            instanceId,
		ClientToken:          client.GenerateClientToken(),
	}

	err := client.CreateRouteEntry(&createArgs)
	if err != nil {
		t.Errorf("Failed to create route entry: %v", err)
	}

	describeArgs := DescribeRouteTablesArgs{
		VRouterId: vrouterId,
	}

	routeTables, _, err := client.DescribeRouteTables(&describeArgs)

	if err != nil {
		t.Errorf("Failed to describe route tables: %v", err)
	} else {
		t.Logf("RouteTables of VRouter %s: %++v", vrouterId, routeTables)
	}

	err = client.WaitForAllRouteEntriesAvailable(vrouterId, routeTableId, 60)
	if err != nil {
		t.Errorf("Failed to wait route entries: %v", err)
	}
	deleteArgs := DeleteRouteEntryArgs{
		RouteTableId:         routeTableId,
		DestinationCidrBlock: cidrBlock,
		NextHopId:            instanceId,
	}

	err = client.DeleteRouteEntry(&deleteArgs)
	if err != nil {
		t.Errorf("Failed to delete route entry: %v", err)
	}

}
