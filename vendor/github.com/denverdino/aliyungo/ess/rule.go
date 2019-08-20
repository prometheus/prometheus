package ess

import "github.com/denverdino/aliyungo/common"

type AdjustmentType string

const (
	QuantityChangeInCapacity = AdjustmentType("QuantityChangeInCapacity")
	PercentChangeInCapacity  = AdjustmentType("PercentChangeInCapacity")
	TotalCapacity            = AdjustmentType("TotalCapacity")
)

type CreateScalingRuleArgs struct {
	RegionId        common.Region
	ScalingGroupId  string
	AdjustmentType  AdjustmentType
	AdjustmentValue int
	Cooldown        int
	ScalingRuleName string
}

type CreateScalingRuleResponse struct {
	common.Response
	ScalingRuleId  string
	ScalingRuleAri string
}

// CreateScalingRule create scaling rule
//
// You can read doc at https://help.aliyun.com/document_detail/25948.html?spm=5176.doc25944.6.629.FLkNnj
func (client *Client) CreateScalingRule(args *CreateScalingRuleArgs) (resp *CreateScalingRuleResponse, err error) {
	response := CreateScalingRuleResponse{}
	err = client.Invoke("CreateScalingRule", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type ModifyScalingRuleArgs struct {
	RegionId        common.Region
	ScalingRuleId   string
	AdjustmentType  AdjustmentType
	AdjustmentValue int
	Cooldown        int
	ScalingRuleName string
}

type ModifyScalingRuleResponse struct {
	common.Response
}

// ModifyScalingRule modify scaling rule
//
// You can read doc at https://help.aliyun.com/document_detail/25949.html?spm=5176.doc25948.6.630.HGN1va
func (client *Client) ModifyScalingRule(args *ModifyScalingRuleArgs) (resp *ModifyScalingRuleResponse, err error) {
	response := ModifyScalingRuleResponse{}
	err = client.Invoke("ModifyScalingRule", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type DescribeScalingRulesArgs struct {
	common.Pagination
	RegionId        common.Region
	ScalingGroupId  string
	ScalingRuleId   common.FlattenArray
	ScalingRuleName common.FlattenArray
	ScalingRuleAri  common.FlattenArray
}

type DescribeScalingRulesResponse struct {
	common.Response
	common.PaginationResult
	ScalingRules struct {
		ScalingRule []ScalingRuleItemType
	}
}

type ScalingRuleItemType struct {
	ScalingRuleId   string
	ScalingGroupId  string
	ScalingRuleName string
	AdjustmentType  string
	ScalingRuleAri  string
	Cooldown        int
	AdjustmentValue int
}

// DescribeScalingRules describes scaling rules
//
// You can read doc at https://help.aliyun.com/document_detail/25950.html?spm=5176.doc25949.6.631.RwPguo
func (client *Client) DescribeScalingRules(args *DescribeScalingRulesArgs) (configs []ScalingRuleItemType, pagination *common.PaginationResult, err error) {
	args.Validate()
	response := DescribeScalingRulesResponse{}

	err = client.InvokeByFlattenMethod("DescribeScalingRules", args, &response)

	if err == nil {
		return response.ScalingRules.ScalingRule, &response.PaginationResult, nil
	}

	return nil, nil, err
}

type DeleteScalingRuleArgs struct {
	RegionId      common.Region
	ScalingRuleId string
}

type DeleteScalingRuleResponse struct {
	common.Response
}

// DeleteScalingRule delete scaling rule
//
// You can read doc at https://help.aliyun.com/document_detail/25951.html?spm=5176.doc25950.6.632.HbPLMZ
func (client *Client) DeleteScalingRule(args *DeleteScalingRuleArgs) (resp *DeleteScalingRuleResponse, err error) {
	response := DeleteScalingRuleResponse{}
	err = client.InvokeByFlattenMethod("DeleteScalingRule", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type ExecuteScalingRuleArgs struct {
	ScalingRuleAri string
	ClientToken    string
}

type ExecuteScalingRuleResponse struct {
	common.Response
	ScalingActivityId string
}

// ExecuteScalingRule execute scaling rule
//
// You can read doc at https://help.aliyun.com/document_detail/25953.html?spm=5176.doc25961.6.632.7sXDx6
func (client *Client) ExecuteScalingRule(args *ExecuteScalingRuleArgs) (*ExecuteScalingRuleResponse, error) {
	resp := ExecuteScalingRuleResponse{}
	err := client.InvokeByFlattenMethod("ExecuteScalingRule", args, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
