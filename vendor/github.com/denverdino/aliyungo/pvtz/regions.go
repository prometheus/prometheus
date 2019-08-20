package pvtz

import "github.com/denverdino/aliyungo/common"

type DescribeRegionsArgs struct {
}

//
type RegionType struct {
	RegionId   common.Region
	RegionName string
}

type DescribeRegionsResponse struct {
	common.Response
	Regions struct {
		Region []RegionType
	}
}

// DescribeRegions describes regions
//
// You can read doc at https://help.aliyun.com/document_detail/66246.html
func (client *Client) DescribeRegions() (regions []RegionType, err error) {
	response := DescribeRegionsResponse{}

	err = client.Invoke("DescribeRegions", &DescribeRegionsArgs{}, &response)

	if err != nil {
		return []RegionType{}, err
	}
	return response.Regions.Region, nil
}
