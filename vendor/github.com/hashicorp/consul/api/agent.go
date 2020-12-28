package api

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ServiceKind is the kind of service being registered.
type ServiceKind string

const (
	// ServiceKindTypical is a typical, classic Consul service. This is
	// represented by the absence of a value. This was chosen for ease of
	// backwards compatibility: existing services in the catalog would
	// default to the typical service.
	ServiceKindTypical ServiceKind = ""

	// ServiceKindConnectProxy is a proxy for the Connect feature. This
	// service proxies another service within Consul and speaks the connect
	// protocol.
	ServiceKindConnectProxy ServiceKind = "connect-proxy"

	// ServiceKindMeshGateway is a Mesh Gateway for the Connect feature. This
	// service will proxy connections based off the SNI header set by other
	// connect proxies
	ServiceKindMeshGateway ServiceKind = "mesh-gateway"

	// ServiceKindTerminatingGateway is a Terminating Gateway for the Connect
	// feature. This service will proxy connections to services outside the mesh.
	ServiceKindTerminatingGateway ServiceKind = "terminating-gateway"

	// ServiceKindIngressGateway is an Ingress Gateway for the Connect feature.
	// This service will ingress connections based of configuration defined in
	// the ingress-gateway config entry.
	ServiceKindIngressGateway ServiceKind = "ingress-gateway"
)

// UpstreamDestType is the type of upstream discovery mechanism.
type UpstreamDestType string

const (
	// UpstreamDestTypeService discovers instances via healthy service lookup.
	UpstreamDestTypeService UpstreamDestType = "service"

	// UpstreamDestTypePreparedQuery discovers instances via prepared query
	// execution.
	UpstreamDestTypePreparedQuery UpstreamDestType = "prepared_query"
)

// AgentCheck represents a check known to the agent
type AgentCheck struct {
	Node        string
	CheckID     string
	Name        string
	Status      string
	Notes       string
	Output      string
	ServiceID   string
	ServiceName string
	Type        string
	Definition  HealthCheckDefinition
	Namespace   string `json:",omitempty"`
}

// AgentWeights represent optional weights for a service
type AgentWeights struct {
	Passing int
	Warning int
}

// AgentService represents a service known to the agent
type AgentService struct {
	Kind              ServiceKind `json:",omitempty"`
	ID                string
	Service           string
	Tags              []string
	Meta              map[string]string
	Port              int
	Address           string
	TaggedAddresses   map[string]ServiceAddress `json:",omitempty"`
	Weights           AgentWeights
	EnableTagOverride bool
	CreateIndex       uint64                          `json:",omitempty" bexpr:"-"`
	ModifyIndex       uint64                          `json:",omitempty" bexpr:"-"`
	ContentHash       string                          `json:",omitempty" bexpr:"-"`
	Proxy             *AgentServiceConnectProxyConfig `json:",omitempty"`
	Connect           *AgentServiceConnect            `json:",omitempty"`
	// NOTE: If we ever set the ContentHash outside of singular service lookup then we may need
	// to include the Namespace in the hash. When we do, then we are in for lots of fun with tests.
	// For now though, ignoring it works well enough.
	Namespace string `json:",omitempty" bexpr:"-" hash:"ignore"`
	// Datacenter is only ever returned and is ignored if presented.
	Datacenter string `json:",omitempty" bexpr:"-" hash:"ignore"`
}

// AgentServiceChecksInfo returns information about a Service and its checks
type AgentServiceChecksInfo struct {
	AggregatedStatus string
	Service          *AgentService
	Checks           HealthChecks
}

// AgentServiceConnect represents the Connect configuration of a service.
type AgentServiceConnect struct {
	Native         bool                      `json:",omitempty"`
	SidecarService *AgentServiceRegistration `json:",omitempty" bexpr:"-"`
}

// AgentServiceConnectProxyConfig is the proxy configuration in a connect-proxy
// ServiceDefinition or response.
type AgentServiceConnectProxyConfig struct {
	DestinationServiceName string                 `json:",omitempty"`
	DestinationServiceID   string                 `json:",omitempty"`
	LocalServiceAddress    string                 `json:",omitempty"`
	LocalServicePort       int                    `json:",omitempty"`
	Config                 map[string]interface{} `json:",omitempty" bexpr:"-"`
	Upstreams              []Upstream             `json:",omitempty"`
	MeshGateway            MeshGatewayConfig      `json:",omitempty"`
	Expose                 ExposeConfig           `json:",omitempty"`
}

const (
	// MemberTagKeyACLMode is the key used to indicate what ACL mode the agent is
	// operating in. The values of this key will be one of the MemberACLMode constants
	// with the key not being present indicating ACLModeUnknown.
	MemberTagKeyACLMode = "acls"

	// MemberTagRole is the key used to indicate that the member is a server or not.
	MemberTagKeyRole = "role"

	// MemberTagValueRoleServer is the value of the MemberTagKeyRole used to indicate
	// that the member represents a Consul server.
	MemberTagValueRoleServer = "consul"

	// MemberTagKeySegment is the key name of the tag used to indicate which network
	// segment this member is in.
	// Network Segments are a Consul Enterprise feature.
	MemberTagKeySegment = "segment"

	// MemberTagKeyBootstrap is the key name of the tag used to indicate whether this
	// agent was started with the "bootstrap" configuration enabled
	MemberTagKeyBootstrap = "bootstrap"
	// MemberTagValueBootstrap is the value of the MemberTagKeyBootstrap key when the
	// agent was started with the "bootstrap" configuration enabled.
	MemberTagValueBootstrap = "1"

	// MemberTagKeyBootstrapExpect is the key name of the tag used to indicate whether
	// this agent was started with the "bootstrap_expect" configuration set to a non-zero
	// value. The value of this key will be the string for of that configuration value.
	MemberTagKeyBootstrapExpect = "expect"

	// MemberTagKeyUseTLS is the key name of the tag used to indicate whther this agent
	// was configured to use TLS.
	MemberTagKeyUseTLS = "use_tls"
	// MemberTagValueUseTLS is the value of the MemberTagKeyUseTLS when the agent was
	// configured to use TLS. Any other value indicates that it was not setup in
	// that manner.
	MemberTagValueUseTLS = "1"

	// MemberTagKeyReadReplica is the key used to indicate that the member is a read
	// replica server (will remain a Raft non-voter).
	// Read Replicas are a Consul Enterprise feature.
	MemberTagKeyReadReplica = "read_replica"
	// MemberTagValueReadReplica is the value of the MemberTagKeyReadReplica key when
	// the member is in fact a read-replica. Any other value indicates that it is not.
	// Read Replicas are a Consul Enterprise feature.
	MemberTagValueReadReplica = "1"
)

type MemberACLMode string

const (
	// ACLModeDisables indicates that ACLs are disabled for this agent
	ACLModeDisabled MemberACLMode = "0"
	// ACLModeEnabled indicates that ACLs are enabled and operating in new ACL
	// mode (v1.4.0+ ACLs)
	ACLModeEnabled MemberACLMode = "1"
	// ACLModeLegacy indicates that ACLs are enabled and operating in legacy mode.
	ACLModeLegacy MemberACLMode = "2"
	// ACLModeUnkown is used to indicate that the AgentMember.Tags didn't advertise
	// an ACL mode at all. This is the case for Consul versions before v1.4.0 and
	// should be treated similarly to ACLModeLegacy.
	ACLModeUnknown MemberACLMode = "3"
)

// AgentMember represents a cluster member known to the agent
type AgentMember struct {
	Name string
	Addr string
	Port uint16
	Tags map[string]string
	// Status of the Member which corresponds to  github.com/hashicorp/serf/serf.MemberStatus
	// Value is one of:
	//
	// 	  AgentMemberNone    = 0
	//	  AgentMemberAlive   = 1
	//	  AgentMemberLeaving = 2
	//	  AgentMemberLeft    = 3
	//	  AgentMemberFailed  = 4
	Status      int
	ProtocolMin uint8
	ProtocolMax uint8
	ProtocolCur uint8
	DelegateMin uint8
	DelegateMax uint8
	DelegateCur uint8
}

// ACLMode returns the ACL mode this agent is operating in.
func (m *AgentMember) ACLMode() MemberACLMode {
	mode := m.Tags[MemberTagKeyACLMode]

	// the key may not have existed but then an
	// empty string will be returned and we will
	// handle that in the default case of the switch
	switch MemberACLMode(mode) {
	case ACLModeDisabled:
		return ACLModeDisabled
	case ACLModeEnabled:
		return ACLModeEnabled
	case ACLModeLegacy:
		return ACLModeLegacy
	default:
		return ACLModeUnknown
	}
}

// IsConsulServer returns true when this member is a Consul server.
func (m *AgentMember) IsConsulServer() bool {
	return m.Tags[MemberTagKeyRole] == MemberTagValueRoleServer
}

// AllSegments is used to select for all segments in MembersOpts.
const AllSegments = "_all"

// MembersOpts is used for querying member information.
type MembersOpts struct {
	// WAN is whether to show members from the WAN.
	WAN bool

	// Segment is the LAN segment to show members for. Setting this to the
	// AllSegments value above will show members in all segments.
	Segment string
}

// AgentServiceRegistration is used to register a new service
type AgentServiceRegistration struct {
	Kind              ServiceKind               `json:",omitempty"`
	ID                string                    `json:",omitempty"`
	Name              string                    `json:",omitempty"`
	Tags              []string                  `json:",omitempty"`
	Port              int                       `json:",omitempty"`
	Address           string                    `json:",omitempty"`
	TaggedAddresses   map[string]ServiceAddress `json:",omitempty"`
	EnableTagOverride bool                      `json:",omitempty"`
	Meta              map[string]string         `json:",omitempty"`
	Weights           *AgentWeights             `json:",omitempty"`
	Check             *AgentServiceCheck
	Checks            AgentServiceChecks
	Proxy             *AgentServiceConnectProxyConfig `json:",omitempty"`
	Connect           *AgentServiceConnect            `json:",omitempty"`
	Namespace         string                          `json:",omitempty" bexpr:"-" hash:"ignore"`
}

//ServiceRegisterOpts is used to pass extra options to the service register.
type ServiceRegisterOpts struct {
	//Missing healthchecks will be deleted from the agent.
	//Using this parameter allows to idempotently register a service and its checks without
	//having to manually deregister checks.
	ReplaceExistingChecks bool
}

// AgentCheckRegistration is used to register a new check
type AgentCheckRegistration struct {
	ID        string `json:",omitempty"`
	Name      string `json:",omitempty"`
	Notes     string `json:",omitempty"`
	ServiceID string `json:",omitempty"`
	AgentServiceCheck
	Namespace string `json:",omitempty"`
}

// AgentServiceCheck is used to define a node or service level check
type AgentServiceCheck struct {
	CheckID                string              `json:",omitempty"`
	Name                   string              `json:",omitempty"`
	Args                   []string            `json:"ScriptArgs,omitempty"`
	DockerContainerID      string              `json:",omitempty"`
	Shell                  string              `json:",omitempty"` // Only supported for Docker.
	Interval               string              `json:",omitempty"`
	Timeout                string              `json:",omitempty"`
	TTL                    string              `json:",omitempty"`
	HTTP                   string              `json:",omitempty"`
	Header                 map[string][]string `json:",omitempty"`
	Method                 string              `json:",omitempty"`
	Body                   string              `json:",omitempty"`
	TCP                    string              `json:",omitempty"`
	Status                 string              `json:",omitempty"`
	Notes                  string              `json:",omitempty"`
	TLSSkipVerify          bool                `json:",omitempty"`
	GRPC                   string              `json:",omitempty"`
	GRPCUseTLS             bool                `json:",omitempty"`
	AliasNode              string              `json:",omitempty"`
	AliasService           string              `json:",omitempty"`
	SuccessBeforePassing   int                 `json:",omitempty"`
	FailuresBeforeCritical int                 `json:",omitempty"`

	// In Consul 0.7 and later, checks that are associated with a service
	// may also contain this optional DeregisterCriticalServiceAfter field,
	// which is a timeout in the same Go time format as Interval and TTL. If
	// a check is in the critical state for more than this configured value,
	// then its associated service (and all of its associated checks) will
	// automatically be deregistered.
	DeregisterCriticalServiceAfter string `json:",omitempty"`
}
type AgentServiceChecks []*AgentServiceCheck

// AgentToken is used when updating ACL tokens for an agent.
type AgentToken struct {
	Token string
}

// Metrics info is used to store different types of metric values from the agent.
type MetricsInfo struct {
	Timestamp string
	Gauges    []GaugeValue
	Points    []PointValue
	Counters  []SampledValue
	Samples   []SampledValue
}

// GaugeValue stores one value that is updated as time goes on, such as
// the amount of memory allocated.
type GaugeValue struct {
	Name   string
	Value  float32
	Labels map[string]string
}

// PointValue holds a series of points for a metric.
type PointValue struct {
	Name   string
	Points []float32
}

// SampledValue stores info about a metric that is incremented over time,
// such as the number of requests to an HTTP endpoint.
type SampledValue struct {
	Name   string
	Count  int
	Sum    float64
	Min    float64
	Max    float64
	Mean   float64
	Stddev float64
	Labels map[string]string
}

// AgentAuthorizeParams are the request parameters for authorizing a request.
type AgentAuthorizeParams struct {
	Target           string
	ClientCertURI    string
	ClientCertSerial string
}

// AgentAuthorize is the response structure for Connect authorization.
type AgentAuthorize struct {
	Authorized bool
	Reason     string
}

// ConnectProxyConfig is the response structure for agent-local proxy
// configuration.
type ConnectProxyConfig struct {
	ProxyServiceID    string
	TargetServiceID   string
	TargetServiceName string
	ContentHash       string
	Config            map[string]interface{} `bexpr:"-"`
	Upstreams         []Upstream
}

// Upstream is the response structure for a proxy upstream configuration.
type Upstream struct {
	DestinationType      UpstreamDestType `json:",omitempty"`
	DestinationNamespace string           `json:",omitempty"`
	DestinationName      string
	Datacenter           string                 `json:",omitempty"`
	LocalBindAddress     string                 `json:",omitempty"`
	LocalBindPort        int                    `json:",omitempty"`
	Config               map[string]interface{} `json:",omitempty" bexpr:"-"`
	MeshGateway          MeshGatewayConfig      `json:",omitempty"`
}

// Agent can be used to query the Agent endpoints
type Agent struct {
	c *Client

	// cache the node name
	nodeName string
}

// Agent returns a handle to the agent endpoints
func (c *Client) Agent() *Agent {
	return &Agent{c: c}
}

// Self is used to query the agent we are speaking to for
// information about itself
func (a *Agent) Self() (map[string]map[string]interface{}, error) {
	r := a.c.newRequest("GET", "/v1/agent/self")
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out map[string]map[string]interface{}
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Host is used to retrieve information about the host the
// agent is running on such as CPU, memory, and disk. Requires
// a operator:read ACL token.
func (a *Agent) Host() (map[string]interface{}, error) {
	r := a.c.newRequest("GET", "/v1/agent/host")
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out map[string]interface{}
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Metrics is used to query the agent we are speaking to for
// its current internal metric data
func (a *Agent) Metrics() (*MetricsInfo, error) {
	r := a.c.newRequest("GET", "/v1/agent/metrics")
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out *MetricsInfo
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Reload triggers a configuration reload for the agent we are connected to.
func (a *Agent) Reload() error {
	r := a.c.newRequest("PUT", "/v1/agent/reload")
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// NodeName is used to get the node name of the agent
func (a *Agent) NodeName() (string, error) {
	if a.nodeName != "" {
		return a.nodeName, nil
	}
	info, err := a.Self()
	if err != nil {
		return "", err
	}
	name := info["Config"]["NodeName"].(string)
	a.nodeName = name
	return name, nil
}

// Checks returns the locally registered checks
func (a *Agent) Checks() (map[string]*AgentCheck, error) {
	return a.ChecksWithFilter("")
}

// ChecksWithFilter returns a subset of the locally registered checks that match
// the given filter expression
func (a *Agent) ChecksWithFilter(filter string) (map[string]*AgentCheck, error) {
	r := a.c.newRequest("GET", "/v1/agent/checks")
	r.filterQuery(filter)
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out map[string]*AgentCheck
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Services returns the locally registered services
func (a *Agent) Services() (map[string]*AgentService, error) {
	return a.ServicesWithFilter("")
}

// ServicesWithFilter returns a subset of the locally registered services that match
// the given filter expression
func (a *Agent) ServicesWithFilter(filter string) (map[string]*AgentService, error) {
	r := a.c.newRequest("GET", "/v1/agent/services")
	r.filterQuery(filter)
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out map[string]*AgentService
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// AgentHealthServiceByID returns for a given serviceID: the aggregated health status, the service definition or an error if any
// - If the service is not found, will return status (critical, nil, nil)
// - If the service is found, will return (critical|passing|warning), AgentServiceChecksInfo, nil)
// - In all other cases, will return an error
func (a *Agent) AgentHealthServiceByID(serviceID string) (string, *AgentServiceChecksInfo, error) {
	path := fmt.Sprintf("/v1/agent/health/service/id/%v", url.PathEscape(serviceID))
	r := a.c.newRequest("GET", path)
	r.params.Add("format", "json")
	r.header.Set("Accept", "application/json")
	_, resp, err := a.c.doRequest(r)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	// Service not Found
	if resp.StatusCode == http.StatusNotFound {
		return HealthCritical, nil, nil
	}
	var out *AgentServiceChecksInfo
	if err := decodeBody(resp, &out); err != nil {
		return HealthCritical, out, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return HealthPassing, out, nil
	case http.StatusTooManyRequests:
		return HealthWarning, out, nil
	case http.StatusServiceUnavailable:
		return HealthCritical, out, nil
	}
	return HealthCritical, out, fmt.Errorf("Unexpected Error Code %v for %s", resp.StatusCode, path)
}

// AgentHealthServiceByName returns for a given service name: the aggregated health status for all services
// having the specified name.
// - If no service is not found, will return status (critical, [], nil)
// - If the service is found, will return (critical|passing|warning), []api.AgentServiceChecksInfo, nil)
// - In all other cases, will return an error
func (a *Agent) AgentHealthServiceByName(service string) (string, []AgentServiceChecksInfo, error) {
	path := fmt.Sprintf("/v1/agent/health/service/name/%v", url.PathEscape(service))
	r := a.c.newRequest("GET", path)
	r.params.Add("format", "json")
	r.header.Set("Accept", "application/json")
	_, resp, err := a.c.doRequest(r)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	// Service not Found
	if resp.StatusCode == http.StatusNotFound {
		return HealthCritical, nil, nil
	}
	var out []AgentServiceChecksInfo
	if err := decodeBody(resp, &out); err != nil {
		return HealthCritical, out, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return HealthPassing, out, nil
	case http.StatusTooManyRequests:
		return HealthWarning, out, nil
	case http.StatusServiceUnavailable:
		return HealthCritical, out, nil
	}
	return HealthCritical, out, fmt.Errorf("Unexpected Error Code %v for %s", resp.StatusCode, path)
}

// Service returns a locally registered service instance and allows for
// hash-based blocking.
//
// Note that this uses an unconventional blocking mechanism since it's
// agent-local state. That means there is no persistent raft index so we block
// based on object hash instead.
func (a *Agent) Service(serviceID string, q *QueryOptions) (*AgentService, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/agent/service/"+serviceID)
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out *AgentService
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}

	return out, qm, nil
}

// Members returns the known gossip members. The WAN
// flag can be used to query a server for WAN members.
func (a *Agent) Members(wan bool) ([]*AgentMember, error) {
	r := a.c.newRequest("GET", "/v1/agent/members")
	if wan {
		r.params.Set("wan", "1")
	}
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out []*AgentMember
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// MembersOpts returns the known gossip members and can be passed
// additional options for WAN/segment filtering.
func (a *Agent) MembersOpts(opts MembersOpts) ([]*AgentMember, error) {
	r := a.c.newRequest("GET", "/v1/agent/members")
	r.params.Set("segment", opts.Segment)
	if opts.WAN {
		r.params.Set("wan", "1")
	}

	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out []*AgentMember
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ServiceRegister is used to register a new service with
// the local agent
func (a *Agent) ServiceRegister(service *AgentServiceRegistration) error {
	opts := ServiceRegisterOpts{
		ReplaceExistingChecks: false,
	}

	return a.serviceRegister(service, opts)
}

// ServiceRegister is used to register a new service with
// the local agent and can be passed additional options.
func (a *Agent) ServiceRegisterOpts(service *AgentServiceRegistration, opts ServiceRegisterOpts) error {
	return a.serviceRegister(service, opts)
}

func (a *Agent) serviceRegister(service *AgentServiceRegistration, opts ServiceRegisterOpts) error {
	r := a.c.newRequest("PUT", "/v1/agent/service/register")
	r.obj = service
	if opts.ReplaceExistingChecks {
		r.params.Set("replace-existing-checks", "true")
	}
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ServiceDeregister is used to deregister a service with
// the local agent
func (a *Agent) ServiceDeregister(serviceID string) error {
	r := a.c.newRequest("PUT", "/v1/agent/service/deregister/"+serviceID)
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// PassTTL is used to set a TTL check to the passing state.
//
// DEPRECATION NOTICE: This interface is deprecated in favor of UpdateTTL().
// The client interface will be removed in 0.8 or changed to use
// UpdateTTL()'s endpoint and the server endpoints will be removed in 0.9.
func (a *Agent) PassTTL(checkID, note string) error {
	return a.updateTTL(checkID, note, "pass")
}

// WarnTTL is used to set a TTL check to the warning state.
//
// DEPRECATION NOTICE: This interface is deprecated in favor of UpdateTTL().
// The client interface will be removed in 0.8 or changed to use
// UpdateTTL()'s endpoint and the server endpoints will be removed in 0.9.
func (a *Agent) WarnTTL(checkID, note string) error {
	return a.updateTTL(checkID, note, "warn")
}

// FailTTL is used to set a TTL check to the failing state.
//
// DEPRECATION NOTICE: This interface is deprecated in favor of UpdateTTL().
// The client interface will be removed in 0.8 or changed to use
// UpdateTTL()'s endpoint and the server endpoints will be removed in 0.9.
func (a *Agent) FailTTL(checkID, note string) error {
	return a.updateTTL(checkID, note, "fail")
}

// updateTTL is used to update the TTL of a check. This is the internal
// method that uses the old API that's present in Consul versions prior to
// 0.6.4. Since Consul didn't have an analogous "update" API before it seemed
// ok to break this (former) UpdateTTL in favor of the new UpdateTTL below,
// but keep the old Pass/Warn/Fail methods using the old API under the hood.
//
// DEPRECATION NOTICE: This interface is deprecated in favor of UpdateTTL().
// The client interface will be removed in 0.8 and the server endpoints will
// be removed in 0.9.
func (a *Agent) updateTTL(checkID, note, status string) error {
	switch status {
	case "pass":
	case "warn":
	case "fail":
	default:
		return fmt.Errorf("Invalid status: %s", status)
	}
	endpoint := fmt.Sprintf("/v1/agent/check/%s/%s", status, checkID)
	r := a.c.newRequest("PUT", endpoint)
	r.params.Set("note", note)
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// checkUpdate is the payload for a PUT for a check update.
type checkUpdate struct {
	// Status is one of the api.Health* states: HealthPassing
	// ("passing"), HealthWarning ("warning"), or HealthCritical
	// ("critical").
	Status string

	// Output is the information to post to the UI for operators as the
	// output of the process that decided to hit the TTL check. This is
	// different from the note field that's associated with the check
	// itself.
	Output string
}

// UpdateTTL is used to update the TTL of a check. This uses the newer API
// that was introduced in Consul 0.6.4 and later. We translate the old status
// strings for compatibility (though a newer version of Consul will still be
// required to use this API).
func (a *Agent) UpdateTTL(checkID, output, status string) error {
	switch status {
	case "pass", HealthPassing:
		status = HealthPassing
	case "warn", HealthWarning:
		status = HealthWarning
	case "fail", HealthCritical:
		status = HealthCritical
	default:
		return fmt.Errorf("Invalid status: %s", status)
	}

	endpoint := fmt.Sprintf("/v1/agent/check/update/%s", checkID)
	r := a.c.newRequest("PUT", endpoint)
	r.obj = &checkUpdate{
		Status: status,
		Output: output,
	}

	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// CheckRegister is used to register a new check with
// the local agent
func (a *Agent) CheckRegister(check *AgentCheckRegistration) error {
	r := a.c.newRequest("PUT", "/v1/agent/check/register")
	r.obj = check
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// CheckDeregister is used to deregister a check with
// the local agent
func (a *Agent) CheckDeregister(checkID string) error {
	r := a.c.newRequest("PUT", "/v1/agent/check/deregister/"+checkID)
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Join is used to instruct the agent to attempt a join to
// another cluster member
func (a *Agent) Join(addr string, wan bool) error {
	r := a.c.newRequest("PUT", "/v1/agent/join/"+addr)
	if wan {
		r.params.Set("wan", "1")
	}
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Leave is used to have the agent gracefully leave the cluster and shutdown
func (a *Agent) Leave() error {
	r := a.c.newRequest("PUT", "/v1/agent/leave")
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ForceLeave is used to have the agent eject a failed node
func (a *Agent) ForceLeave(node string) error {
	r := a.c.newRequest("PUT", "/v1/agent/force-leave/"+node)
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

//ForceLeavePrune is used to have an a failed agent removed
//from the list of members
func (a *Agent) ForceLeavePrune(node string) error {
	r := a.c.newRequest("PUT", "/v1/agent/force-leave/"+node)
	r.params.Set("prune", "1")
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ConnectAuthorize is used to authorize an incoming connection
// to a natively integrated Connect service.
func (a *Agent) ConnectAuthorize(auth *AgentAuthorizeParams) (*AgentAuthorize, error) {
	r := a.c.newRequest("POST", "/v1/agent/connect/authorize")
	r.obj = auth
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out AgentAuthorize
	if err := decodeBody(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ConnectCARoots returns the list of roots.
func (a *Agent) ConnectCARoots(q *QueryOptions) (*CARootList, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/agent/connect/ca/roots")
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out CARootList
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return &out, qm, nil
}

// ConnectCALeaf gets the leaf certificate for the given service ID.
func (a *Agent) ConnectCALeaf(serviceID string, q *QueryOptions) (*LeafCert, *QueryMeta, error) {
	r := a.c.newRequest("GET", "/v1/agent/connect/ca/leaf/"+serviceID)
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out LeafCert
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return &out, qm, nil
}

// EnableServiceMaintenance toggles service maintenance mode on
// for the given service ID.
func (a *Agent) EnableServiceMaintenance(serviceID, reason string) error {
	r := a.c.newRequest("PUT", "/v1/agent/service/maintenance/"+serviceID)
	r.params.Set("enable", "true")
	r.params.Set("reason", reason)
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DisableServiceMaintenance toggles service maintenance mode off
// for the given service ID.
func (a *Agent) DisableServiceMaintenance(serviceID string) error {
	r := a.c.newRequest("PUT", "/v1/agent/service/maintenance/"+serviceID)
	r.params.Set("enable", "false")
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// EnableNodeMaintenance toggles node maintenance mode on for the
// agent we are connected to.
func (a *Agent) EnableNodeMaintenance(reason string) error {
	r := a.c.newRequest("PUT", "/v1/agent/maintenance")
	r.params.Set("enable", "true")
	r.params.Set("reason", reason)
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DisableNodeMaintenance toggles node maintenance mode off for the
// agent we are connected to.
func (a *Agent) DisableNodeMaintenance() error {
	r := a.c.newRequest("PUT", "/v1/agent/maintenance")
	r.params.Set("enable", "false")
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Monitor returns a channel which will receive streaming logs from the agent
// Providing a non-nil stopCh can be used to close the connection and stop the
// log stream. An empty string will be sent down the given channel when there's
// nothing left to stream, after which the caller should close the stopCh.
func (a *Agent) Monitor(loglevel string, stopCh <-chan struct{}, q *QueryOptions) (chan string, error) {
	return a.monitor(loglevel, false, stopCh, q)
}

// MonitorJSON is like Monitor except it returns logs in JSON format.
func (a *Agent) MonitorJSON(loglevel string, stopCh <-chan struct{}, q *QueryOptions) (chan string, error) {
	return a.monitor(loglevel, true, stopCh, q)
}
func (a *Agent) monitor(loglevel string, logJSON bool, stopCh <-chan struct{}, q *QueryOptions) (chan string, error) {
	r := a.c.newRequest("GET", "/v1/agent/monitor")
	r.setQueryOptions(q)
	if loglevel != "" {
		r.params.Add("loglevel", loglevel)
	}
	if logJSON {
		r.params.Set("logjson", "true")
	}
	_, resp, err := requireOK(a.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	logCh := make(chan string, 64)
	go func() {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for {
			select {
			case <-stopCh:
				close(logCh)
				return
			default:
			}
			if scanner.Scan() {
				// An empty string signals to the caller that
				// the scan is done, so make sure we only emit
				// that when the scanner says it's done, not if
				// we happen to ingest an empty line.
				if text := scanner.Text(); text != "" {
					logCh <- text
				} else {
					logCh <- " "
				}
			} else {
				logCh <- ""
			}
		}
	}()
	return logCh, nil
}

// UpdateACLToken updates the agent's "acl_token". See updateToken for more
// details.
//
// DEPRECATED (ACL-Legacy-Compat) - Prefer UpdateDefaultACLToken for v1.4.3 and above
func (a *Agent) UpdateACLToken(token string, q *WriteOptions) (*WriteMeta, error) {
	return a.updateToken("acl_token", token, q)
}

// UpdateACLAgentToken updates the agent's "acl_agent_token". See updateToken
// for more details.
//
// DEPRECATED (ACL-Legacy-Compat) - Prefer UpdateAgentACLToken for v1.4.3 and above
func (a *Agent) UpdateACLAgentToken(token string, q *WriteOptions) (*WriteMeta, error) {
	return a.updateToken("acl_agent_token", token, q)
}

// UpdateACLAgentMasterToken updates the agent's "acl_agent_master_token". See
// updateToken for more details.
//
// DEPRECATED (ACL-Legacy-Compat) - Prefer UpdateAgentMasterACLToken for v1.4.3 and above
func (a *Agent) UpdateACLAgentMasterToken(token string, q *WriteOptions) (*WriteMeta, error) {
	return a.updateToken("acl_agent_master_token", token, q)
}

// UpdateACLReplicationToken updates the agent's "acl_replication_token". See
// updateToken for more details.
//
// DEPRECATED (ACL-Legacy-Compat) - Prefer UpdateReplicationACLToken for v1.4.3 and above
func (a *Agent) UpdateACLReplicationToken(token string, q *WriteOptions) (*WriteMeta, error) {
	return a.updateToken("acl_replication_token", token, q)
}

// UpdateDefaultACLToken updates the agent's "default" token. See updateToken
// for more details
func (a *Agent) UpdateDefaultACLToken(token string, q *WriteOptions) (*WriteMeta, error) {
	return a.updateTokenFallback("default", "acl_token", token, q)
}

// UpdateAgentACLToken updates the agent's "agent" token. See updateToken
// for more details
func (a *Agent) UpdateAgentACLToken(token string, q *WriteOptions) (*WriteMeta, error) {
	return a.updateTokenFallback("agent", "acl_agent_token", token, q)
}

// UpdateAgentMasterACLToken updates the agent's "agent_master" token. See updateToken
// for more details
func (a *Agent) UpdateAgentMasterACLToken(token string, q *WriteOptions) (*WriteMeta, error) {
	return a.updateTokenFallback("agent_master", "acl_agent_master_token", token, q)
}

// UpdateReplicationACLToken updates the agent's "replication" token. See updateToken
// for more details
func (a *Agent) UpdateReplicationACLToken(token string, q *WriteOptions) (*WriteMeta, error) {
	return a.updateTokenFallback("replication", "acl_replication_token", token, q)
}

// updateToken can be used to update one of an agent's ACL tokens after the agent has
// started. The tokens are may not be persisted, so will need to be updated again if
// the agent is restarted unless the agent is configured to persist them.
func (a *Agent) updateToken(target, token string, q *WriteOptions) (*WriteMeta, error) {
	meta, _, err := a.updateTokenOnce(target, token, q)
	return meta, err
}

func (a *Agent) updateTokenFallback(target, fallback, token string, q *WriteOptions) (*WriteMeta, error) {
	meta, status, err := a.updateTokenOnce(target, token, q)
	if err != nil && status == 404 {
		meta, _, err = a.updateTokenOnce(fallback, token, q)
	}
	return meta, err
}

func (a *Agent) updateTokenOnce(target, token string, q *WriteOptions) (*WriteMeta, int, error) {
	r := a.c.newRequest("PUT", fmt.Sprintf("/v1/agent/token/%s", target))
	r.setWriteOptions(q)
	r.obj = &AgentToken{Token: token}

	rtt, resp, err := a.c.doRequest(r)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{RequestTime: rtt}

	if resp.StatusCode != 200 {
		var buf bytes.Buffer
		io.Copy(&buf, resp.Body)
		return wm, resp.StatusCode, fmt.Errorf("Unexpected response code: %d (%s)", resp.StatusCode, buf.Bytes())
	}

	return wm, resp.StatusCode, nil
}
