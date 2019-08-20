package ros

import (
	"fmt"
	"net/http"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

//https://help.aliyun.com/document_detail/28916.html?spm=5176.doc49066.6.588.d7Ntjs
type Resource struct {
	Id           string
	Name         string
	Type         string
	Status       string
	StatusReason string
	Updated      string
	PhysicalId   string
}

func (client *Client) DescribeResources(stackId, stackName string) ([]*Resource, error) {
	response := make([]*Resource, 0)
	err := client.Invoke("", http.MethodGet, fmt.Sprintf("/stacks/%s/%s/resources", stackName, stackId), nil, nil, &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/28917.html?spm=5176.doc28916.6.589.BUPJqx
func (client *Client) DescribeResource(stackId, stackName, resourceName string) (*Resource, error) {
	response := &Resource{}
	err := client.Invoke("", http.MethodGet, fmt.Sprintf("/stacks/%s/%s/resources/%s", stackName, stackId, resourceName), nil, nil, &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/28918.html?spm=5176.doc28917.6.590.smknll
type SupportStatus string

const (
	SUPPORT_STATUS_UNKNOWN     = "UNKNOWN"
	SUPPORT_STATUS_SUPPORTED   = "SUPPORTED"
	SUPPORT_STATUS_DEPRECATED  = "DEPRECATED"
	SUPPORT_STATUS_UNSUPPORTED = "UNSUPPORTED"
	SUPPORT_STATUS_HIDDEN      = "HIDDEN"
)

type DescribeResoureTypesRequest struct {
	SupportStatus SupportStatus
}

type DescribeResoureTypesResponse struct {
	common.Response
	ResourceTypes []string
}

func (client *Client) DescribeResoureTypes(supportStatus SupportStatus) (*DescribeResoureTypesResponse, error) {
	query := util.ConvertToQueryValues(&DescribeResoureTypesRequest{
		SupportStatus: supportStatus,
	})

	response := &DescribeResoureTypesResponse{}
	err := client.Invoke("", http.MethodGet, "/resource_types", query, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/28919.html?spm=5176.doc28918.6.591.7QkDYC
type DescribeResoureTypeResponse struct {
	common.Response
	ResourceType  string
	Attributes    interface{}
	SupportStatus interface{}
	Properties    interface{}
}

func (client *Client) DescribeResoureType(typeName string) (*DescribeResoureTypeResponse, error) {
	response := &DescribeResoureTypeResponse{}
	err := client.Invoke("", http.MethodGet, fmt.Sprintf("/resource_types/%s", typeName), nil, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/28920.html?spm=5176.doc28919.6.592.IiEwar
type DescribeResoureTypeTemplateResponse struct {
	common.Response
	ROSTemplateFormatVersion string
	Parameters               interface{}
	Outputs                  interface{}
	Resources                interface{}
}

func (client *Client) DescribeResoureTypeTemplate(typeName string) (*DescribeResoureTypeTemplateResponse, error) {
	response := &DescribeResoureTypeTemplateResponse{}
	err := client.Invoke("", http.MethodGet, fmt.Sprintf("/resource_types/%s/template", typeName), nil, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}
