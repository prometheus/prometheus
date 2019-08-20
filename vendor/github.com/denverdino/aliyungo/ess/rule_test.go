package ess

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

func TestEssScalingRuleCreationAndDeletion(t *testing.T) {

	if TestIAmRich == false {
		// Avoid payment
		return
	}
	client := NewTestClient(common.Region(RegionId))
	cArgs := CreateScalingRuleArgs{
		RegionId:        common.Region(RegionId),
		ScalingGroupId:  ScalingGroupId,
		AdjustmentType:  TotalCapacity,
		AdjustmentValue: 3,
		ScalingRuleName: "srn",
	}

	csc, err := client.CreateScalingRule(&cArgs)
	if err != nil {
		t.Errorf("Failed to create scaling rule %v", err)
	}
	ruleId := csc.ScalingRuleId
	t.Logf("Rule %s is created successfully.", ruleId)

	mArgs := ModifyScalingRuleArgs{
		RegionId:        common.Region(RegionId),
		ScalingRuleId:   ruleId,
		AdjustmentValue: 2,
		ScalingRuleName: "srnm",
	}

	_, err = client.ModifyScalingRule(&mArgs)
	if err != nil {
		t.Errorf("Failed to modify scaling rule %v", err)
	}
	t.Logf("Rule %s is modify successfully.", ruleId)

	eArgs := ExecuteScalingRuleArgs{
		ScalingRuleAri: csc.ScalingRuleAri,
		ClientToken:    util.CreateRandomString(),
	}
	_, err = client.ExecuteScalingRule(&eArgs)
	if err != nil {
		t.Errorf("Failed to execute scaling rule: %v", err)
	}
	t.Logf("Rule %s is execute successfully.", ruleId)

	sArgs := DescribeScalingRulesArgs{
		RegionId:       common.Region(RegionId),
		ScalingGroupId: ScalingGroupId,
		ScalingRuleId:  []string{ruleId},
	}
	sResp, _, err := client.DescribeScalingRules(&sArgs)
	if len(sResp) < 1 {
		t.Fatalf("Failed to describe rules %s", ruleId)
	}

	rule := sResp[0]
	t.Logf("Rule: %++v  %v", rule, err)

	dcArgs := DeleteScalingRuleArgs{
		RegionId:      common.Region(RegionId),
		ScalingRuleId: ruleId,
	}

	_, err = client.DeleteScalingRule(&dcArgs)
	if err != nil {
		t.Errorf("Failed to delete scaling rule %v", err)
	}

	t.Logf("Rule %s is deleted successfully.", ruleId)

}
