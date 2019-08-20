package location

import (
	"github.com/denverdino/aliyungo/common"
)

type DescribeEndpointArgs struct {
	Id          common.Region
	ServiceCode string
	Type        string
}

type EndpointItem struct {
	Protocols struct {
		Protocols []string
	}
	Type        string
	Namespace   string
	Id          common.Region
	SerivceCode string
	Endpoint    string
}

type DescribeEndpointResponse struct {
	common.Response
	EndpointItem
}

func (client *Client) DescribeEndpoint(args *DescribeEndpointArgs) (*DescribeEndpointResponse, error) {
	response := &DescribeEndpointResponse{}
	err := client.Invoke("DescribeEndpoint", args, response)
	if err != nil {
		return nil, err
	}
	return response, err
}

type DescribeEndpointsArgs struct {
	Id          common.Region
	ServiceCode string
	Type        string
}

type DescribeEndpointsResponse struct {
	common.Response
	Endpoints struct {
		Endpoint []EndpointItem
	}
}

func (client *Client) DescribeEndpoints(args *DescribeEndpointsArgs) (*DescribeEndpointsResponse, error) {
	response := &DescribeEndpointsResponse{}
	err := client.Invoke("DescribeEndpoints", args, response)
	if err != nil {
		return nil, err
	}
	return response, err
}
