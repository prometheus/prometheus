package ecs

import (
	"github.com/denverdino/aliyungo/common"
)

type ResourceType string

const (
	ResourceTypeInstance            = ResourceType("Instance")
	ResourceTypeDisk                = ResourceType("Disk")
	ResourceTypeVSwitch             = ResourceType("VSwitch")
	ResourceTypeIOOptimizedInstance = ResourceType("IoOptimized")
)

// The sub-item of the type AvailableResourcesType
type SupportedResourceType string

const (
	SupportedInstanceType       = SupportedResourceType("supportedInstanceType")
	SupportedInstanceTypeFamily = SupportedResourceType("supportedInstanceTypeFamily")
	SupportedInstanceGeneration = SupportedResourceType("supportedInstanceGeneration")
	SupportedSystemDiskCategory = SupportedResourceType("supportedSystemDiskCategory")
	SupportedDataDiskCategory   = SupportedResourceType("supportedDataDiskCategory")
	SupportedNetworkCategory    = SupportedResourceType("supportedNetworkCategory")
)

//
// You can read doc at https://help.aliyun.com/document_detail/25670.html?spm=5176.doc25640.2.1.J24zQt
type ResourcesInfoType struct {
	ResourcesInfo []AvailableResourcesType
}

// Because the sub-item of AvailableResourcesType starts with supported and golang struct cann't refer them, this uses map to parse ResourcesInfo
type AvailableResourcesType struct {
	IoOptimized          bool
	NetworkTypes         map[SupportedResourceType][]string
	InstanceGenerations  map[SupportedResourceType][]string
	InstanceTypeFamilies map[SupportedResourceType][]string
	InstanceTypes        map[SupportedResourceType][]string
	SystemDiskCategories map[SupportedResourceType][]DiskCategory
	DataDiskCategories   map[SupportedResourceType][]DiskCategory
}

type DescribeZonesArgs struct {
	RegionId common.Region
}

//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/datatype&availableresourcecreationtype
type AvailableResourceCreationType struct {
	ResourceTypes []ResourceType //enum for Instance, Disk, VSwitch
}

//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/datatype&availablediskcategoriestype
type AvailableDiskCategoriesType struct {
	DiskCategories []DiskCategory //enum for cloud, ephemeral, ephemeral_ssd
}

type AvailableInstanceTypesType struct {
	InstanceTypes []string
}

//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/datatype&zonetype
type ZoneType struct {
	ZoneId                    string
	LocalName                 string
	AvailableResources        ResourcesInfoType
	AvailableInstanceTypes    AvailableInstanceTypesType
	AvailableResourceCreation AvailableResourceCreationType
	AvailableDiskCategories   AvailableDiskCategoriesType
}

type DescribeZonesResponse struct {
	common.Response
	Zones struct {
		Zone []ZoneType
	}
}

// DescribeZones describes zones
func (client *Client) DescribeZones(regionId common.Region) (zones []ZoneType, err error) {
	response, err := client.DescribeZonesWithRaw(regionId)
	if err == nil {
		return response.Zones.Zone, nil
	}

	return []ZoneType{}, err
}

func (client *Client) DescribeZonesWithRaw(regionId common.Region) (response *DescribeZonesResponse, err error) {
	args := DescribeZonesArgs{
		RegionId: regionId,
	}
	response = &DescribeZonesResponse{}

	err = client.Invoke("DescribeZones", &args, response)

	if err == nil {
		return response, nil
	}

	return nil, err
}

type DescribeAvailableResourceArgs struct {
	RegionId            string
	DestinationResource string
	ZoneId              string
	InstanceChargeType  string
	SpotStrategy        string
	IoOptimized         string
	InstanceType        string
	SystemDiskCategory  string
	DataDiskCategory    string
	NetworkCategory     string
}

type DescribeAvailableResourceResponse struct {
	common.Response
	AvailableZones struct {
		AvailableZone []AvailableZoneType
	}
}

type AvailableZoneType struct {
	RegionId           string
	ZoneId             string
	Status             string
	AvailableResources struct {
		AvailableResource []NewAvailableResourcesType
	}
}

type NewAvailableResourcesType struct {
	Type               string
	SupportedResources struct {
		SupportedResource []SupportedResourcesType
	}
}

type SupportedResourcesType struct {
	Value  string
	Status string
	Min    string
	Max    string
	Unit   string
}

// https://www.alibabacloud.com/help/doc-detail/66186.htm
func (client *Client) DescribeAvailableResource(args *DescribeAvailableResourceArgs) (response *DescribeAvailableResourceResponse, err error) {

	response = &DescribeAvailableResourceResponse{}
	err = client.Invoke("DescribeAvailableResource", args, response)
	return response, err
}
