package kms

import "github.com/denverdino/aliyungo/common"

type DescribeRegionsArgs struct {
}

type DescribeRegionsResponse struct {
	common.Response
	Regions Regions
}

type Regions struct {
	Region []Region
}

type Region struct {
	RegionId string
}

func (client *Client) DescribeRegions() (*DescribeRegionsResponse, error) {
	response := &DescribeRegionsResponse{}
	err := client.Invoke("DescribeRegions", &DescribeRegionsArgs{}, response)
	return response, err
}
