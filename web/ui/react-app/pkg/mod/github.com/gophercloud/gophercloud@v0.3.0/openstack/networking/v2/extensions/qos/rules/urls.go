package rules

import "github.com/gophercloud/gophercloud"

const (
	rootPath = "qos/policies"

	bandwidthLimitRulesResourcePath   = "bandwidth_limit_rules"
	dscpMarkingRulesResourcePath      = "dscp_marking_rules"
	minimumBandwidthRulesResourcePath = "minimum_bandwidth_rules"
)

func bandwidthLimitRulesRootURL(c *gophercloud.ServiceClient, policyID string) string {
	return c.ServiceURL(rootPath, policyID, bandwidthLimitRulesResourcePath)
}

func bandwidthLimitRulesResourceURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return c.ServiceURL(rootPath, policyID, bandwidthLimitRulesResourcePath, ruleID)
}

func listBandwidthLimitRulesURL(c *gophercloud.ServiceClient, policyID string) string {
	return bandwidthLimitRulesRootURL(c, policyID)
}

func getBandwidthLimitRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return bandwidthLimitRulesResourceURL(c, policyID, ruleID)
}

func createBandwidthLimitRuleURL(c *gophercloud.ServiceClient, policyID string) string {
	return bandwidthLimitRulesRootURL(c, policyID)
}

func updateBandwidthLimitRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return bandwidthLimitRulesResourceURL(c, policyID, ruleID)
}

func deleteBandwidthLimitRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return bandwidthLimitRulesResourceURL(c, policyID, ruleID)
}

func dscpMarkingRulesRootURL(c *gophercloud.ServiceClient, policyID string) string {
	return c.ServiceURL(rootPath, policyID, dscpMarkingRulesResourcePath)
}

func dscpMarkingRulesResourceURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return c.ServiceURL(rootPath, policyID, dscpMarkingRulesResourcePath, ruleID)
}

func listDSCPMarkingRulesURL(c *gophercloud.ServiceClient, policyID string) string {
	return dscpMarkingRulesRootURL(c, policyID)
}

func getDSCPMarkingRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return dscpMarkingRulesResourceURL(c, policyID, ruleID)
}

func createDSCPMarkingRuleURL(c *gophercloud.ServiceClient, policyID string) string {
	return dscpMarkingRulesRootURL(c, policyID)
}

func updateDSCPMarkingRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return dscpMarkingRulesResourceURL(c, policyID, ruleID)
}

func deleteDSCPMarkingRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return dscpMarkingRulesResourceURL(c, policyID, ruleID)
}

func minimumBandwidthRulesRootURL(c *gophercloud.ServiceClient, policyID string) string {
	return c.ServiceURL(rootPath, policyID, minimumBandwidthRulesResourcePath)
}

func minimumBandwidthRulesResourceURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return c.ServiceURL(rootPath, policyID, minimumBandwidthRulesResourcePath, ruleID)
}

func listMinimumBandwidthRulesURL(c *gophercloud.ServiceClient, policyID string) string {
	return minimumBandwidthRulesRootURL(c, policyID)
}

func getMinimumBandwidthRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return minimumBandwidthRulesResourceURL(c, policyID, ruleID)
}

func createMinimumBandwidthRuleURL(c *gophercloud.ServiceClient, policyID string) string {
	return minimumBandwidthRulesRootURL(c, policyID)
}

func updateMinimumBandwidthRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return minimumBandwidthRulesResourceURL(c, policyID, ruleID)
}

func deleteMinimumBandwidthRuleURL(c *gophercloud.ServiceClient, policyID, ruleID string) string {
	return minimumBandwidthRulesResourceURL(c, policyID, ruleID)
}
