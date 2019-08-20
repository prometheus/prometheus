package ecs

import (
	"time"

	"github.com/denverdino/aliyungo/common"
)

type EcsCommonResponse struct {
	common.Response
}
type RouterType string
type InterfaceStatus string
type Role string
type Spec string

const (
	VRouter = RouterType("VRouter")
	VBR     = RouterType("VBR")

	Idl      = InterfaceStatus("Idl")
	Active   = InterfaceStatus("Active")
	Inactive = InterfaceStatus("Inactive")
	// 'Idle' means the router interface is not connected. 'Idl' may be a incorrect status.
	Idle = InterfaceStatus("Idle")

	InitiatingSide = Role("InitiatingSide")
	AcceptingSide  = Role("AcceptingSide")

	Small1  = Spec("Small.1")
	Small2  = Spec("Small.2")
	Small5  = Spec("Small.5")
	Middle1 = Spec("Middle.1")
	Middle2 = Spec("Middle.2")
	Middle5 = Spec("Middle.5")
	Large1  = Spec("Large.1")
	Large2  = Spec("Large.2")
)

type CreateRouterInterfaceArgs struct {
	RegionId                 common.Region
	OppositeRegionId         common.Region
	RouterType               RouterType
	OppositeRouterType       RouterType
	RouterId                 string
	OppositeRouterId         string
	Role                     Role
	Spec                     Spec
	AccessPointId            string
	OppositeAccessPointId    string
	OppositeInterfaceId      string
	OppositeInterfaceOwnerId string
	Name                     string
	Description              string
	HealthCheckSourceIp      string
	HealthCheckTargetIp      string
}

type CreateRouterInterfaceResponse struct {
	common.Response
	RouterInterfaceId string
}

// CreateRouterInterface create Router interface
//
// You can read doc at https://help.aliyun.com/document_detail/36032.html?spm=5176.product27706.6.664.EbBsxC
func (client *Client) CreateRouterInterface(args *CreateRouterInterfaceArgs) (response *CreateRouterInterfaceResponse, err error) {
	response = &CreateRouterInterfaceResponse{}
	err = client.Invoke("CreateRouterInterface", args, &response)
	if err != nil {
		return response, err
	}
	return response, nil
}

type Filter struct {
	Key   string
	Value []string
}

type DescribeRouterInterfacesArgs struct {
	RegionId common.Region
	common.Pagination
	Filter []Filter
}

type RouterInterfaceItemType struct {
	ChargeType                      string
	RouterInterfaceId               string
	AccessPointId                   string
	OppositeRegionId                string
	OppositeAccessPointId           string
	Role                            Role
	Spec                            Spec
	Name                            string
	Description                     string
	RouterId                        string
	RouterType                      RouterType
	CreationTime                    string
	Status                          string
	BusinessStatus                  string
	ConnectedTime                   string
	OppositeInterfaceId             string
	OppositeInterfaceSpec           string
	OppositeInterfaceStatus         string
	OppositeInterfaceBusinessStatus string
	OppositeRouterId                string
	OppositeRouterType              RouterType
	OppositeInterfaceOwnerId        string
	HealthCheckSourceIp             string
	HealthCheckTargetIp             string
}

type DescribeRouterInterfacesResponse struct {
	RouterInterfaceSet struct {
		RouterInterfaceType []RouterInterfaceItemType
	}
	common.PaginationResult
}

// DescribeRouterInterfaces describe Router interfaces
//
// You can read doc at https://help.aliyun.com/document_detail/36032.html?spm=5176.product27706.6.664.EbBsxC
func (client *Client) DescribeRouterInterfaces(args *DescribeRouterInterfacesArgs) (response *DescribeRouterInterfacesResponse, err error) {
	response = &DescribeRouterInterfacesResponse{}
	err = client.Invoke("DescribeRouterInterfaces", args, &response)
	if err != nil {
		return response, err
	}
	return response, nil
}

type OperateRouterInterfaceArgs struct {
	RegionId          common.Region
	RouterInterfaceId string
}

// ConnectRouterInterface
//
// You can read doc at https://help.aliyun.com/document_detail/36031.html?spm=5176.doc36035.6.666.wkyljN
func (client *Client) ConnectRouterInterface(args *OperateRouterInterfaceArgs) (response *EcsCommonResponse, err error) {
	response = &EcsCommonResponse{}
	err = client.Invoke("ConnectRouterInterface", args, &response)
	if err != nil {
		return response, err
	}
	return response, nil
}

// ActivateRouterInterface active Router Interface
//
// You can read doc at https://help.aliyun.com/document_detail/36030.html?spm=5176.doc36031.6.667.DAuZLD
func (client *Client) ActivateRouterInterface(args *OperateRouterInterfaceArgs) (response *EcsCommonResponse, err error) {
	response = &EcsCommonResponse{}
	err = client.Invoke("ActivateRouterInterface", args, &response)
	if err != nil {
		return response, err
	}
	return response, nil
}

// DeactivateRouterInterface deactivate Router Interface
//
// You can read doc at https://help.aliyun.com/document_detail/36033.html?spm=5176.doc36030.6.668.JqCWUz
func (client *Client) DeactivateRouterInterface(args *OperateRouterInterfaceArgs) (response *EcsCommonResponse, err error) {
	response = &EcsCommonResponse{}
	err = client.Invoke("DeactivateRouterInterface", args, &response)
	if err != nil {
		return response, err
	}
	return response, nil
}

type ModifyRouterInterfaceSpecArgs struct {
	RegionId          common.Region
	RouterInterfaceId string
	Spec              Spec
}

type ModifyRouterInterfaceSpecResponse struct {
	common.Response
	Spec Spec
}

// ModifyRouterInterfaceSpec
//
// You can read doc at https://help.aliyun.com/document_detail/36037.html?spm=5176.doc36036.6.669.McKiye
func (client *Client) ModifyRouterInterfaceSpec(args *ModifyRouterInterfaceSpecArgs) (response *ModifyRouterInterfaceSpecResponse, err error) {
	response = &ModifyRouterInterfaceSpecResponse{}
	err = client.Invoke("ModifyRouterInterfaceSpec", args, &response)
	if err != nil {
		return response, err
	}
	return response, nil
}

type ModifyRouterInterfaceAttributeArgs struct {
	RegionId                 common.Region
	RouterInterfaceId        string
	Name                     string
	Description              string
	OppositeInterfaceId      string
	OppositeRouterId         string
	OppositeInterfaceOwnerId string
	HealthCheckSourceIp      string
	HealthCheckTargetIp      string
}

// ModifyRouterInterfaceAttribute
//
// You can read doc at https://help.aliyun.com/document_detail/36036.html?spm=5176.doc36037.6.670.Dcz3xS
func (client *Client) ModifyRouterInterfaceAttribute(args *ModifyRouterInterfaceAttributeArgs) (response *EcsCommonResponse, err error) {
	response = &EcsCommonResponse{}
	err = client.Invoke("ModifyRouterInterfaceAttribute", args, &response)
	if err != nil {
		return response, err
	}
	return response, nil
}

// DeleteRouterInterface delete Router Interface
//
// You can read doc at https://help.aliyun.com/document_detail/36034.html?spm=5176.doc36036.6.671.y2xpNt
func (client *Client) DeleteRouterInterface(args *OperateRouterInterfaceArgs) (response *EcsCommonResponse, err error) {
	response = &EcsCommonResponse{}
	err = client.Invoke("DeleteRouterInterface", args, &response)
	if err != nil {
		return response, err
	}
	return response, nil
}

// WaitForRouterInterface waits for router interface to given status
func (client *Client) WaitForRouterInterfaceAsyn(regionId common.Region, interfaceId string, status InterfaceStatus, timeout int) error {
	if timeout <= 0 {
		timeout = InstanceDefaultTimeout
	}
	for {
		interfaces, err := client.DescribeRouterInterfaces(&DescribeRouterInterfacesArgs{
			RegionId: regionId,
			Filter:   []Filter{Filter{Key: "RouterInterfaceId", Value: []string{interfaceId}}},
		})
		if err != nil {
			return err
		} else if interfaces != nil && InterfaceStatus(interfaces.RouterInterfaceSet.RouterInterfaceType[0].Status) == status {
			//TODO
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
