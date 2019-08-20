package slb

import (
	"encoding/json"
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestCreateRules(t *testing.T) {
	client := NewTestClientForDebug()

	rulesArr := []Rule{
		Rule{RuleName: "rule-001", Domain: "datapaking.com", Url: "/rule0001", VServerGroupId: TestVServerGroupID},
		Rule{RuleName: "rule-002", Domain: "datapaking.com", Url: "/rule0002", VServerGroupId: TestVServerGroupID},
	}
	ruleStr, _ := json.Marshal(rulesArr)

	args := &CreateRulesArgs{
		RegionId:       common.Beijing,
		LoadBalancerId: TestLoadBlancerID,
		ListenerPort:   TestListenerPort,
		RuleList:       string(ruleStr),
	}

	err := client.CreateRules(args)
	if err != nil {
		t.Fatalf("failed to create rules error %++v", err)
	}

	t.Logf("create rules ok")
}

func TestSetRule(t *testing.T) {
	client := NewTestClientForDebug()

	args := &SetRuleArgs{
		RegionId:       common.Beijing,
		RuleId:         "rule-2zexlb2k7fybx",
		VServerGroupId: "rsp-2zef5v6xvfyug",
	}

	err := client.SetRule(args)
	if err != nil {
		t.Fatalf("failed to set rule error %++v", err)
	}

	t.Logf("set rule ok")
}

func TestDescribeRuleAttribute(t *testing.T) {
	client := NewTestClientForDebug()

	args := &DescribeRuleAttributeArgs{
		RegionId: common.Beijing,
		RuleId:   "rule-2zexlb2k7fybx",
	}

	rule, err := client.DescribeRuleAttribute(args)
	if err != nil {
		t.Fatalf("failed to describe rule error %++v", err)
	}

	t.Logf("rule is %++v", rule)
}

func TestDescribeRules(t *testing.T) {
	client := NewTestClientForDebug()

	args := &DescribeRulesArgs{
		RegionId:       common.Beijing,
		LoadBalancerId: TestLoadBlancerID,
		ListenerPort:   TestListenerPort,
	}

	rules, err := client.DescribeRules(args)
	if err != nil {
		t.Fatalf("failed to describe rules error %++v", err)
	}

	t.Logf("rule list is %++v", rules)
}

func TestDeleteRules(t *testing.T) {
	client := NewTestClientForDebug()

	args := &DeleteRulesArgs{
		RegionId: common.Beijing,
		RuleIds:  "['rule-2zexlb2k7fybx']",
	}

	err := client.DeleteRules(args)
	if err != nil {
		t.Fatalf("failed to delete rules error %++v", err)
	}

	t.Logf("delete rules ok")
}
