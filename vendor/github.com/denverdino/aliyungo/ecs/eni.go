package ecs

import (
	"fmt"
	"time"

	"github.com/denverdino/aliyungo/common"
)

type CreateNetworkInterfaceArgs struct {
	RegionId             common.Region
	VSwitchId            string
	PrimaryIpAddress     string // optional
	SecurityGroupId      string
	NetworkInterfaceName string // optional
	Description          string // optional
	ClientToken          string // optional
}

type CreateNetworkInterfaceResponse struct {
	common.Response
	NetworkInterfaceId string
}
type DeleteNetworkInterfaceArgs struct {
	RegionId           common.Region
	NetworkInterfaceId string
}

type DeleteNetworkInterfaceResponse struct {
	common.Response
}

type DescribeNetworkInterfacesArgs struct {
	RegionId             common.Region
	VSwitchId            string
	PrimaryIpAddress     string
	SecurityGroupId      string
	NetworkInterfaceName string
	Type                 string
	InstanceId           string
	NetworkInterfaceId   []string `query:"list"`
	PageNumber           int
	PageSize             int
}
type NetworkInterfaceType struct {
	NetworkInterfaceId   string
	NetworkInterfaceName string
	PrimaryIpAddress     string
	MacAddress           string
	Status               string
	PrivateIpAddress     string
}

type DescribeNetworkInterfacesResponse struct {
	common.Response
	NetworkInterfaceSets struct {
		NetworkInterfaceSet []NetworkInterfaceType
	}
	TotalCount int
	PageNumber int
	PageSize   int
}
type AttachNetworkInterfaceArgs struct {
	RegionId           common.Region
	NetworkInterfaceId string
	InstanceId         string
}

type AttachNetworkInterfaceResponse common.Response

type DetachNetworkInterfaceArgs AttachNetworkInterfaceArgs

type DetachNetworkInterfaceResponse common.Response

type ModifyNetworkInterfaceAttributeArgs struct {
	RegionId             common.Region
	NetworkInterfaceId   string
	SecurityGroupId      []string
	NetworkInterfaceName string
	Description          string
}
type ModifyNetworkInterfaceAttributeResponse common.Response

func (client *Client) CreateNetworkInterface(args *CreateNetworkInterfaceArgs) (resp *CreateNetworkInterfaceResponse, err error) {
	resp = &CreateNetworkInterfaceResponse{}
	err = client.Invoke("CreateNetworkInterface", args, resp)
	return resp, err
}

func (client *Client) DeleteNetworkInterface(args *DeleteNetworkInterfaceArgs) (resp *DeleteNetworkInterfaceResponse, err error) {
	resp = &DeleteNetworkInterfaceResponse{}
	err = client.Invoke("DeleteNetworkInterface", args, resp)
	return resp, err
}

func (client *Client) DescribeNetworkInterfaces(args *DescribeNetworkInterfacesArgs) (resp *DescribeNetworkInterfacesResponse, err error) {
	resp = &DescribeNetworkInterfacesResponse{}
	err = client.Invoke("DescribeNetworkInterfaces", args, resp)
	return resp, err
}

func (client *Client) AttachNetworkInterface(args *AttachNetworkInterfaceArgs) error {
	resp := &AttachNetworkInterfaceResponse{}
	err := client.Invoke("AttachNetworkInterface", args, resp)
	return err
}

func (client *Client) DetachNetworkInterface(args *DetachNetworkInterfaceArgs) (resp *DetachNetworkInterfaceResponse, err error) {
	resp = &DetachNetworkInterfaceResponse{}
	err = client.Invoke("DetachNetworkInterface", args, resp)
	return resp, err
}

func (client *Client) ModifyNetworkInterfaceAttribute(args *ModifyNetworkInterfaceAttributeArgs) (resp *ModifyNetworkInterfaceAttributeResponse, err error) {
	resp = &ModifyNetworkInterfaceAttributeResponse{}
	err = client.Invoke("ModifyNetworkInterfaceAttribute", args, resp)
	return resp, err
}

// Default timeout value for WaitForInstance method
const NetworkInterfacesDefaultTimeout = 120

// WaitForInstance waits for instance to given status
func (client *Client) WaitForNetworkInterface(regionId common.Region, eniID string, status string, timeout int) error {
	if timeout <= 0 {
		timeout = NetworkInterfacesDefaultTimeout
	}
	for {

		eniIds := []string{eniID}

		describeNetworkInterfacesArgs := DescribeNetworkInterfacesArgs{
			RegionId:           regionId,
			NetworkInterfaceId: eniIds,
		}

		nisResponse, err := client.DescribeNetworkInterfaces(&describeNetworkInterfacesArgs)
		if err != nil {
			return fmt.Errorf("Failed to describe network interface %v: %v", eniID, err)
		}

		if len(nisResponse.NetworkInterfaceSets.NetworkInterfaceSet) > 0 && nisResponse.NetworkInterfaceSets.NetworkInterfaceSet[0].Status == status {
			break
		}

		timeout = timeout - DefaultWaitForInterval
		if timeout <= 0 {
			return fmt.Errorf("Timeout for waiting available status for network interfaces")
		}
		time.Sleep(DefaultWaitForInterval * time.Second)

	}
	return nil
}
