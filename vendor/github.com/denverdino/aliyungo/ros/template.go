package ros

import (
	"fmt"
	"net/http"

	"github.com/denverdino/aliyungo/common"
)

//https://help.aliyun.com/document_detail/28922.html?spm=5176.doc28920.6.594.UI5p6A
type DescribeTemplateResponse struct {
	common.Response
	ROSTemplateFormatVersion string
	Parameters               interface{}
	Outputs                  interface{}
	Resources                interface{}
	Description              interface{}
	Conditions               interface{}
	Mappings                 interface{}
	Metadata                 interface{}
}

func (client *Client) DescribeTemplate(stackId, stackName string) (*DescribeTemplateResponse, error) {
	response := &DescribeTemplateResponse{}
	err := client.Invoke("", http.MethodGet, fmt.Sprintf("/stacks/%s/%s/template", stackName, stackId), nil, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/28923.html?spm=5176.doc28922.6.595.uhWWET
type ValidateTemplateRequest struct {
	Template string
}

type ValidateTemplateResponse struct {
	common.Response
	Parameters  interface{}
	Description interface{}
}

func (client *Client) ValidateTemplate(args *ValidateTemplateRequest) (*ValidateTemplateResponse, error) {
	response := &ValidateTemplateResponse{}
	err := client.Invoke("", http.MethodPost, "/validate", nil, args, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}
