package ecs

import (
	"github.com/denverdino/aliyungo/common"
)

type BandwidthPackageType struct {
	IpCount   int
	Bandwidth int
	Zone      string
}

type CreateNatGatewayArgs struct {
	RegionId         common.Region
	VpcId            string
	Spec             string
	BandwidthPackage []BandwidthPackageType
	Name             string
	Description      string
	ClientToken      string
}

type ForwardTableIdType struct {
	ForwardTableId []string
}

type SnatTableIdType struct {
	SnatTableId []string
}

type IpListsType struct {
	IpList []IpListItem
}

type IpListItem struct {
	IpAddress    string
	AllocationId string
	UsingStatus  string
}

type BandwidthPackageIdType struct {
	BandwidthPackageId []string
}

type CreateNatGatewayResponse struct {
	common.Response
	NatGatewayId        string
	ForwardTableIds     ForwardTableIdType
	BandwidthPackageIds BandwidthPackageIdType
}

// CreateNatGateway creates Virtual Private Cloud
//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/vpc&createvpc
func (client *Client) CreateNatGateway(args *CreateNatGatewayArgs) (resp *CreateNatGatewayResponse, err error) {
	response := CreateNatGatewayResponse{}
	err = client.Invoke("CreateNatGateway", args, &response)
	if err != nil {
		return nil, err
	}
	return &response, err
}

type NatGatewaySetType struct {
	BusinessStatus      string
	Description         string
	BandwidthPackageIds BandwidthPackageIdType
	ForwardTableIds     ForwardTableIdType
	SnatTableIds        SnatTableIdType
	IpLists             IpListsType
	InstanceChargeType  string
	Name                string
	NatGatewayId        string
	RegionId            common.Region
	Spec                string
	Status              string
	VpcId               string
}

type DescribeNatGatewayResponse struct {
	common.Response
	common.PaginationResult
	NatGateways struct {
		NatGateway []NatGatewaySetType
	}
}

type DescribeNatGatewaysArgs struct {
	RegionId     common.Region
	NatGatewayId string
	VpcId        string
	common.Pagination
}

func (client *Client) DescribeNatGateways(args *DescribeNatGatewaysArgs) (natGateways []NatGatewaySetType,
	pagination *common.PaginationResult, err error) {
	response, err := client.DescribeNatGatewaysWithRaw(args)
	if err == nil {
		return response.NatGateways.NatGateway, &response.PaginationResult, nil
	}

	return nil, nil, err
}

func (client *Client) DescribeNatGatewaysWithRaw(args *DescribeNatGatewaysArgs) (response *DescribeNatGatewayResponse, err error) {
	args.Validate()
	response = &DescribeNatGatewayResponse{}

	err = client.Invoke("DescribeNatGateways", args, response)

	if err == nil {
		return response, nil
	}

	return nil, err
}

type ModifyNatGatewayAttributeArgs struct {
	RegionId     common.Region
	NatGatewayId string
	Name         string
	Description  string
}

type ModifyNatGatewayAttributeResponse struct {
	common.Response
}

func (client *Client) ModifyNatGatewayAttribute(args *ModifyNatGatewayAttributeArgs) error {
	response := ModifyNatGatewayAttributeResponse{}
	return client.Invoke("ModifyNatGatewayAttribute", args, &response)
}

type ModifyNatGatewaySpecArgs struct {
	RegionId     common.Region
	NatGatewayId string
	Spec         NatGatewaySpec
}

func (client *Client) ModifyNatGatewaySpec(args *ModifyNatGatewaySpecArgs) error {
	response := ModifyNatGatewayAttributeResponse{}
	return client.Invoke("ModifyNatGatewaySpec", args, &response)
}

type DeleteNatGatewayArgs struct {
	RegionId     common.Region
	NatGatewayId string
}

type DeleteNatGatewayResponse struct {
	common.Response
}

func (client *Client) DeleteNatGateway(args *DeleteNatGatewayArgs) error {
	response := DeleteNatGatewayResponse{}
	err := client.Invoke("DeleteNatGateway", args, &response)
	return err
}

type DescribeBandwidthPackagesArgs struct {
	RegionId common.Region
	common.Pagination
	BandwidthPackageId string
	NatGatewayId       string
}

type PublicIpAddresseType struct {
	AllocationId string
	IpAddress    string
}

type DescribeBandwidthPackageType struct {
	Bandwidth          string
	BandwidthPackageId string
	IpCount            string
	PublicIpAddresses  struct {
		PublicIpAddresse []PublicIpAddresseType
	}

	ZoneId string
}

type DescribeBandwidthPackagesResponse struct {
	common.Response
	common.PaginationResult
	BandwidthPackages struct {
		BandwidthPackage []DescribeBandwidthPackageType
	}
}

func (client *Client) DescribeBandwidthPackages(args *DescribeBandwidthPackagesArgs) (*DescribeBandwidthPackagesResponse, error) {
	response := &DescribeBandwidthPackagesResponse{}

	err := client.Invoke("DescribeBandwidthPackages", args, response)
	if err != nil {
		return nil, err
	}

	return response, err
}

type DeleteBandwidthPackageArgs struct {
	RegionId           common.Region
	BandwidthPackageId string
}

type DeleteBandwidthPackageResponse struct {
	common.Response
}

func (client *Client) DeleteBandwidthPackage(args *DeleteBandwidthPackageArgs) error {
	response := DeleteBandwidthPackageResponse{}
	err := client.Invoke("DeleteBandwidthPackage", args, &response)
	return err
}

type NatGatewaySpec string

const (
	NatGatewaySmallSpec  = NatGatewaySpec("Small")
	NatGatewayMiddleSpec = NatGatewaySpec("Middle")
	NatGatewayLargeSpec  = NatGatewaySpec("Large")
)
