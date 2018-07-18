package api

import (
	"fmt"
	"strings"
)

const (
	// HealthAny is special, and is used as a wild card,
	// not as a specific state.
	HealthAny      = "any"
	HealthPassing  = "passing"
	HealthWarning  = "warning"
	HealthCritical = "critical"
	HealthMaint    = "maintenance"
)

const (
	// NodeMaint is the special key set by a node in maintenance mode.
	NodeMaint = "_node_maintenance"

	// ServiceMaintPrefix is the prefix for a service in maintenance mode.
	ServiceMaintPrefix = "_service_maintenance:"
)

// HealthCheck is used to represent a single check
type HealthCheck struct {
	Node        string
	CheckID     string
	Name        string
	Status      string
	Notes       string
	Output      string
	ServiceID   string
	ServiceName string
	ServiceTags []string

	Definition HealthCheckDefinition
}

// HealthCheckDefinition is used to store the details about
// a health check's execution.
type HealthCheckDefinition struct {
	HTTP                           string
	Header                         map[string][]string
	Method                         string
	TLSSkipVerify                  bool
	TCP                            string
	Interval                       ReadableDuration
	Timeout                        ReadableDuration
	DeregisterCriticalServiceAfter ReadableDuration
}

// HealthChecks is a collection of HealthCheck structs.
type HealthChecks []*HealthCheck

// AggregatedStatus returns the "best" status for the list of health checks.
// Because a given entry may have many service and node-level health checks
// attached, this function determines the best representative of the status as
// as single string using the following heuristic:
//
//  maintenance > critical > warning > passing
//
func (c HealthChecks) AggregatedStatus() string {
	var passing, warning, critical, maintenance bool
	for _, check := range c {
		id := string(check.CheckID)
		if id == NodeMaint || strings.HasPrefix(id, ServiceMaintPrefix) {
			maintenance = true
			continue
		}

		switch check.Status {
		case HealthPassing:
			passing = true
		case HealthWarning:
			warning = true
		case HealthCritical:
			critical = true
		default:
			return ""
		}
	}

	switch {
	case maintenance:
		return HealthMaint
	case critical:
		return HealthCritical
	case warning:
		return HealthWarning
	case passing:
		return HealthPassing
	default:
		return HealthPassing
	}
}

// ServiceEntry is used for the health service endpoint
type ServiceEntry struct {
	Node    *Node
	Service *AgentService
	Checks  HealthChecks
}

// Health can be used to query the Health endpoints
type Health struct {
	c *Client
}

// Health returns a handle to the health endpoints
func (c *Client) Health() *Health {
	return &Health{c}
}

// Node is used to query for checks belonging to a given node
func (h *Health) Node(node string, q *QueryOptions) (HealthChecks, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/health/node/"+node)
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(h.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out HealthChecks
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return out, qm, nil
}

// Checks is used to return the checks associated with a service
func (h *Health) Checks(service string, q *QueryOptions) (HealthChecks, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/health/checks/"+service)
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(h.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out HealthChecks
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return out, qm, nil
}

// Service is used to query health information along with service info
// for a given service. It can optionally do server-side filtering on a tag
// or nodes with passing health checks only.
func (h *Health) Service(service, tag string, passingOnly bool, q *QueryOptions) ([]*ServiceEntry, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/health/service/"+service)
	r.setQueryOptions(q)
	if tag != "" {
		r.params.Set("tag", tag)
	}
	if passingOnly {
		r.params.Set(HealthPassing, "1")
	}
	rtt, resp, err := requireOK(h.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out []*ServiceEntry
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return out, qm, nil
}

// State is used to retrieve all the checks in a given state.
// The wildcard "any" state can also be used for all checks.
func (h *Health) State(state string, q *QueryOptions) (HealthChecks, *QueryMeta, error) {
	switch state {
	case HealthAny:
	case HealthWarning:
	case HealthCritical:
	case HealthPassing:
	default:
		return nil, nil, fmt.Errorf("Unsupported state: %v", state)
	}
	r := h.c.newRequest("GET", "/v1/health/state/"+state)
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(h.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out HealthChecks
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return out, qm, nil
}
