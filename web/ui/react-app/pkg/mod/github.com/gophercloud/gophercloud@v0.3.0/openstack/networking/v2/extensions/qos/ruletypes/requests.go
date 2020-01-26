package ruletypes

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// ListRuleTypes returns the list of rule types from the server
func ListRuleTypes(c *gophercloud.ServiceClient) (result pagination.Pager) {
	return pagination.NewPager(c, listRuleTypesURL(c), func(r pagination.PageResult) pagination.Page {
		return ListRuleTypesPage{pagination.SinglePageBase(r)}
	})
}

// GetRuleType retrieves a specific QoS RuleType based on its name.
func GetRuleType(c *gophercloud.ServiceClient, name string) (r GetResult) {
	_, r.Err = c.Get(getRuleTypeURL(c, name), &r.Body, nil)
	return
}
