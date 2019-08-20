package ros

import (
	"net/http"

	"fmt"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

//https://help.aliyun.com/document_detail/28910.html?spm=5176.doc50083.6.580.b5wkQr
type CreateStackRequest struct {
	Name            string
	Template        string
	Parameters      interface{}
	DisableRollback bool
	TimeoutMins     int
}

type CreateStackResponse struct {
	Id   string
	Name string
}

func (client *Client) CreateStack(regionId common.Region, args *CreateStackRequest) (*CreateStackResponse, error) {
	stack := &CreateStackResponse{}
	err := client.Invoke(regionId, http.MethodPost, "/stacks", nil, args, stack)
	if err != nil {
		return nil, err
	}

	return stack, nil
}

//https://help.aliyun.com/document_detail/28911.html?spm=5176.doc28910.6.581.etoi2Z
type DeleteStackRequest struct {
	RegionId common.Region
}

type DeleteStackResponse struct {
	common.Response
}

func (client *Client) DeleteStack(regionId common.Region, stackId string, stackName string) (*DeleteStackResponse, error) {
	args := &DeleteStackRequest{
		RegionId: regionId,
	}

	response := &DeleteStackResponse{}
	query := util.ConvertToQueryValues(args)
	err := client.Invoke(regionId, http.MethodDelete, fmt.Sprintf("/stacks/%s/%s", stackName, stackId), query, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/28912.html?spm=5176.doc28911.6.582.X0FKwG
type AbandonStackRequest struct {
	RegionId common.Region
}

type AbandonStackResponse struct {
	common.Response
	Id          string
	Name        string
	Action      string
	Status      string
	Template    interface{}
	Resources   interface{}
	Environment interface{}
}

func (client *Client) AbandonStack(regionId common.Region, stackId string, stackName string) (*AbandonStackResponse, error) {
	args := &DeleteStackRequest{
		RegionId: regionId,
	}

	response := &AbandonStackResponse{}
	query := util.ConvertToQueryValues(args)
	err := client.Invoke(regionId, http.MethodDelete, fmt.Sprintf("/stacks/%s/%s/abandon", stackName, stackId), query, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/28913.html?spm=5176.doc28912.6.583.vrfk38
type DescribeStacksRequest struct {
	RegionId   common.Region
	StackId    string
	Name       string
	Status     string
	PageNumber string
	PageSize   string
}

type DescribeStacksResponse struct {
	common.Response
	TotalCount int
	PageNumber int
	PageSize   int
	Stacks     []Stack
}

type Stack struct {
	Region          common.Region
	Id              string
	Name            string
	Updated         string
	Created         string
	StatusReason    string
	Status          string
	Description     string
	DisableRollback bool
	TimeoutMins     int
}

func (client *Client) DescribeStacks(args *DescribeStacksRequest) (*DescribeStacksResponse, error) {
	query := util.ConvertToQueryValues(args)
	stacks := &DescribeStacksResponse{}
	err := client.Invoke(args.RegionId, http.MethodGet, "/stacks", query, nil, stacks)
	if err != nil {
		return nil, err
	}

	return stacks, nil
}

//https://help.aliyun.com/document_detail/28914.html?spm=5176.doc28913.6.584.9JAYPI
type DescribeStackRequest struct {
	RegionId common.Region
}

type DescribeStackResponse struct {
	Parameters interface{}
	common.Response
	Id                  string
	Region              common.Region
	Name                string
	Updated             string
	Created             string
	TemplateDescription string
	StatusReason        string
	Status              string
	Outputs             interface{}
	DisableRollback     bool
	TimeoutMins         int
}

func (client *Client) DescribeStack(regionId common.Region, stackId string, stackName string) (*DescribeStackResponse, error) {
	args := &DescribeStackRequest{
		RegionId: regionId,
	}

	response := &DescribeStackResponse{}
	query := util.ConvertToQueryValues(args)
	err := client.Invoke(regionId, http.MethodGet, fmt.Sprintf("/stacks/%s/%s", stackName, stackId), query, nil, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

//https://help.aliyun.com/document_detail/50083.html?spm=5176.doc28914.6.585.QnbbaF
type PreviewStackRequest struct {
	Name            string
	Template        string
	Parameters      string
	DisableRollback bool
	TimeoutMins     int
}

type PreviewStackResponse struct {
	common.Response
	Id                  string
	Resources           interface{}
	Region              common.Region
	Description         string
	Updated             string
	Created             string
	Parameters          interface{}
	TemplateDescription string
	Webhook             string
	DisableRollback     bool
	TimeoutMins         int
}

func (client *Client) PreviewStack(regionId common.Region, args PreviewStackRequest) (*PreviewStackResponse, error) {
	query := util.ConvertToQueryValues(args)
	stack := &PreviewStackResponse{}
	err := client.Invoke(regionId, http.MethodPost, "/stacks/preview", query, args, stack)
	if err != nil {
		return nil, err
	}

	return stack, nil
}

//https://help.aliyun.com/document_detail/49066.html?spm=5176.doc28910.6.586.MJjWQh
type UpdateStackRequest struct {
	Template        string
	Parameters      interface{}
	DisableRollback bool
	TimeoutMins     int
}

type UpdateStackResponse struct {
	common.Response
	Id   string
	Name string
}

func (client *Client) UpdateStack(regionId common.Region, stackId string, stackName string, args *UpdateStackRequest) (*UpdateStackResponse, error) {
	stack := &UpdateStackResponse{}
	err := client.Invoke(regionId, http.MethodPut, fmt.Sprintf("/stacks/%s/%s", stackName, stackId), nil, args, stack)
	if err != nil {
		return nil, err
	}

	return stack, nil
}
