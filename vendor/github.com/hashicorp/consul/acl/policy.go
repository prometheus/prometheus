package acl

import (
	"fmt"

	"github.com/hashicorp/hcl"
)

const (
	PolicyDeny  = "deny"
	PolicyRead  = "read"
	PolicyWrite = "write"
)

// Policy is used to represent the policy specified by
// an ACL configuration.
type Policy struct {
	ID              string                 `hcl:"-"`
	Agents          []*AgentPolicy         `hcl:"agent,expand"`
	Keys            []*KeyPolicy           `hcl:"key,expand"`
	Nodes           []*NodePolicy          `hcl:"node,expand"`
	Services        []*ServicePolicy       `hcl:"service,expand"`
	Sessions        []*SessionPolicy       `hcl:"session,expand"`
	Events          []*EventPolicy         `hcl:"event,expand"`
	PreparedQueries []*PreparedQueryPolicy `hcl:"query,expand"`
	Keyring         string                 `hcl:"keyring"`
	Operator        string                 `hcl:"operator"`
}

// AgentPolicy represents a policy for working with agent endpoints on nodes
// with specific name prefixes.
type AgentPolicy struct {
	Node   string `hcl:",key"`
	Policy string
}

func (a *AgentPolicy) GoString() string {
	return fmt.Sprintf("%#v", *a)
}

// KeyPolicy represents a policy for a key
type KeyPolicy struct {
	Prefix string `hcl:",key"`
	Policy string
}

func (k *KeyPolicy) GoString() string {
	return fmt.Sprintf("%#v", *k)
}

// NodePolicy represents a policy for a node
type NodePolicy struct {
	Name   string `hcl:",key"`
	Policy string
}

func (n *NodePolicy) GoString() string {
	return fmt.Sprintf("%#v", *n)
}

// ServicePolicy represents a policy for a service
type ServicePolicy struct {
	Name   string `hcl:",key"`
	Policy string
}

func (s *ServicePolicy) GoString() string {
	return fmt.Sprintf("%#v", *s)
}

// SessionPolicy represents a policy for making sessions tied to specific node
// name prefixes.
type SessionPolicy struct {
	Node   string `hcl:",key"`
	Policy string
}

func (s *SessionPolicy) GoString() string {
	return fmt.Sprintf("%#v", *s)
}

// EventPolicy represents a user event policy.
type EventPolicy struct {
	Event  string `hcl:",key"`
	Policy string
}

func (e *EventPolicy) GoString() string {
	return fmt.Sprintf("%#v", *e)
}

// PreparedQueryPolicy represents a prepared query policy.
type PreparedQueryPolicy struct {
	Prefix string `hcl:",key"`
	Policy string
}

func (p *PreparedQueryPolicy) GoString() string {
	return fmt.Sprintf("%#v", *p)
}

// isPolicyValid makes sure the given string matches one of the valid policies.
func isPolicyValid(policy string) bool {
	switch policy {
	case PolicyDeny:
		return true
	case PolicyRead:
		return true
	case PolicyWrite:
		return true
	default:
		return false
	}
}

// Parse is used to parse the specified ACL rules into an
// intermediary set of policies, before being compiled into
// the ACL
func Parse(rules string) (*Policy, error) {
	// Decode the rules
	p := &Policy{}
	if rules == "" {
		// Hot path for empty rules
		return p, nil
	}

	if err := hcl.Decode(p, rules); err != nil {
		return nil, fmt.Errorf("Failed to parse ACL rules: %v", err)
	}

	// Validate the agent policy
	for _, ap := range p.Agents {
		if !isPolicyValid(ap.Policy) {
			return nil, fmt.Errorf("Invalid agent policy: %#v", ap)
		}
	}

	// Validate the key policy
	for _, kp := range p.Keys {
		if !isPolicyValid(kp.Policy) {
			return nil, fmt.Errorf("Invalid key policy: %#v", kp)
		}
	}

	// Validate the node policies
	for _, np := range p.Nodes {
		if !isPolicyValid(np.Policy) {
			return nil, fmt.Errorf("Invalid node policy: %#v", np)
		}
	}

	// Validate the service policies
	for _, sp := range p.Services {
		if !isPolicyValid(sp.Policy) {
			return nil, fmt.Errorf("Invalid service policy: %#v", sp)
		}
	}

	// Validate the session policies
	for _, sp := range p.Sessions {
		if !isPolicyValid(sp.Policy) {
			return nil, fmt.Errorf("Invalid session policy: %#v", sp)
		}
	}

	// Validate the user event policies
	for _, ep := range p.Events {
		if !isPolicyValid(ep.Policy) {
			return nil, fmt.Errorf("Invalid event policy: %#v", ep)
		}
	}

	// Validate the prepared query policies
	for _, pq := range p.PreparedQueries {
		if !isPolicyValid(pq.Policy) {
			return nil, fmt.Errorf("Invalid query policy: %#v", pq)
		}
	}

	// Validate the keyring policy - this one is allowed to be empty
	if p.Keyring != "" && !isPolicyValid(p.Keyring) {
		return nil, fmt.Errorf("Invalid keyring policy: %#v", p.Keyring)
	}

	// Validate the operator policy - this one is allowed to be empty
	if p.Operator != "" && !isPolicyValid(p.Operator) {
		return nil, fmt.Errorf("Invalid operator policy: %#v", p.Operator)
	}

	return p, nil
}
