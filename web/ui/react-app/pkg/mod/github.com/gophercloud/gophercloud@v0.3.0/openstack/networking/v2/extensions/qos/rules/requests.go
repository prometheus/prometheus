package rules

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// ListOptsBuilder allows extensions to add additional parameters to the
// List request.
type BandwidthLimitRulesListOptsBuilder interface {
	ToBandwidthLimitRulesListQuery() (string, error)
}

// ListOpts allows the filtering and sorting of paginated collections through
// the Neutron API. Filtering is achieved by passing in struct field values
// that map to the BandwidthLimitRules attributes you want to see returned.
// SortKey allows you to sort by a particular BandwidthLimitRule attribute.
// SortDir sets the direction, and is either `asc' or `desc'.
// Marker and Limit are used for the pagination.
type BandwidthLimitRulesListOpts struct {
	ID           string `q:"id"`
	TenantID     string `q:"tenant_id"`
	MaxKBps      int    `q:"max_kbps"`
	MaxBurstKBps int    `q:"max_burst_kbps"`
	Direction    string `q:"direction"`
	Limit        int    `q:"limit"`
	Marker       string `q:"marker"`
	SortKey      string `q:"sort_key"`
	SortDir      string `q:"sort_dir"`
	Tags         string `q:"tags"`
	TagsAny      string `q:"tags-any"`
	NotTags      string `q:"not-tags"`
	NotTagsAny   string `q:"not-tags-any"`
}

// ToBandwidthLimitRulesListQuery formats a ListOpts into a query string.
func (opts BandwidthLimitRulesListOpts) ToBandwidthLimitRulesListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

// ListBandwidthLimitRules returns a Pager which allows you to iterate over a collection of
// BandwidthLimitRules. It accepts a ListOpts struct, which allows you to filter and sort
// the returned collection for greater efficiency.
func ListBandwidthLimitRules(c *gophercloud.ServiceClient, policyID string, opts BandwidthLimitRulesListOptsBuilder) pagination.Pager {
	url := listBandwidthLimitRulesURL(c, policyID)
	if opts != nil {
		query, err := opts.ToBandwidthLimitRulesListQuery()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query
	}
	return pagination.NewPager(c, url, func(r pagination.PageResult) pagination.Page {
		return BandwidthLimitRulePage{pagination.LinkedPageBase{PageResult: r}}

	})
}

// GetBandwidthLimitRule retrieves a specific BandwidthLimitRule based on its ID.
func GetBandwidthLimitRule(c *gophercloud.ServiceClient, policyID, ruleID string) (r GetBandwidthLimitRuleResult) {
	_, r.Err = c.Get(getBandwidthLimitRuleURL(c, policyID, ruleID), &r.Body, nil)
	return
}

// CreateBandwidthLimitRuleOptsBuilder allows to add additional parameters to the
// CreateBandwidthLimitRule request.
type CreateBandwidthLimitRuleOptsBuilder interface {
	ToBandwidthLimitRuleCreateMap() (map[string]interface{}, error)
}

// CreateBandwidthLimitRuleOpts specifies parameters of a new BandwidthLimitRule.
type CreateBandwidthLimitRuleOpts struct {
	// MaxKBps is a maximum kilobits per second. It's a required parameter.
	MaxKBps int `json:"max_kbps"`

	// MaxBurstKBps is a maximum burst size in kilobits.
	MaxBurstKBps int `json:"max_burst_kbps,omitempty"`

	// Direction represents the direction of traffic.
	Direction string `json:"direction,omitempty"`
}

// ToBandwidthLimitRuleCreateMap constructs a request body from CreateBandwidthLimitRuleOpts.
func (opts CreateBandwidthLimitRuleOpts) ToBandwidthLimitRuleCreateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "bandwidth_limit_rule")
}

// CreateBandwidthLimitRule requests the creation of a new BandwidthLimitRule on the server.
func CreateBandwidthLimitRule(client *gophercloud.ServiceClient, policyID string, opts CreateBandwidthLimitRuleOptsBuilder) (r CreateBandwidthLimitRuleResult) {
	b, err := opts.ToBandwidthLimitRuleCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createBandwidthLimitRuleURL(client, policyID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})
	return
}

// UpdateBandwidthLimitRuleOptsBuilder allows to add additional parameters to the
// UpdateBandwidthLimitRule request.
type UpdateBandwidthLimitRuleOptsBuilder interface {
	ToBandwidthLimitRuleUpdateMap() (map[string]interface{}, error)
}

// UpdateBandwidthLimitRuleOpts specifies parameters for the Update call.
type UpdateBandwidthLimitRuleOpts struct {
	// MaxKBps is a maximum kilobits per second.
	MaxKBps *int `json:"max_kbps,omitempty"`

	// MaxBurstKBps is a maximum burst size in kilobits.
	MaxBurstKBps *int `json:"max_burst_kbps,omitempty"`

	// Direction represents the direction of traffic.
	Direction string `json:"direction,omitempty"`
}

// ToBandwidthLimitRuleUpdateMap constructs a request body from UpdateBandwidthLimitRuleOpts.
func (opts UpdateBandwidthLimitRuleOpts) ToBandwidthLimitRuleUpdateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "bandwidth_limit_rule")
}

// UpdateBandwidthLimitRule requests the creation of a new BandwidthLimitRule on the server.
func UpdateBandwidthLimitRule(client *gophercloud.ServiceClient, policyID, ruleID string, opts UpdateBandwidthLimitRuleOptsBuilder) (r UpdateBandwidthLimitRuleResult) {
	b, err := opts.ToBandwidthLimitRuleUpdateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Put(updateBandwidthLimitRuleURL(client, policyID, ruleID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	return
}

// Delete accepts policy and rule ID and deletes the BandwidthLimitRule associated with them.
func DeleteBandwidthLimitRule(c *gophercloud.ServiceClient, policyID, ruleID string) (r DeleteBandwidthLimitRuleResult) {
	_, r.Err = c.Delete(deleteBandwidthLimitRuleURL(c, policyID, ruleID), nil)
	return
}

// DSCPMarkingRulesListOptsBuilder allows extensions to add additional parameters to the
// List request.
type DSCPMarkingRulesListOptsBuilder interface {
	ToDSCPMarkingRulesListQuery() (string, error)
}

// DSCPMarkingRulesListOpts allows the filtering and sorting of paginated collections through
// the Neutron API. Filtering is achieved by passing in struct field values
// that map to the DSCPMarking attributes you want to see returned.
// SortKey allows you to sort by a particular DSCPMarkingRule attribute.
// SortDir sets the direction, and is either `asc' or `desc'.
// Marker and Limit are used for the pagination.
type DSCPMarkingRulesListOpts struct {
	ID         string `q:"id"`
	TenantID   string `q:"tenant_id"`
	DSCPMark   int    `q:"dscp_mark"`
	Limit      int    `q:"limit"`
	Marker     string `q:"marker"`
	SortKey    string `q:"sort_key"`
	SortDir    string `q:"sort_dir"`
	Tags       string `q:"tags"`
	TagsAny    string `q:"tags-any"`
	NotTags    string `q:"not-tags"`
	NotTagsAny string `q:"not-tags-any"`
}

// ToDSCPMarkingRulesListQuery formats a ListOpts into a query string.
func (opts DSCPMarkingRulesListOpts) ToDSCPMarkingRulesListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

// ListDSCPMarkingRules returns a Pager which allows you to iterate over a collection of
// DSCPMarkingRules. It accepts a ListOpts struct, which allows you to filter and sort
// the returned collection for greater efficiency.
func ListDSCPMarkingRules(c *gophercloud.ServiceClient, policyID string, opts DSCPMarkingRulesListOptsBuilder) pagination.Pager {
	url := listDSCPMarkingRulesURL(c, policyID)
	if opts != nil {
		query, err := opts.ToDSCPMarkingRulesListQuery()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query
	}
	return pagination.NewPager(c, url, func(r pagination.PageResult) pagination.Page {
		return DSCPMarkingRulePage{pagination.LinkedPageBase{PageResult: r}}

	})
}

// GetDSCPMarkingRule retrieves a specific DSCPMarkingRule based on its ID.
func GetDSCPMarkingRule(c *gophercloud.ServiceClient, policyID, ruleID string) (r GetDSCPMarkingRuleResult) {
	_, r.Err = c.Get(getDSCPMarkingRuleURL(c, policyID, ruleID), &r.Body, nil)
	return
}

// CreateDSCPMarkingRuleOptsBuilder allows to add additional parameters to the
// CreateDSCPMarkingRule request.
type CreateDSCPMarkingRuleOptsBuilder interface {
	ToDSCPMarkingRuleCreateMap() (map[string]interface{}, error)
}

// CreateDSCPMarkingRuleOpts specifies parameters of a new DSCPMarkingRule.
type CreateDSCPMarkingRuleOpts struct {
	// DSCPMark contains DSCP mark value.
	DSCPMark int `json:"dscp_mark"`
}

// ToDSCPMarkingRuleCreateMap constructs a request body from CreateDSCPMarkingRuleOpts.
func (opts CreateDSCPMarkingRuleOpts) ToDSCPMarkingRuleCreateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "dscp_marking_rule")
}

// CreateDSCPMarkingRule requests the creation of a new DSCPMarkingRule on the server.
func CreateDSCPMarkingRule(client *gophercloud.ServiceClient, policyID string, opts CreateDSCPMarkingRuleOptsBuilder) (r CreateDSCPMarkingRuleResult) {
	b, err := opts.ToDSCPMarkingRuleCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createDSCPMarkingRuleURL(client, policyID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})
	return
}

// UpdateDSCPMarkingRuleOptsBuilder allows to add additional parameters to the
// UpdateDSCPMarkingRule request.
type UpdateDSCPMarkingRuleOptsBuilder interface {
	ToDSCPMarkingRuleUpdateMap() (map[string]interface{}, error)
}

// UpdateDSCPMarkingRuleOpts specifies parameters for the Update call.
type UpdateDSCPMarkingRuleOpts struct {
	// DSCPMark contains DSCP mark value.
	DSCPMark *int `json:"dscp_mark,omitempty"`
}

// ToDSCPMarkingRuleUpdateMap constructs a request body from UpdateDSCPMarkingRuleOpts.
func (opts UpdateDSCPMarkingRuleOpts) ToDSCPMarkingRuleUpdateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "dscp_marking_rule")
}

// UpdateDSCPMarkingRule requests the creation of a new DSCPMarkingRule on the server.
func UpdateDSCPMarkingRule(client *gophercloud.ServiceClient, policyID, ruleID string, opts UpdateDSCPMarkingRuleOptsBuilder) (r UpdateDSCPMarkingRuleResult) {
	b, err := opts.ToDSCPMarkingRuleUpdateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Put(updateDSCPMarkingRuleURL(client, policyID, ruleID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	return
}

// DeleteDSCPMarkingRule accepts policy and rule ID and deletes the DSCPMarkingRule associated with them.
func DeleteDSCPMarkingRule(c *gophercloud.ServiceClient, policyID, ruleID string) (r DeleteDSCPMarkingRuleResult) {
	_, r.Err = c.Delete(deleteDSCPMarkingRuleURL(c, policyID, ruleID), nil)
	return
}

// ListOptsBuilder allows extensions to add additional parameters to the
// List request.
type MinimumBandwidthRulesListOptsBuilder interface {
	ToMinimumBandwidthRulesListQuery() (string, error)
}

// ListOpts allows the filtering and sorting of paginated collections through
// the Neutron API. Filtering is achieved by passing in struct field values
// that map to the MinimumBandwidthRules attributes you want to see returned.
// SortKey allows you to sort by a particular MinimumBandwidthRule attribute.
// SortDir sets the direction, and is either `asc' or `desc'.
// Marker and Limit are used for the pagination.
type MinimumBandwidthRulesListOpts struct {
	ID         string `q:"id"`
	TenantID   string `q:"tenant_id"`
	MinKBps    int    `q:"min_kbps"`
	Direction  string `q:"direction"`
	Limit      int    `q:"limit"`
	Marker     string `q:"marker"`
	SortKey    string `q:"sort_key"`
	SortDir    string `q:"sort_dir"`
	Tags       string `q:"tags"`
	TagsAny    string `q:"tags-any"`
	NotTags    string `q:"not-tags"`
	NotTagsAny string `q:"not-tags-any"`
}

// ToMinimumBandwidthRulesListQuery formats a ListOpts into a query string.
func (opts MinimumBandwidthRulesListOpts) ToMinimumBandwidthRulesListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

// ListMinimumBandwidthRules returns a Pager which allows you to iterate over a collection of
// MinimumBandwidthRules. It accepts a ListOpts struct, which allows you to filter and sort
// the returned collection for greater efficiency.
func ListMinimumBandwidthRules(c *gophercloud.ServiceClient, policyID string, opts MinimumBandwidthRulesListOptsBuilder) pagination.Pager {
	url := listMinimumBandwidthRulesURL(c, policyID)
	if opts != nil {
		query, err := opts.ToMinimumBandwidthRulesListQuery()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query
	}
	return pagination.NewPager(c, url, func(r pagination.PageResult) pagination.Page {
		return MinimumBandwidthRulePage{pagination.LinkedPageBase{PageResult: r}}

	})
}

// GetMinimumBandwidthRule retrieves a specific MinimumBandwidthRule based on its ID.
func GetMinimumBandwidthRule(c *gophercloud.ServiceClient, policyID, ruleID string) (r GetMinimumBandwidthRuleResult) {
	_, r.Err = c.Get(getMinimumBandwidthRuleURL(c, policyID, ruleID), &r.Body, nil)
	return
}

// CreateMinimumBandwidthRuleOptsBuilder allows to add additional parameters to the
// CreateMinimumBandwidthRule request.
type CreateMinimumBandwidthRuleOptsBuilder interface {
	ToMinimumBandwidthRuleCreateMap() (map[string]interface{}, error)
}

// CreateMinimumBandwidthRuleOpts specifies parameters of a new MinimumBandwidthRule.
type CreateMinimumBandwidthRuleOpts struct {
	// MaxKBps is a minimum kilobits per second. It's a required parameter.
	MinKBps int `json:"min_kbps"`

	// Direction represents the direction of traffic.
	Direction string `json:"direction,omitempty"`
}

// ToMinimumBandwidthRuleCreateMap constructs a request body from CreateMinimumBandwidthRuleOpts.
func (opts CreateMinimumBandwidthRuleOpts) ToMinimumBandwidthRuleCreateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "minimum_bandwidth_rule")
}

// CreateMinimumBandwidthRule requests the creation of a new MinimumBandwidthRule on the server.
func CreateMinimumBandwidthRule(client *gophercloud.ServiceClient, policyID string, opts CreateMinimumBandwidthRuleOptsBuilder) (r CreateMinimumBandwidthRuleResult) {
	b, err := opts.ToMinimumBandwidthRuleCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createMinimumBandwidthRuleURL(client, policyID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})
	return
}

// UpdateMinimumBandwidthRuleOptsBuilder allows to add additional parameters to the
// UpdateMinimumBandwidthRule request.
type UpdateMinimumBandwidthRuleOptsBuilder interface {
	ToMinimumBandwidthRuleUpdateMap() (map[string]interface{}, error)
}

// UpdateMinimumBandwidthRuleOpts specifies parameters for the Update call.
type UpdateMinimumBandwidthRuleOpts struct {
	// MaxKBps is a minimum kilobits per second. It's a required parameter.
	MinKBps *int `json:"min_kbps,omitempty"`

	// Direction represents the direction of traffic.
	Direction string `json:"direction,omitempty"`
}

// ToMinimumBandwidthRuleUpdateMap constructs a request body from UpdateMinimumBandwidthRuleOpts.
func (opts UpdateMinimumBandwidthRuleOpts) ToMinimumBandwidthRuleUpdateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "minimum_bandwidth_rule")
}

// UpdateMinimumBandwidthRule requests the creation of a new MinimumBandwidthRule on the server.
func UpdateMinimumBandwidthRule(client *gophercloud.ServiceClient, policyID, ruleID string, opts UpdateMinimumBandwidthRuleOptsBuilder) (r UpdateMinimumBandwidthRuleResult) {
	b, err := opts.ToMinimumBandwidthRuleUpdateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Put(updateMinimumBandwidthRuleURL(client, policyID, ruleID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	return
}

// DeleteMinimumBandwidthRule accepts policy and rule ID and deletes the MinimumBandwidthRule associated with them.
func DeleteMinimumBandwidthRule(c *gophercloud.ServiceClient, policyID, ruleID string) (r DeleteMinimumBandwidthRuleResult) {
	_, r.Err = c.Delete(deleteMinimumBandwidthRuleURL(c, policyID, ruleID), nil)
	return
}
