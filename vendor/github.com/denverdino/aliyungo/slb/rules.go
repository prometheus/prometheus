package slb

import "github.com/denverdino/aliyungo/common"

type CreateRulesResponse struct {
	common.Response
}

type CreateRulesArgs struct {
	RegionId       common.Region
	LoadBalancerId string
	ListenerPort   int
	RuleList       string
}

type Rule struct {
	RuleId         string
	RuleName       string
	Domain         string
	Url            string `json:",omitempty"`
	VServerGroupId string
}

type Rules struct {
	Rule []Rule
}

// Create forward rules
//
// You can read doc at https://help.aliyun.com/document_detail/35226.html?spm=5176.doc35226.6.671.625Omh
func (client *Client) CreateRules(args *CreateRulesArgs) error {
	response := CreateRulesResponse{}
	err := client.Invoke("CreateRules", args, &response)
	if err != nil {
		return err
	}
	return err
}

type DeleteRulesArgs struct {
	RegionId common.Region
	RuleIds  string
}

type DeleteRulesResponse struct {
	common.Response
}

// Delete forward rules
//
// You can read doc at https://help.aliyun.com/document_detail/35227.html?spm=5176.doc35226.6.672.6iNBtR
func (client *Client) DeleteRules(args *DeleteRulesArgs) error {
	response := DeleteRulesResponse{}
	err := client.Invoke("DeleteRules", args, &response)
	if err != nil {
		return err
	}
	return err
}

type SetRuleArgs struct {
	RegionId       common.Region
	RuleId         string
	VServerGroupId string
}

type SetRuleResponse struct {
	common.Response
}

// Modify forward rules
//
// You can read doc at https://help.aliyun.com/document_detail/35228.html?spm=5176.doc35227.6.673.rq40a9
func (client *Client) SetRule(args *SetRuleArgs) error {
	response := SetRuleResponse{}
	err := client.Invoke("SetRule", args, &response)
	if err != nil {
		return err
	}
	return err
}

type DescribeRuleAttributeArgs struct {
	RegionId common.Region
	RuleId   string
}

type DescribeRuleAttributeResponse struct {
	common.Response
	LoadBalancerId string
	ListenerPort   int
	Rule
}

// Describe rule
//
// You can read doc at https://help.aliyun.com/document_detail/35229.html?spm=5176.doc35226.6.674.DRJeKJ
func (client *Client) DescribeRuleAttribute(args *DescribeRuleAttributeArgs) (*DescribeRuleAttributeResponse, error) {
	response := &DescribeRuleAttributeResponse{}
	err := client.Invoke("DescribeRuleAttribute", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type DescribeRulesArgs struct {
	RegionId       common.Region
	LoadBalancerId string
	ListenerPort   int
}

type DescribeRulesResponse struct {
	common.Response
	Rules struct {
		Rule []Rule
	}
}

// Describe rule
//
// You can read doc at https://help.aliyun.com/document_detail/35229.html?spm=5176.doc35226.6.674.DRJeKJ
func (client *Client) DescribeRules(args *DescribeRulesArgs) (*DescribeRulesResponse, error) {
	response := &DescribeRulesResponse{}
	err := client.Invoke("DescribeRules", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}
