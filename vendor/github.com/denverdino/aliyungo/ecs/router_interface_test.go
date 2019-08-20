package ecs

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestRouteInterface(t *testing.T) {

	client := NewTestClient()

	routerId := "****"
	oldSpec, newSpec := Spec("Middle.1"), Spec("Middle.2")
	initName, newName := "initName", "newName"
	initDesc, newDesc := "initDesc", "newDesc"
	oppositeRouterId, oppositeInterfaceId, oppositeInterfaceOwnerId := "***", "****", "*****"

	createArgs := CreateRouterInterfaceArgs{
		RegionId:           common.Beijing,
		RouterType:         VRouter,
		RouterId:           routerId,
		Role:               InitiatingSide,
		Spec:               oldSpec,
		OppositeRegionId:   common.Hangzhou,
		OppositeRouterType: VRouter,
		Name:               initName,
		Description:        initDesc,
	}
	resp, err := client.CreateRouterInterface(&createArgs)
	if err != nil {
		t.Errorf("Failed to create route interface: %v", err)
	}

	var filter []Filter
	filter = append(filter, Filter{Key: "RouterId", Value: []string{createArgs.RouterId}})
	describeArgs := DescribeRouterInterfacesArgs{
		RegionId: common.Beijing,
		Filter:   filter,
	}
	_, err = client.DescribeRouterInterfaces(&describeArgs)
	if err != nil {
		t.Errorf("Failed to describe route interfaces: %v", err)
	}

	modifyArgs := ModifyRouterInterfaceSpecArgs{
		Spec:              newSpec,
		RegionId:          createArgs.RegionId,
		RouterInterfaceId: resp.RouterInterfaceId,
	}
	_, err = client.ModifyRouterInterfaceSpec(&modifyArgs)
	if err != nil {
		t.Errorf("Failed to modify route interface spec: %v", err)
	}

	modifyAArgs := ModifyRouterInterfaceAttributeArgs{
		RegionId:                 createArgs.RegionId,
		RouterInterfaceId:        resp.RouterInterfaceId,
		Name:                     newName,
		Description:              newDesc,
		OppositeRouterId:         oppositeRouterId,
		OppositeInterfaceId:      oppositeInterfaceId,
		OppositeInterfaceOwnerId: oppositeInterfaceOwnerId,
	}
	_, err = client.ModifyRouterInterfaceAttribute(&modifyAArgs)
	if err != nil {
		t.Errorf("Failed to modify route interface spec: %v", err)
	}

	deleteArgs := OperateRouterInterfaceArgs{
		RegionId:          createArgs.RegionId,
		RouterInterfaceId: resp.RouterInterfaceId,
	}
	_, err = client.DeleteRouterInterface(&deleteArgs)
	if err != nil {
		t.Errorf("Failed to delete route interface: %v", err)
	}
}
