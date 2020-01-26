package rules

import (
	"testing"

	"github.com/gophercloud/gophercloud/acceptance/clients"
	accpolicies "github.com/gophercloud/gophercloud/acceptance/openstack/networking/v2/extensions/qos/policies"
	"github.com/gophercloud/gophercloud/acceptance/tools"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/qos/policies"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/qos/rules"
	th "github.com/gophercloud/gophercloud/testhelper"
)

func TestBandwidthLimitRulesCRUD(t *testing.T) {
	client, err := clients.NewNetworkV2Client()
	th.AssertNoErr(t, err)

	// Create a QoS policy
	policy, err := accpolicies.CreateQoSPolicy(t, client)
	th.AssertNoErr(t, err)
	defer policies.Delete(client, policy.ID)

	tools.PrintResource(t, policy)

	// Create a QoS policy rule.
	rule, err := CreateBandwidthLimitRule(t, client, policy.ID)
	th.AssertNoErr(t, err)
	defer rules.DeleteBandwidthLimitRule(client, policy.ID, rule.ID)

	// Update the QoS policy rule.
	newMaxBurstKBps := 0
	updateOpts := rules.UpdateBandwidthLimitRuleOpts{
		MaxBurstKBps: &newMaxBurstKBps,
	}
	newRule, err := rules.UpdateBandwidthLimitRule(client, policy.ID, rule.ID, updateOpts).ExtractBandwidthLimitRule()
	th.AssertNoErr(t, err)

	tools.PrintResource(t, newRule)
	th.AssertEquals(t, newRule.MaxBurstKBps, 0)

	allPages, err := rules.ListBandwidthLimitRules(client, policy.ID, rules.BandwidthLimitRulesListOpts{}).AllPages()
	th.AssertNoErr(t, err)

	allRules, err := rules.ExtractBandwidthLimitRules(allPages)
	th.AssertNoErr(t, err)

	var found bool
	for _, rule := range allRules {
		if rule.ID == newRule.ID {
			found = true
		}
	}

	th.AssertEquals(t, found, true)
}

func TestDSCPMarkingRulesCRUD(t *testing.T) {
	client, err := clients.NewNetworkV2Client()
	th.AssertNoErr(t, err)

	// Create a QoS policy
	policy, err := accpolicies.CreateQoSPolicy(t, client)
	th.AssertNoErr(t, err)
	defer policies.Delete(client, policy.ID)

	tools.PrintResource(t, policy)

	// Create a QoS policy rule.
	rule, err := CreateDSCPMarkingRule(t, client, policy.ID)
	th.AssertNoErr(t, err)
	defer rules.DeleteDSCPMarkingRule(client, policy.ID, rule.ID)

	// Update the QoS policy rule.
	dscpMark := 20
	updateOpts := rules.UpdateDSCPMarkingRuleOpts{
		DSCPMark: &dscpMark,
	}
	newRule, err := rules.UpdateDSCPMarkingRule(client, policy.ID, rule.ID, updateOpts).ExtractDSCPMarkingRule()
	th.AssertNoErr(t, err)

	tools.PrintResource(t, newRule)
	th.AssertEquals(t, newRule.DSCPMark, 20)

	allPages, err := rules.ListDSCPMarkingRules(client, policy.ID, rules.DSCPMarkingRulesListOpts{}).AllPages()
	th.AssertNoErr(t, err)

	allRules, err := rules.ExtractDSCPMarkingRules(allPages)
	th.AssertNoErr(t, err)

	var found bool
	for _, rule := range allRules {
		if rule.ID == newRule.ID {
			found = true
		}
	}

	th.AssertEquals(t, found, true)
}

func TestMinimumBandwidthRulesCRUD(t *testing.T) {
	client, err := clients.NewNetworkV2Client()
	th.AssertNoErr(t, err)

	// Create a QoS policy
	policy, err := accpolicies.CreateQoSPolicy(t, client)
	th.AssertNoErr(t, err)
	defer policies.Delete(client, policy.ID)

	tools.PrintResource(t, policy)

	// Create a QoS policy rule.
	rule, err := CreateMinimumBandwidthRule(t, client, policy.ID)
	th.AssertNoErr(t, err)
	defer rules.DeleteMinimumBandwidthRule(client, policy.ID, rule.ID)

	// Update the QoS policy rule.
	minKBps := 500
	updateOpts := rules.UpdateMinimumBandwidthRuleOpts{
		MinKBps: &minKBps,
	}
	newRule, err := rules.UpdateMinimumBandwidthRule(client, policy.ID, rule.ID, updateOpts).ExtractMinimumBandwidthRule()
	th.AssertNoErr(t, err)

	tools.PrintResource(t, newRule)
	th.AssertEquals(t, newRule.MinKBps, 500)

	allPages, err := rules.ListMinimumBandwidthRules(client, policy.ID, rules.MinimumBandwidthRulesListOpts{}).AllPages()
	th.AssertNoErr(t, err)

	allRules, err := rules.ExtractMinimumBandwidthRules(allPages)
	th.AssertNoErr(t, err)

	var found bool
	for _, rule := range allRules {
		if rule.ID == newRule.ID {
			found = true
		}
	}

	th.AssertEquals(t, found, true)
}
