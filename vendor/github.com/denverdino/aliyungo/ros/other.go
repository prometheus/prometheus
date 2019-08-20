package ros

import (
	"fmt"
	"net/http"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

//https://help.aliyun.com/document_detail/28925.html?spm=5176.doc28923.6.597.Wktzdg
type DescribeEventsRequest struct {
	ResourceStatus string
	ResourceName   string
	ResourceType   string
	PageNumber     int
	PageSize       int
}

type Event struct {
	ResourceStatus     string
	ResourceName       string
	StatusReason       string
	Id                 string
	ResourceId         string
	ResourceType       string
	ResourcePhysicalId string
	Time               string
}

type DescribeEventsResponse struct {
	common.Response
	TotalCount int
	PageNumber int
	PageSize   int
	Events     []Event
}

func (client *Client) DescribeEvents(stackId, stackName string, args *DescribeEventsRequest) (*DescribeEventsResponse, error) {
	response := &DescribeEventsResponse{}
	query := util.ConvertToQueryValues(args)
	err := client.Invoke("", http.MethodGet, fmt.Sprintf("/stacks/%s/%s/events", stackName, stackId), query, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/50086.html?spm=5176.doc28910.6.598.ngYYj6
type Region struct {
	LocalName string
	RegionId  string
}

type DescribeRegionsResponse struct {
	common.Response
	Regions []Region
}

func (client *Client) DescribeRegions() (*DescribeRegionsResponse, error) {
	response := &DescribeRegionsResponse{}
	err := client.Invoke("", http.MethodGet, "/regions", nil, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}
