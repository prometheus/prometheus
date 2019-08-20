package pvtz

import (
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

type DescribeZonesArgs struct {
	Keyword      string
	Lang         string
	UserClientIp string
	common.Pagination
}

//
type ZoneType struct {
	ZoneName    string
	ZoneId      string
	IsPtr       bool
	RecordCount int
	CreateTime  util.ISO6801Time
	UpdateTime  util.ISO6801Time
}

type DescribeZonesResponse struct {
	common.Response
	common.PaginationResult
	Zones struct {
		Zone []ZoneType
	}
}

// DescribeZones describes zones
//
// You can read doc at https://help.aliyun.com/document_detail/66243.html
func (client *Client) DescribeZones(args *DescribeZonesArgs) (zones []ZoneType, err error) {

	result := []ZoneType{}

	for {
		response := DescribeZonesResponse{}
		err = client.Invoke("DescribeZones", args, &response)

		if err != nil {
			return result, err
		}

		result = append(result, response.Zones.Zone...)

		nextPage := response.PaginationResult.NextPage()
		if nextPage == nil {
			break
		}
		args.Pagination = *nextPage
	}

	return result, nil
}

type AddZoneArgs struct {
	ZoneName     string
	Lang         string
	UserClientIp string
}

type AddZoneResponse struct {
	common.Response
	Success  bool
	ZoneId   string
	ZoneName string
}

// AddZone add zone
//
// You can read doc at https://help.aliyun.com/document_detail/66240.html
func (client *Client) AddZone(args *AddZoneArgs) (response *AddZoneResponse, err error) {
	response = &AddZoneResponse{}

	err = client.Invoke("AddZone", args, &response)

	return response, err
}

type DeleteZoneArgs struct {
	ZoneId       string
	Lang         string
	UserClientIp string
}

type DeleteZoneResponse struct {
	common.Response
	ZoneId string
}

// DeleteZone delete zone
//
// You can read doc at https://help.aliyun.com/document_detail/66240.html
func (client *Client) DeleteZone(args *DeleteZoneArgs) (err error) {
	response := &DeleteZoneResponse{}
	err = client.Invoke("DeleteZone", args, &response)

	return err
}

type CheckZoneNameArgs struct {
	ZoneName     string
	Lang         string
	UserClientIp string
}

type CheckZoneNameResponse struct {
	common.Response
	Success bool
	Check   bool
}

// CheckZoneName check zone name available or not
//
// You can read doc at https://help.aliyun.com/document_detail/66240.html
func (client *Client) CheckZoneName(args *CheckZoneNameArgs) (bool, error) {
	response := &CheckZoneNameResponse{}
	err := client.Invoke("CheckZoneName", args, &response)
	if err != nil {
		return false, err
	}

	return response.Check, err
}

type UpdateZoneRemarkArgs struct {
	ZoneId       string
	Lang         string
	UserClientIp string
	Remark       string
}

type UpdateZoneRemarkResponse struct {
	common.Response
	ZoneId string
}

// CheckZoneName check zone name available or not
//
// You can read doc at https://help.aliyun.com/document_detail/66242.html
func (client *Client) UpdateZoneRemark(args *UpdateZoneRemarkArgs) error {
	response := &UpdateZoneRemarkResponse{}
	err := client.Invoke("UpdateZoneRemark", args, &response)
	return err
}

type DescribeZoneInfoArgs struct {
	ZoneId       string
	Lang         string
	UserClientIp string
	common.Pagination
}

type VPCType struct {
	RegionId common.Region
	VpcId    string
	VpcName  string
}

type DescribeZoneInfoResponse struct {
	common.Response
	ZoneName        string
	ZoneId          string
	Remark          string
	RecordCount     int
	RegionName      string
	IsPtr           bool
	CreateTime      util.ISO6801Time
	CreateTimestamp int64
	UpdateTime      util.ISO6801Time
	UpdateTimestamp int64
	BindVpcs        struct {
		VPC []VPCType
	}
}

// DescribeZoneInfo describes zone info
//
// You can read doc at https://help.aliyun.com/document_detail/66244.html
func (client *Client) DescribeZoneInfo(args *DescribeZoneInfoArgs) (response *DescribeZoneInfoResponse, err error) {

	response = &DescribeZoneInfoResponse{}
	err = client.Invoke("DescribeZoneInfo", args, response)
	return response, err
}

type BindZoneVpcArgs struct {
	ZoneId       string
	Lang         string
	UserClientIp string
	Vpcs         []VPCType
}

type BindZoneVpcResponse struct {
	common.Response
}

// BindZoneVpc bind zone to VPC
//
// You can read doc at https://help.aliyun.com/document_detail/66244.html
func (client *Client) BindZoneVpc(args *BindZoneVpcArgs) (err error) {
	response := &BindZoneVpcResponse{}
	err = client.Invoke("BindZoneVpc", args, response)
	return err
}
