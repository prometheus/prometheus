package ess

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestEssScalingConfigurationCreationAndDeletion(t *testing.T) {

	if TestIAmRich == false {
		// Avoid payment
		return
	}

	client := NewTestClient(common.Region(RegionId))
	ecsClient := NewTestEcsClient(common.Region(RegionId))

	ecs, err := ecsClient.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to describe ecs %s: %v", TestInstanceId, err)
	}
	t.Logf("ECS instance: %++v  %v", ecs, err)

	cArgs := CreateScalingConfigurationArgs{
		ScalingGroupId:  ScalingGroupId,
		ImageId:         ecs.ImageId,
		InstanceType:    ecs.InstanceType,
		SecurityGroupId: ecs.SecurityGroupIds.SecurityGroupId[0],
	}

	csc, err := client.CreateScalingConfiguration(&cArgs)
	if err != nil {
		t.Errorf("Failed to create scaling configuration %v", err)
	}
	configurationId := csc.ScalingConfigurationId
	t.Logf("Configuration %s is created successfully.", configurationId)

	sArgs := DescribeScalingConfigurationsArgs{
		RegionId:               RegionId,
		ScalingGroupId:         ScalingGroupId,
		ScalingConfigurationId: []string{configurationId},
	}
	sResp, _, err := client.DescribeScalingConfigurations(&sArgs)
	if len(sResp) < 1 {
		t.Fatalf("Failed to describe confituration %s", configurationId)
	}

	config := sResp[0]
	t.Logf("Configuration: %++v  %v", config, err)

	dcArgs := DeleteScalingConfigurationArgs{
		ScalingGroupId:         ScalingGroupId,
		ScalingConfigurationId: configurationId,
		ImageId:                config.ImageId,
	}

	_, err = client.DeleteScalingConfiguration(&dcArgs)
	if err != nil {
		t.Errorf("Failed to delete scaling configuration %v", err)
	}

	t.Logf("Configuration %s is deleted successfully.", configurationId)

}
