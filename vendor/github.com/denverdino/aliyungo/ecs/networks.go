// API on Network

package ecs

import (
	"time"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

type AllocatePublicIpAddressArgs struct {
	InstanceId string
}

type AllocatePublicIpAddressResponse struct {
	common.Response

	IpAddress string
}

// AllocatePublicIpAddress allocates Public Ip Address
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/network&allocatepublicipaddress
func (client *Client) AllocatePublicIpAddress(instanceId string) (ipAddress string, err error) {
	args := AllocatePublicIpAddressArgs{
		InstanceId: instanceId,
	}
	response := AllocatePublicIpAddressResponse{}
	err = client.Invoke("AllocatePublicIpAddress", &args, &response)
	if err != nil {
		return "", err
	}
	return response.IpAddress, nil
}

type ModifyInstanceNetworkSpec struct {
	InstanceId              string
	InternetMaxBandwidthOut *int
	InternetMaxBandwidthIn  *int
	NetworkChargeType       common.InternetChargeType
}

type ModifyInstanceNetworkSpecResponse struct {
	common.Response
}

// ModifyInstanceNetworkSpec modifies instance network spec
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/network&modifyinstancenetworkspec
func (client *Client) ModifyInstanceNetworkSpec(args *ModifyInstanceNetworkSpec) error {

	response := ModifyInstanceNetworkSpecResponse{}
	return client.Invoke("ModifyInstanceNetworkSpec", args, &response)
}

type AllocateEipAddressArgs struct {
	RegionId           common.Region
	Bandwidth          int
	InternetChargeType common.InternetChargeType
	ISP                string
	ClientToken        string
}

type AllocateEipAddressResponse struct {
	common.Response
	EipAddress   string
	AllocationId string
}

// AllocateEipAddress allocates Eip Address
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/network&allocateeipaddress
func (client *Client) AllocateEipAddress(args *AllocateEipAddressArgs) (EipAddress string, AllocationId string, err error) {
	if args.Bandwidth == 0 {
		args.Bandwidth = 5
	}
	response := AllocateEipAddressResponse{}
	err = client.Invoke("AllocateEipAddress", args, &response)
	if err != nil {
		return "", "", err
	}
	return response.EipAddress, response.AllocationId, nil
}

type EipInstanceType string

const (
	EcsInstance = "EcsInstance"
	SlbInstance = "SlbInstance"
	Nat         = "Nat"
	HaVip       = "HaVip"
)

type AssociateEipAddressArgs struct {
	AllocationId string
	InstanceId   string
	InstanceType EipInstanceType
}

type AssociateEipAddressResponse struct {
	common.Response
}

// AssociateEipAddress associates EIP address to VM instance
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/network&associateeipaddress
func (client *Client) AssociateEipAddress(allocationId string, instanceId string) error {
	args := AssociateEipAddressArgs{
		AllocationId: allocationId,
		InstanceId:   instanceId,
	}
	response := ModifyInstanceNetworkSpecResponse{}
	return client.Invoke("AssociateEipAddress", &args, &response)
}

func (client *Client) NewAssociateEipAddress(args *AssociateEipAddressArgs) error {
	response := ModifyInstanceNetworkSpecResponse{}
	return client.Invoke("AssociateEipAddress", args, &response)
}

// Status of disks
type EipStatus string

const (
	EipStatusAssociating   = EipStatus("Associating")
	EipStatusUnassociating = EipStatus("Unassociating")
	EipStatusInUse         = EipStatus("InUse")
	EipStatusAvailable     = EipStatus("Available")
)

type AssociatedInstanceType string

const (
	AssociatedInstanceTypeEcsInstance = AssociatedInstanceType("EcsInstance")
	AssociatedInstanceTypeSlbInstance = AssociatedInstanceType("SlbInstance")
	AssociatedInstanceTypeNat         = AssociatedInstanceType("Nat")
	AssociatedInstanceTypeHaVip       = AssociatedInstanceType("HaVip")
)

type DescribeEipAddressesArgs struct {
	RegionId               common.Region
	Status                 EipStatus //enum Associating | Unassociating | InUse | Available
	EipAddress             string
	AllocationId           string
	AssociatedInstanceType AssociatedInstanceType //enum EcsInstance | SlbInstance | Nat | HaVip
	AssociatedInstanceId   string                 //绑定的资源的Id。 这是一个过滤器性质的参数，若不指定，则表示不适用该条件对结果进行过滤。 如果要使用该过滤器，必须同时使用AssociatedInstanceType。若InstanceType为EcsInstance，则此处填写ECS实例Id。若InstanceType为SlbInstance，则此处填写VPC类型的私网SLB 的实例ID。若InstanceType为Nat，则此处填写NAT 的实例ID。。若InstanceType为HaVip，则此处填写HaVipId。
	common.Pagination
}

//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/datatype&eipaddresssettype
type EipAddressSetType struct {
	RegionId           common.Region
	IpAddress          string
	AllocationId       string
	Status             EipStatus
	InstanceId         string
	InstanceType       string
	Bandwidth          string // Why string
	InternetChargeType common.InternetChargeType
	OperationLocks     OperationLocksType
	AllocationTime     util.ISO6801Time
}

type DescribeEipAddressesResponse struct {
	common.Response
	common.PaginationResult
	EipAddresses struct {
		EipAddress []EipAddressSetType
	}
}

// DescribeInstanceStatus describes instance status
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/network&describeeipaddresses
func (client *Client) DescribeEipAddresses(args *DescribeEipAddressesArgs) (eipAddresses []EipAddressSetType, pagination *common.PaginationResult, err error) {
	response, err := client.DescribeEipAddressesWithRaw(args)
	if err == nil {
		return response.EipAddresses.EipAddress, &response.PaginationResult, nil
	}

	return nil, nil, err
}

func (client *Client) DescribeEipAddressesWithRaw(args *DescribeEipAddressesArgs) (response *DescribeEipAddressesResponse, err error) {
	args.Validate()
	response = &DescribeEipAddressesResponse{}

	err = client.Invoke("DescribeEipAddresses", args, response)

	if err == nil {
		return response, nil
	}

	return nil, err
}

type ModifyEipAddressAttributeArgs struct {
	AllocationId string
	Bandwidth    int
}

type ModifyEipAddressAttributeResponse struct {
	common.Response
}

// ModifyEipAddressAttribute Modifies EIP attribute
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/network&modifyeipaddressattribute
func (client *Client) ModifyEipAddressAttribute(allocationId string, bandwidth int) error {
	args := ModifyEipAddressAttributeArgs{
		AllocationId: allocationId,
		Bandwidth:    bandwidth,
	}
	response := ModifyEipAddressAttributeResponse{}
	return client.Invoke("ModifyEipAddressAttribute", &args, &response)
}

type UnallocateEipAddressArgs struct {
	AllocationId string
	InstanceId   string
}

type UnallocateEipAddressResponse struct {
	common.Response
}

// UnassociateEipAddress unallocates Eip Address from instance
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/network&unassociateeipaddress
func (client *Client) UnassociateEipAddress(allocationId string, instanceId string) error {
	args := UnallocateEipAddressArgs{
		AllocationId: allocationId,
		InstanceId:   instanceId,
	}
	response := UnallocateEipAddressResponse{}
	return client.Invoke("UnassociateEipAddress", &args, &response)
}

type ReleaseEipAddressArgs struct {
	AllocationId string
}

type ReleaseEipAddressResponse struct {
	common.Response
}

// ReleaseEipAddress releases Eip address
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/network&releaseeipaddress
func (client *Client) ReleaseEipAddress(allocationId string) error {
	args := ReleaseEipAddressArgs{
		AllocationId: allocationId,
	}
	response := ReleaseEipAddressResponse{}
	return client.Invoke("ReleaseEipAddress", &args, &response)
}

// WaitForVSwitchAvailable waits for VSwitch to given status
func (client *Client) WaitForEip(regionId common.Region, allocationId string, status EipStatus, timeout int) error {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	args := DescribeEipAddressesArgs{
		RegionId:     regionId,
		AllocationId: allocationId,
	}
	for {
		eips, _, err := client.DescribeEipAddresses(&args)
		if err != nil {
			return err
		}
		if len(eips) == 0 {
			return common.GetClientErrorFromString("Not found")
		}
		if eips[0].Status == status {
			break
		}
		timeout = timeout - DefaultWaitForInterval
		if timeout <= 0 {
			return common.GetClientErrorFromString("Timeout")
		}
		time.Sleep(DefaultWaitForInterval * time.Second)
	}
	return nil
}
