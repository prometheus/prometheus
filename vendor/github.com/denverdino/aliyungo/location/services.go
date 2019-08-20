package location

import (
	"github.com/denverdino/aliyungo/common"
)

type DescribeServicesArgs struct {
	RegionId common.Region
}

type DescribeServicesResponse struct {
	common.Response

	TotalCount int
	Services   struct {
		Services []string
	}
}

func (client *Client) DescribeServices(args *DescribeServicesArgs) (*DescribeServicesResponse, error) {
	response := &DescribeServicesResponse{}
	err := client.Invoke("DescribeServices", args, response)
	if err != nil {
		return nil, err
	}
	return response, err
}
