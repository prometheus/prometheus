package policies

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/pagination"
)

// PortCreateOptsExt adds QoS options to the base ports.CreateOpts.
type PortCreateOptsExt struct {
	ports.CreateOptsBuilder

	// QoSPolicyID represents an associated QoS policy.
	QoSPolicyID string `json:"qos_policy_id,omitempty"`
}

// ToPortCreateMap casts a CreateOpts struct to a map.
func (opts PortCreateOptsExt) ToPortCreateMap() (map[string]interface{}, error) {
	base, err := opts.CreateOptsBuilder.ToPortCreateMap()
	if err != nil {
		return nil, err
	}

	port := base["port"].(map[string]interface{})

	if opts.QoSPolicyID != "" {
		port["qos_policy_id"] = opts.QoSPolicyID
	}

	return base, nil
}

// PortUpdateOptsExt adds QoS options to the base ports.UpdateOpts.
type PortUpdateOptsExt struct {
	ports.UpdateOptsBuilder

	// QoSPolicyID represents an associated QoS policy.
	// Setting it to a pointer of an empty string will remove associated QoS policy from port.
	QoSPolicyID *string `json:"qos_policy_id,omitempty"`
}

// ToPortUpdateMap casts a UpdateOpts struct to a map.
func (opts PortUpdateOptsExt) ToPortUpdateMap() (map[string]interface{}, error) {
	base, err := opts.UpdateOptsBuilder.ToPortUpdateMap()
	if err != nil {
		return nil, err
	}

	port := base["port"].(map[string]interface{})

	if opts.QoSPolicyID != nil {
		qosPolicyID := *opts.QoSPolicyID
		if qosPolicyID != "" {
			port["qos_policy_id"] = qosPolicyID
		} else {
			port["qos_policy_id"] = nil
		}
	}

	return base, nil
}

// NetworkCreateOptsExt adds QoS options to the base networks.CreateOpts.
type NetworkCreateOptsExt struct {
	networks.CreateOptsBuilder

	// QoSPolicyID represents an associated QoS policy.
	QoSPolicyID string `json:"qos_policy_id,omitempty"`
}

// ToNetworkCreateMap casts a CreateOpts struct to a map.
func (opts NetworkCreateOptsExt) ToNetworkCreateMap() (map[string]interface{}, error) {
	base, err := opts.CreateOptsBuilder.ToNetworkCreateMap()
	if err != nil {
		return nil, err
	}

	network := base["network"].(map[string]interface{})

	if opts.QoSPolicyID != "" {
		network["qos_policy_id"] = opts.QoSPolicyID
	}

	return base, nil
}

// NetworkUpdateOptsExt adds QoS options to the base networks.UpdateOpts.
type NetworkUpdateOptsExt struct {
	networks.UpdateOptsBuilder

	// QoSPolicyID represents an associated QoS policy.
	// Setting it to a pointer of an empty string will remove associated QoS policy from network.
	QoSPolicyID *string `json:"qos_policy_id,omitempty"`
}

// ToNetworkUpdateMap casts a UpdateOpts struct to a map.
func (opts NetworkUpdateOptsExt) ToNetworkUpdateMap() (map[string]interface{}, error) {
	base, err := opts.UpdateOptsBuilder.ToNetworkUpdateMap()
	if err != nil {
		return nil, err
	}

	network := base["network"].(map[string]interface{})

	if opts.QoSPolicyID != nil {
		qosPolicyID := *opts.QoSPolicyID
		if qosPolicyID != "" {
			network["qos_policy_id"] = qosPolicyID
		} else {
			network["qos_policy_id"] = nil
		}
	}

	return base, nil
}

// PolicyListOptsBuilder allows extensions to add additional parameters to the List request.
type PolicyListOptsBuilder interface {
	ToPolicyListQuery() (string, error)
}

// ListOpts allows the filtering and sorting of paginated collections through
// the Neutron API. Filtering is achieved by passing in struct field values
// that map to the Policy attributes you want to see returned.
// SortKey allows you to sort by a particular Policy attribute.
// SortDir sets the direction, and is either `asc' or `desc'.
// Marker and Limit are used for the pagination.
type ListOpts struct {
	ID             string `q:"id"`
	TenantID       string `q:"tenant_id"`
	ProjectID      string `q:"project_id"`
	Name           string `q:"name"`
	Description    string `q:"description"`
	RevisionNumber *int   `q:"revision_number"`
	IsDefault      *bool  `q:"is_default"`
	Shared         *bool  `q:"shared"`
	Limit          int    `q:"limit"`
	Marker         string `q:"marker"`
	SortKey        string `q:"sort_key"`
	SortDir        string `q:"sort_dir"`
	Tags           string `q:"tags"`
	TagsAny        string `q:"tags-any"`
	NotTags        string `q:"not-tags"`
	NotTagsAny     string `q:"not-tags-any"`
}

// ToPolicyListQuery formats a ListOpts into a query string.
func (opts ListOpts) ToPolicyListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

// List returns a Pager which allows you to iterate over a collection of
// Policy. It accepts a ListOpts struct, which allows you to filter and sort
// the returned collection for greater efficiency.
func List(c *gophercloud.ServiceClient, opts PolicyListOptsBuilder) pagination.Pager {
	url := listURL(c)
	if opts != nil {
		query, err := opts.ToPolicyListQuery()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query
	}
	return pagination.NewPager(c, url, func(r pagination.PageResult) pagination.Page {
		return PolicyPage{pagination.LinkedPageBase{PageResult: r}}

	})
}

// Get retrieves a specific QoS policy based on its ID.
func Get(c *gophercloud.ServiceClient, id string) (r GetResult) {
	_, r.Err = c.Get(getURL(c, id), &r.Body, nil)
	return
}

// CreateOptsBuilder allows to add additional parameters to the
// Create request.
type CreateOptsBuilder interface {
	ToPolicyCreateMap() (map[string]interface{}, error)
}

// CreateOpts specifies parameters of a new QoS policy.
type CreateOpts struct {
	// Name is the human-readable name of the QoS policy.
	Name string `json:"name"`

	// TenantID is the id of the Identity project.
	TenantID string `json:"tenant_id,omitempty"`

	// ProjectID is the id of the Identity project.
	ProjectID string `json:"project_id,omitempty"`

	// Shared indicates whether this QoS policy is shared across all projects.
	Shared bool `json:"shared,omitempty"`

	// Description is the human-readable description for the QoS policy.
	Description string `json:"description,omitempty"`

	// IsDefault indicates if this QoS policy is default policy or not.
	IsDefault bool `json:"is_default,omitempty"`
}

// ToPolicyCreateMap constructs a request body from CreateOpts.
func (opts CreateOpts) ToPolicyCreateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "policy")
}

// Create requests the creation of a new QoS policy on the server.
func Create(client *gophercloud.ServiceClient, opts CreateOptsBuilder) (r CreateResult) {
	b, err := opts.ToPolicyCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createURL(client), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})
	return
}

// UpdateOptsBuilder allows extensions to add additional parameters to the
// Update request.
type UpdateOptsBuilder interface {
	ToPolicyUpdateMap() (map[string]interface{}, error)
}

// UpdateOpts represents options used to update a QoS policy.
type UpdateOpts struct {
	// Name is the human-readable name of the QoS policy.
	Name string `json:"name,omitempty"`

	// Shared indicates whether this QoS policy is shared across all projects.
	Shared *bool `json:"shared,omitempty"`

	// Description is the human-readable description for the QoS policy.
	Description *string `json:"description,omitempty"`

	// IsDefault indicates if this QoS policy is default policy or not.
	IsDefault *bool `json:"is_default,omitempty"`
}

// ToPolicyUpdateMap builds a request body from UpdateOpts.
func (opts UpdateOpts) ToPolicyUpdateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "policy")
}

// Update accepts a UpdateOpts struct and updates an existing policy using the
// values provided.
func Update(c *gophercloud.ServiceClient, policyID string, opts UpdateOptsBuilder) (r UpdateResult) {
	b, err := opts.ToPolicyUpdateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = c.Put(updateURL(c, policyID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	return
}

// Delete accepts a unique ID and deletes the QoS policy associated with it.
func Delete(c *gophercloud.ServiceClient, id string) (r DeleteResult) {
	_, r.Err = c.Delete(deleteURL(c, id), nil)
	return
}
