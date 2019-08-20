package ess

import (
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
)

const (
	TestAccessKeyId        = "MY_ACCESS_KEY_ID"
	TestAccessKeySecret    = "MY_ACCESS_KEY_SECRET"
	RegionId               = "MY_TEST_REGION"
	ScalingGroupId         = "MY_TEST_SCALING_GROUP_ID"
	TestInstanceId         = "MY_TEST_INSTANCE_ID"
	TestRuleArn            = "MY_TEST_RULE_ARN"
	TestScheduleLaunchTime = "MY_TEST_SCHEDULE_LAUNCH_TIME"
	TestIAmRich            = false
)

var testClient *Client

func NewTestClient(regionId common.Region) *Client {
	if testClient == nil {
		testClient = NewESSClient(TestAccessKeyId, TestAccessKeySecret, regionId)
	}
	return testClient
}

var testDebugClient *Client

func NewTestClientForDebug(regionId common.Region) *Client {
	if testDebugClient == nil {
		testDebugClient = NewESSClient(TestAccessKeyId, TestAccessKeySecret, regionId)
		testDebugClient.SetDebug(true)
	}
	return testDebugClient
}

var testEcsClient *ecs.Client

func NewTestEcsClient(regionId common.Region) *ecs.Client {
	if testEcsClient == nil {
		testEcsClient = ecs.NewECSClient(TestAccessKeyId, TestAccessKeySecret, regionId)
	}
	return testEcsClient
}
