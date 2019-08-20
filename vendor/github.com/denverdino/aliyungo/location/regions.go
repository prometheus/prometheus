package location

import (
	"github.com/denverdino/aliyungo/common"
)

type DescribeRegionsArgs struct {
	Password string
}

type DescribeRegionsResponse struct {
	common.Response

	TotalCount int
	RegionIds  struct {
		RegionIds []string
	}
}

func (client *Client) DescribeRegions(args *DescribeRegionsArgs) (*DescribeRegionsResponse, error) {
	response := &DescribeRegionsResponse{}
	err := client.Invoke("DescribeRegions", args, response)
	if err != nil {
		return nil, err
	}
	return response, err
}
