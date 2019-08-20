package ecs

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func testCreateVSwitch(t *testing.T, client *Client, regionId common.Region, zoneId string, vpcId string, vrouterId string) (vSwitchId string, err error) {

	args := CreateVSwitchArgs{
		ZoneId:      zoneId,
		CidrBlock:   "172.16.10.0/24",
		VpcId:       vpcId,
		VSwitchName: "AliyunGo_test_vSwitch",
		Description: "AliyunGo test vSwitch",
		ClientToken: client.GenerateClientToken(),
	}

	vSwitchId, err = client.CreateVSwitch(&args)

	if err != nil {
		t.Errorf("Failed to create VSwitch: %v", err)
		return "", err
	}

	t.Logf("VSwitch is created successfully: %s", vSwitchId)

	err = client.WaitForVSwitchAvailable(vpcId, vSwitchId, 60)
	if err != nil {
		t.Errorf("Failed to wait VSwitch %s to available: %v", vSwitchId, err)
	}

	newName := args.VSwitchName + "_update"
	modifyArgs := ModifyVSwitchAttributeArgs{
		VSwitchId:   vSwitchId,
		VSwitchName: newName,
		Description: newName,
	}

	err = client.ModifyVSwitchAttribute(&modifyArgs)
	if err != nil {
		t.Errorf("Failed to modify VSwitch %s: %v", vSwitchId, err)
	}

	argsDescribe := DescribeVSwitchesArgs{
		VpcId:     vpcId,
		VSwitchId: vSwitchId,
	}

	vswitches, _, err := client.DescribeVSwitches(&argsDescribe)
	if err != nil {
		t.Errorf("Failed to describe VSwitch: %v", err)
	}

	if vswitches[0].VSwitchName != newName {
		t.Errorf("Failed to modify VSwitch with new name: %s", newName)
	}

	t.Logf("VSwitch is : %++v", vswitches)

	return vSwitchId, err
}
