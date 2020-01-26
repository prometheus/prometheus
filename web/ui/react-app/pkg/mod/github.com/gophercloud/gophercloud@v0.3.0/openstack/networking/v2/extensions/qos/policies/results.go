package policies

import (
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// QoSPolicyExt represents additional resource attributes available with the QoS extension.
type QoSPolicyExt struct {
	// QoSPolicyID represents an associated QoS policy.
	QoSPolicyID string `json:"qos_policy_id"`
}

type commonResult struct {
	gophercloud.Result
}

// GetResult represents the result of a get operation. Call its Extract
// method to interpret it as a QoS policy.
type GetResult struct {
	commonResult
}

// CreateResult represents the result of a Create operation. Call its Extract
// method to interpret it as a QoS policy.
type CreateResult struct {
	commonResult
}

// UpdateResult represents the result of a Create operation. Call its Extract
// method to interpret it as a QoS policy.
type UpdateResult struct {
	commonResult
}

// DeleteResult represents the result of a delete operation. Call its
// ExtractErr method to determine if the request succeeded or failed.
type DeleteResult struct {
	gophercloud.ErrResult
}

// Extract is a function that accepts a result and extracts a QoS policy resource.
func (r commonResult) Extract() (*Policy, error) {
	var s struct {
		Policy *Policy `json:"policy"`
	}
	err := r.ExtractInto(&s)
	return s.Policy, err
}

// Policy represents a QoS policy.
type Policy struct {
	// ID is the id of the policy.
	ID string `json:"id"`

	// Name is the human-readable name of the policy.
	Name string `json:"name"`

	// TenantID is the id of the Identity project.
	TenantID string `json:"tenant_id"`

	// ProjectID is the id of the Identity project.
	ProjectID string `json:"project_id"`

	// CreatedAt is the time at which the policy has been created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the time at which the policy has been created.
	UpdatedAt time.Time `json:"updated_at"`

	// IsDefault indicates if the policy is default policy or not.
	IsDefault bool `json:"is_default"`

	// Description is thehuman-readable description for the resource.
	Description string `json:"description"`

	// Shared indicates whether this policy is shared across all projects.
	Shared bool `json:"shared"`

	// RevisionNumber represents revision number of the policy.
	RevisionNumber int `json:"revision_number"`

	// Rules represents QoS rules of the policy.
	Rules []map[string]interface{} `json:"rules"`

	// Tags optionally set via extensions/attributestags
	Tags []string `json:"tags"`
}

// PolicyPage stores a single page of Policies from a List() API call.
type PolicyPage struct {
	pagination.LinkedPageBase
}

// NextPageURL is invoked when a paginated collection of policies has reached
// the end of a page and the pager seeks to traverse over a new one.
// In order to do this, it needs to construct the next page's URL.
func (r PolicyPage) NextPageURL() (string, error) {
	var s struct {
		Links []gophercloud.Link `json:"policies_links"`
	}
	err := r.ExtractInto(&s)
	if err != nil {
		return "", err
	}
	return gophercloud.ExtractNextURL(s.Links)
}

// IsEmpty checks whether a PolicyPage is empty.
func (r PolicyPage) IsEmpty() (bool, error) {
	is, err := ExtractPolicies(r)
	return len(is) == 0, err
}

// ExtractPolicies accepts a PolicyPage, and extracts the elements into a slice of Policies.
func ExtractPolicies(r pagination.Page) ([]Policy, error) {
	var s []Policy
	err := ExtractPolicysInto(r, &s)
	return s, err
}

// ExtractPoliciesInto extracts the elements into a slice of RBAC Policy structs.
func ExtractPolicysInto(r pagination.Page, v interface{}) error {
	return r.(PolicyPage).Result.ExtractIntoSlicePtr(v, "policies")
}
