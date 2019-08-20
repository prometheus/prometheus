package slb

import (
	"github.com/denverdino/aliyungo/common"
)

type DescribeZonesArgs struct {
	RegionId common.Region
}

//
// You can read doc at http://docs.aliyun.com/#/pub/ecs/open-api/datatype&zonetype
type ZoneType struct {
	ZoneId     string
	LocalName  string
	SlaveZones struct {
		SlaveZone []ZoneType
	}
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
