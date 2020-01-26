package servergroups

import "github.com/gophercloud/gophercloud"

// ExtractPolicy will extract the policy attribute.
// This requires the client to be set to microversion 2.64 or later.
func ExtractPolicy(r gophercloud.Result) (string, error) {
	var s struct {
		Policy string `json:"policy"`
	}
	err := r.ExtractIntoStructPtr(&s, "server_group")

	return s.Policy, err
}

// ExtractRules will extract the rules attribute.
// This requires the client to be set to microversion 2.64 or later.
func ExtractRules(r gophercloud.Result) (Rules, error) {
	var s struct {
		Rules Rules `json:"rules"`
	}
	err := r.ExtractIntoStructPtr(&s, "server_group")

	return s.Rules, err
}

// Rules represents set of rules for a policy.
type Rules struct {
	// MaxServerPerHost specifies how many servers can reside on a single compute host.
	// It can be used only with the "anti-affinity" policy.
	MaxServerPerHost int `json:"max_server_per_host,omitempty"`
}
