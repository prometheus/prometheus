package ess

import (
	"encoding/base64"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
)

type CreateScalingConfigurationArgs struct {
	ScalingGroupId           string
	ImageId                  string
	InstanceType             string
	IoOptimized              ecs.IoOptimized
	SecurityGroupId          string
	ScalingConfigurationName string
	InternetChargeType       common.InternetChargeType
	InternetMaxBandwidthIn   int
	InternetMaxBandwidthOut  *int
	SystemDisk_Category      common.UnderlineString
	SystemDisk_Size          common.UnderlineString
	DataDisk                 []DataDiskType
	UserData                 string
	KeyPairName              string
	RamRoleName              string
	Tags                     string
	InstanceName             string
}

type DataDiskType struct {
	Category   string
	SnapshotId string
	Device     string
	Size       int
}

type CreateScalingConfigurationResponse struct {
	ScalingConfigurationId string
	common.Response
}

// CreateScalingConfiguration create scaling configuration
//
// You can read doc at https://help.aliyun.com/document_detail/25944.html?spm=5176.doc25942.6.625.KcE5ir
func (client *Client) CreateScalingConfiguration(args *CreateScalingConfigurationArgs) (resp *CreateScalingConfigurationResponse, err error) {
	if args.UserData != "" {
		// Encode to base64 string
		args.UserData = base64.StdEncoding.EncodeToString([]byte(args.UserData))
	}
	response := CreateScalingConfigurationResponse{}
	err = client.InvokeByFlattenMethod("CreateScalingConfiguration", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type DescribeScalingConfigurationsArgs struct {
	RegionId                 common.Region
	ScalingGroupId           string
	ScalingConfigurationId   common.FlattenArray
	ScalingConfigurationName common.FlattenArray
	common.Pagination
}

type DescribeScalingConfigurationsResponse struct {
	common.Response
	common.PaginationResult
	ScalingConfigurations struct {
		ScalingConfiguration []ScalingConfigurationItemType
	}
}
type TagItemType struct {
	Key   string
	Value string
}

type ScalingConfigurationItemType struct {
	ScalingConfigurationId   string
	ScalingConfigurationName string
	ScalingGroupId           string
	ImageId                  string
	InstanceType             string
	InstanceName             string
	IoOptimized              string
	SecurityGroupId          string
	InternetChargeType       string
	LifecycleState           LifecycleState
	CreationTime             string
	InternetMaxBandwidthIn   int
	InternetMaxBandwidthOut  int
	SystemDiskCategory       string
	DataDisks                struct {
		DataDisk []DataDiskItemType
	}
	KeyPairName string
	RamRoleName string
	UserData    string
	Tags        struct {
		Tag []TagItemType
	}
}

type DataDiskItemType struct {
	Size       int
	Category   string
	SnapshotId string
	Device     string
}

// DescribeScalingConfigurations describes scaling configuration
//
// You can read doc at https://help.aliyun.com/document_detail/25945.html?spm=5176.doc25944.6.626.knG0zz
func (client *Client) DescribeScalingConfigurations(args *DescribeScalingConfigurationsArgs) (configs []ScalingConfigurationItemType, pagination *common.PaginationResult, err error) {
	args.Validate()
	response := DescribeScalingConfigurationsResponse{}

	err = client.InvokeByFlattenMethod("DescribeScalingConfigurations", args, &response)

	if err == nil {
		return response.ScalingConfigurations.ScalingConfiguration, &response.PaginationResult, nil
	}

	return nil, nil, err
}

type DeleteScalingConfigurationArgs struct {
	ScalingConfigurationId string
	ScalingGroupId         string
	ImageId                string
}

type DeleteScalingConfigurationResponse struct {
	common.Response
}

// DeleteScalingConfiguration delete scaling configuration
//
// You can read doc at https://help.aliyun.com/document_detail/25946.html?spm=5176.doc25944.6.627.MjkuuL
func (client *Client) DeleteScalingConfiguration(args *DeleteScalingConfigurationArgs) (resp *DeleteScalingConfigurationResponse, err error) {
	response := DeleteScalingConfigurationResponse{}
	err = client.InvokeByFlattenMethod("DeleteScalingConfiguration", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type DeactivateScalingConfigurationArgs struct {
	ScalingConfigurationId string
}

type DeactivateScalingConfigurationResponse struct {
	common.Response
}

// DeactivateScalingConfiguration deactivate scaling configuration
//
func (client *Client) DeactivateScalingConfiguration(args *DeactivateScalingConfigurationArgs) (resp *DeactivateScalingConfigurationResponse, err error) {
	response := DeactivateScalingConfigurationResponse{}
	err = client.InvokeByFlattenMethod("DeactivateScalingConfiguration", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}
