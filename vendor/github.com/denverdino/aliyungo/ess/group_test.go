package ess

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestEssScalingGroupCreationAndDeletion(t *testing.T) {

	if TestIAmRich == false {
		// Avoid payment
		return
	}

	client := NewTestClient(common.Region(RegionId))

	maxSize := 1
	minSize := 1

	args := CreateScalingGroupArgs{
		RegionId:         common.Region(RegionId),
		ScalingGroupName: "test_sg",
		MaxSize:          &maxSize,
		MinSize:          &minSize,
		RemovalPolicy:    []string{"OldestInstance", "NewestInstance"},
	}

	resp, err := client.CreateScalingGroup(&args)
	if err != nil {
		t.Errorf("Failed to create scaling group %v", err)
	}
	instanceId := resp.ScalingGroupId
	t.Logf("Instance %s is created successfully.", instanceId)

	defaultCooldown := 200

	mArgs := ModifyScalingGroupArgs{
		ScalingGroupId:   instanceId,
		ScalingGroupName: "sg_modify",
		DefaultCooldown:  &defaultCooldown,
	}

	_, err = client.ModifyScalingGroup(&mArgs)
	if err != nil {
		t.Errorf("Failed to modify scaling group %v", err)
	}

	arrtArgs := DescribeScalingGroupsArgs{
		RegionId:       common.Region(RegionId),
		ScalingGroupId: []string{instanceId},
	}
	attrResp, _, err := client.DescribeScalingGroups(&arrtArgs)
	t.Logf("Instance: %++v  %v", attrResp[0], err)

	iArgs := DescribeScalingInstancesArgs{
		RegionId:       common.Region(RegionId),
		ScalingGroupId: instanceId,
	}
	iResp, _, err := client.DescribeScalingInstances(&iArgs)
	if len(iResp) < 1 {
		t.Logf("Scaling ecs instances empty.")
	} else {
		t.Logf("ECS: %++v  %v", iResp[0], err)
	}

	dArgs := DeleteScalingGroupArgs{
		ScalingGroupId: instanceId,
		ForceDelete:    true,
	}
	_, err = client.DeleteScalingGroup(&dArgs)

	if err != nil {
		t.Errorf("Failed to delete instance %s: %v", instanceId, err)
	}
	t.Logf("Instance %s is deleted successfully.", instanceId)
}
