package ess

import (
	"time"

	"github.com/denverdino/aliyungo/common"
)

type LifecycleState string

const (
	Active    = LifecycleState("Active")
	Inacitve  = LifecycleState("Inactive")
	Deleting  = LifecycleState("Deleting")
	InService = LifecycleState("InService")
	Pending   = LifecycleState("Pending")
	Removing  = LifecycleState("Removing")
)

type CreateScalingGroupArgs struct {
	RegionId         common.Region
	ScalingGroupName string
	LoadBalancerIds  string
	VpcId            string
	VSwitchId        string
	VSwitchIds       common.FlattenArray
	// NOTE: Set MinSize, MaxSize and DefaultCooldown type to int pointer to distinguish zero value from unset value.
	MinSize         *int
	MaxSize         *int
	DefaultCooldown *int
	RemovalPolicy   common.FlattenArray
	DBInstanceIds   string
}

type CreateScalingGroupResponse struct {
	common.Response
	ScalingGroupId string
}

// CreateScalingGroup create scaling group
//
// You can read doc at https://help.aliyun.com/document_detail/25936.html?spm=5176.doc25940.6.617.vm6LXF
func (client *Client) CreateScalingGroup(args *CreateScalingGroupArgs) (resp *CreateScalingGroupResponse, err error) {
	response := CreateScalingGroupResponse{}
	err = client.InvokeByFlattenMethod("CreateScalingGroup", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type ModifyScalingGroupArgs struct {
	ScalingGroupId               string
	ScalingGroupName             string
	ActiveScalingConfigurationId string
	// NOTE: Set MinSize/MaxSize type to int pointer to distinguish zero value from unset value.
	MinSize         *int
	MaxSize         *int
	DefaultCooldown *int
	RemovalPolicy   common.FlattenArray
}

type ModifyScalingGroupResponse struct {
	common.Response
}

// ModifyScalingGroup modify scaling group
//
// You can read doc at https://help.aliyun.com/document_detail/25937.html?spm=5176.doc25936.6.618.iwDcXT
func (client *Client) ModifyScalingGroup(args *ModifyScalingGroupArgs) (resp *ModifyScalingGroupResponse, err error) {
	response := ModifyScalingGroupResponse{}
	err = client.InvokeByFlattenMethod("ModifyScalingGroup", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type DescribeScalingGroupsArgs struct {
	RegionId         common.Region
	ScalingGroupId   common.FlattenArray
	ScalingGroupName common.FlattenArray
	common.Pagination
}

type DescribeInstancesResponse struct {
	common.Response
	common.PaginationResult
	ScalingGroups struct {
		ScalingGroup []ScalingGroupItemType
	}
}

type ScalingGroupItemType struct {
	ScalingGroupId               string
	ScalingGroupName             string
	ActiveScalingConfigurationId string
	RegionId                     string
	LoadBalancerId               string
	VSwitchId                    string
	VSwitchIds                   VSwitchIdsSetType
	CreationTime                 string
	LifecycleState               LifecycleState
	MinSize                      int
	MaxSize                      int
	DefaultCooldown              int
	TotalCapacity                int
	ActiveCapacity               int
	PendingCapacity              int
	RemovingCapacity             int
	RemovalPolicies              RemovalPolicySetType
	DBInstanceIds                DBInstanceIdSetType
	LoadBalancerIds              LoadBalancerIdSetType
}

type VSwitchIdsSetType struct {
	VSwitchId []string
}

type RemovalPolicySetType struct {
	RemovalPolicy []string
}

type DBInstanceIdSetType struct {
	DBInstanceId []string
}
type LoadBalancerIdSetType struct {
	LoadBalancerId []string
}

// DescribeScalingGroups describes scaling groups
//
// You can read doc at https://help.aliyun.com/document_detail/25938.html?spm=5176.doc25937.6.619.sUUOT7
func (client *Client) DescribeScalingGroups(args *DescribeScalingGroupsArgs) (groups []ScalingGroupItemType, pagination *common.PaginationResult, err error) {
	args.Validate()
	response := DescribeInstancesResponse{}

	err = client.InvokeByFlattenMethod("DescribeScalingGroups", args, &response)

	if err == nil {
		return response.ScalingGroups.ScalingGroup, &response.PaginationResult, nil
	}

	return nil, nil, err
}

type DescribeScalingInstancesArgs struct {
	RegionId               common.Region
	ScalingGroupId         string
	ScalingConfigurationId string
	HealthStatus           string
	CreationType           string
	LifecycleState         LifecycleState
	InstanceId             common.FlattenArray
	common.Pagination
}

type DescribeScalingInstancesResponse struct {
	common.Response
	common.PaginationResult
	ScalingInstances struct {
		ScalingInstance []ScalingInstanceItemType
	}
}

type ScalingInstanceItemType struct {
	InstanceId             string
	ScalingGroupId         string
	ScalingConfigurationId string
	HealthStatus           string
	CreationTime           string
	CreationType           string
	LifecycleState         LifecycleState
}

// DescribeScalingInstances describes scaling instances
//
// You can read doc at https://help.aliyun.com/document_detail/25942.html?spm=5176.doc25941.6.623.2xA0Uj
func (client *Client) DescribeScalingInstances(args *DescribeScalingInstancesArgs) (instances []ScalingInstanceItemType, pagination *common.PaginationResult, err error) {
	args.Validate()
	response := DescribeScalingInstancesResponse{}

	err = client.InvokeByFlattenMethod("DescribeScalingInstances", args, &response)

	if err == nil {
		return response.ScalingInstances.ScalingInstance, &response.PaginationResult, nil
	}

	return nil, nil, err
}

type EnableScalingGroupArgs struct {
	ScalingGroupId               string
	ActiveScalingConfigurationId string
	InstanceId                   common.FlattenArray
}

type EnableScalingGroupResponse struct {
	common.Response
}

// EnableScalingGroup enable scaling group
//
// You can read doc at https://help.aliyun.com/document_detail/25939.html?spm=5176.doc25938.6.620.JiJhkx
func (client *Client) EnableScalingGroup(args *EnableScalingGroupArgs) (resp *EnableScalingGroupResponse, err error) {
	response := EnableScalingGroupResponse{}
	err = client.InvokeByFlattenMethod("EnableScalingGroup", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type DisableScalingGroupArgs struct {
	ScalingGroupId string
}

type DisableScalingGroupResponse struct {
	common.Response
}

// DisableScalingGroup disable scaling group
//
// You can read doc at https://help.aliyun.com/document_detail/25940.html?spm=5176.doc25939.6.621.M8GuuY
func (client *Client) DisableScalingGroup(args *DisableScalingGroupArgs) (resp *DisableScalingGroupResponse, err error) {
	response := DisableScalingGroupResponse{}
	err = client.InvokeByFlattenMethod("DisableScalingGroup", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type DeleteScalingGroupArgs struct {
	ScalingGroupId string
	ForceDelete    bool
}

type DeleteScalingGroupResponse struct {
	common.Response
}

// DeleteScalingGroup delete scaling group
//
// You can read doc at https://help.aliyun.com/document_detail/25941.html?spm=5176.doc25940.6.622.mRBCuw
func (client *Client) DeleteScalingGroup(args *DeleteScalingGroupArgs) (resp *DeleteScalingGroupResponse, err error) {
	response := DeleteScalingGroupResponse{}
	err = client.InvokeByFlattenMethod("DeleteScalingGroup", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type AttachInstancesArgs struct {
	ScalingGroupId string
	InstanceId     common.FlattenArray
}

type AttachInstancesResponse struct {
	common.Response
	ScalingActivityId string
}

type RemoveInstancesArgs struct {
	ScalingGroupId string
	InstanceId     common.FlattenArray
}

type RemoveInstancesResponse struct {
	common.Response
	ScalingActivityId string
}

// AttachInstances attach instances to scaling group
//
// You can read doc at https://help.aliyun.com/document_detail/25954.html?spm=5176.product25855.6.633.y5gmzX
func (client *Client) AttachInstances(args *AttachInstancesArgs) (resp *AttachInstancesResponse, err error) {
	response := AttachInstancesResponse{}
	err = client.InvokeByFlattenMethod("AttachInstances", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

// RemoveInstances detach instances from scaling group
//
// You can read doc at https://help.aliyun.com/document_detail/25955.html?spm=5176.doc25954.6.634.GtpzuJ
func (client *Client) RemoveInstances(args *RemoveInstancesArgs) (resp *RemoveInstancesResponse, err error) {
	response := RemoveInstancesResponse{}
	err = client.InvokeByFlattenMethod("RemoveInstances", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

// Default timeout value for WaitForInstance method
const DefaultWaitTimeout = 120
const DefaultWaitForInterval = 5

// WaitForScalingGroup waits for group to given status
func (client *Client) WaitForScalingGroup(regionId common.Region, groupId string, status LifecycleState, timeout int) error {
	if timeout <= 0 {
		timeout = DefaultWaitTimeout
	}
	for {
		sgs, _, err := client.DescribeScalingGroups(&DescribeScalingGroupsArgs{
			RegionId:       regionId,
			ScalingGroupId: []string{groupId},
		})
		if err != nil {
			return err
		}

		if timeout <= 0 {
			return common.GetClientErrorFromString("Timeout")
		}

		timeout = timeout - DefaultWaitForInterval
		time.Sleep(DefaultWaitForInterval * time.Second)

		if len(sgs) < 1 {
			return common.GetClientErrorFromString("Not found")
		}
		if sgs[0].LifecycleState == status {
			break
		}

	}
	return nil
}
