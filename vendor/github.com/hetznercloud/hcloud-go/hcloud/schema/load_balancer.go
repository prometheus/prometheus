package schema

import "time"

type LoadBalancer struct {
	ID               int                      `json:"id"`
	Name             string                   `json:"name"`
	PublicNet        LoadBalancerPublicNet    `json:"public_net"`
	PrivateNet       []LoadBalancerPrivateNet `json:"private_net"`
	Location         Location                 `json:"location"`
	LoadBalancerType LoadBalancerType         `json:"load_balancer_type"`
	Protection       LoadBalancerProtection   `json:"protection"`
	Labels           map[string]string        `json:"labels"`
	Created          time.Time                `json:"created"`
	Services         []LoadBalancerService    `json:"services"`
	Targets          []LoadBalancerTarget     `json:"targets"`
	Algorithm        LoadBalancerAlgorithm    `json:"algorithm"`
	IncludedTraffic  uint64                   `json:"included_traffic"`
	OutgoingTraffic  *uint64                  `json:"outgoing_traffic"`
	IngoingTraffic   *uint64                  `json:"ingoing_traffic"`
}

type LoadBalancerPublicNet struct {
	Enabled bool                      `json:"enabled"`
	IPv4    LoadBalancerPublicNetIPv4 `json:"ipv4"`
	IPv6    LoadBalancerPublicNetIPv6 `json:"ipv6"`
}

type LoadBalancerPublicNetIPv4 struct {
	IP string `json:"ip"`
}

type LoadBalancerPublicNetIPv6 struct {
	IP string `json:"ip"`
}

type LoadBalancerPrivateNet struct {
	Network int    `json:"network"`
	IP      string `json:"ip"`
}

type LoadBalancerAlgorithm struct {
	Type string `json:"type"`
}

type LoadBalancerProtection struct {
	Delete bool `json:"delete"`
}

type LoadBalancerService struct {
	Protocol        string                          `json:"protocol"`
	ListenPort      int                             `json:"listen_port"`
	DestinationPort int                             `json:"destination_port"`
	Proxyprotocol   bool                            `json:"proxyprotocol"`
	HTTP            *LoadBalancerServiceHTTP        `json:"http"`
	HealthCheck     *LoadBalancerServiceHealthCheck `json:"health_check"`
}

type LoadBalancerServiceHTTP struct {
	CookieName     string `json:"cookie_name"`
	CookieLifetime int    `json:"cookie_lifetime"`
	Certificates   []int  `json:"certificates"`
	RedirectHTTP   bool   `json:"redirect_http"`
	StickySessions bool   `json:"sticky_sessions"`
}

type LoadBalancerServiceHealthCheck struct {
	Protocol string                              `json:"protocol"`
	Port     int                                 `json:"port"`
	Interval int                                 `json:"interval"`
	Timeout  int                                 `json:"timeout"`
	Retries  int                                 `json:"retries"`
	HTTP     *LoadBalancerServiceHealthCheckHTTP `json:"http"`
}

type LoadBalancerServiceHealthCheckHTTP struct {
	Domain      string   `json:"domain"`
	Path        string   `json:"path"`
	Response    string   `json:"response"`
	StatusCodes []string `json:"status_codes"`
	TLS         bool     `json:"tls"`
}

type LoadBalancerTarget struct {
	Type          string                           `json:"type"`
	Server        *LoadBalancerTargetServer        `json:"server"`
	LabelSelector *LoadBalancerTargetLabelSelector `json:"label_selector"`
	IP            *LoadBalancerTargetIP            `json:"ip"`
	HealthStatus  []LoadBalancerTargetHealthStatus `json:"health_status"`
	UsePrivateIP  bool                             `json:"use_private_ip"`
	Targets       []LoadBalancerTarget             `json:"targets,omitempty"`
}

type LoadBalancerTargetHealthStatus struct {
	ListenPort int    `json:"listen_port"`
	Status     string `json:"status"`
}

type LoadBalancerTargetServer struct {
	ID int `json:"id"`
}

type LoadBalancerTargetLabelSelector struct {
	Selector string `json:"selector"`
}

type LoadBalancerTargetIP struct {
	IP string `json:"ip"`
}

type LoadBalancerListResponse struct {
	LoadBalancers []LoadBalancer `json:"load_balancers"`
}

type LoadBalancerGetResponse struct {
	LoadBalancer LoadBalancer `json:"load_balancer"`
}

type LoadBalancerActionAddTargetRequest struct {
	Type          string                                           `json:"type"`
	Server        *LoadBalancerActionAddTargetRequestServer        `json:"server,omitempty"`
	LabelSelector *LoadBalancerActionAddTargetRequestLabelSelector `json:"label_selector,omitempty"`
	IP            *LoadBalancerActionAddTargetRequestIP            `json:"ip,omitempty"`
	UsePrivateIP  *bool                                            `json:"use_private_ip,omitempty"`
}

type LoadBalancerActionAddTargetRequestServer struct {
	ID int `json:"id"`
}

type LoadBalancerActionAddTargetRequestLabelSelector struct {
	Selector string `json:"selector"`
}

type LoadBalancerActionAddTargetRequestIP struct {
	IP string `json:"ip"`
}

type LoadBalancerActionAddTargetResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerActionRemoveTargetRequest struct {
	Type          string                                              `json:"type"`
	Server        *LoadBalancerActionRemoveTargetRequestServer        `json:"server,omitempty"`
	LabelSelector *LoadBalancerActionRemoveTargetRequestLabelSelector `json:"label_selector,omitempty"`
	IP            *LoadBalancerActionRemoveTargetRequestIP            `json:"ip,omitempty"`
}

type LoadBalancerActionRemoveTargetRequestServer struct {
	ID int `json:"id"`
}

type LoadBalancerActionRemoveTargetRequestLabelSelector struct {
	Selector string `json:"selector"`
}

type LoadBalancerActionRemoveTargetRequestIP struct {
	IP string `json:"ip"`
}

type LoadBalancerActionRemoveTargetResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerActionAddServiceRequest struct {
	Protocol        string                                          `json:"protocol"`
	ListenPort      *int                                            `json:"listen_port,omitempty"`
	DestinationPort *int                                            `json:"destination_port,omitempty"`
	Proxyprotocol   *bool                                           `json:"proxyprotocol,omitempty"`
	HTTP            *LoadBalancerActionAddServiceRequestHTTP        `json:"http,omitempty"`
	HealthCheck     *LoadBalancerActionAddServiceRequestHealthCheck `json:"health_check,omitempty"`
}

type LoadBalancerActionAddServiceRequestHTTP struct {
	CookieName     *string `json:"cookie_name,omitempty"`
	CookieLifetime *int    `json:"cookie_lifetime,omitempty"`
	Certificates   *[]int  `json:"certificates,omitempty"`
	RedirectHTTP   *bool   `json:"redirect_http,omitempty"`
	StickySessions *bool   `json:"sticky_sessions,omitempty"`
}

type LoadBalancerActionAddServiceRequestHealthCheck struct {
	Protocol string                                              `json:"protocol"`
	Port     *int                                                `json:"port,omitempty"`
	Interval *int                                                `json:"interval,omitempty"`
	Timeout  *int                                                `json:"timeout,omitempty"`
	Retries  *int                                                `json:"retries,omitempty"`
	HTTP     *LoadBalancerActionAddServiceRequestHealthCheckHTTP `json:"http,omitempty"`
}

type LoadBalancerActionAddServiceRequestHealthCheckHTTP struct {
	Domain      *string   `json:"domain,omitempty"`
	Path        *string   `json:"path,omitempty"`
	Response    *string   `json:"response,omitempty"`
	StatusCodes *[]string `json:"status_codes,omitempty"`
	TLS         *bool     `json:"tls,omitempty"`
}

type LoadBalancerActionAddServiceResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerActionUpdateServiceRequest struct {
	ListenPort      int                                                `json:"listen_port"`
	Protocol        *string                                            `json:"protocol,omitempty"`
	DestinationPort *int                                               `json:"destination_port,omitempty"`
	Proxyprotocol   *bool                                              `json:"proxyprotocol,omitempty"`
	HTTP            *LoadBalancerActionUpdateServiceRequestHTTP        `json:"http,omitempty"`
	HealthCheck     *LoadBalancerActionUpdateServiceRequestHealthCheck `json:"health_check,omitempty"`
}

type LoadBalancerActionUpdateServiceRequestHTTP struct {
	CookieName     *string `json:"cookie_name,omitempty"`
	CookieLifetime *int    `json:"cookie_lifetime,omitempty"`
	Certificates   *[]int  `json:"certificates,omitempty"`
	RedirectHTTP   *bool   `json:"redirect_http,omitempty"`
	StickySessions *bool   `json:"sticky_sessions,omitempty"`
}

type LoadBalancerActionUpdateServiceRequestHealthCheck struct {
	Protocol *string                                                `json:"protocol,omitempty"`
	Port     *int                                                   `json:"port,omitempty"`
	Interval *int                                                   `json:"interval,omitempty"`
	Timeout  *int                                                   `json:"timeout,omitempty"`
	Retries  *int                                                   `json:"retries,omitempty"`
	HTTP     *LoadBalancerActionUpdateServiceRequestHealthCheckHTTP `json:"http,omitempty"`
}

type LoadBalancerActionUpdateServiceRequestHealthCheckHTTP struct {
	Domain      *string   `json:"domain,omitempty"`
	Path        *string   `json:"path,omitempty"`
	Response    *string   `json:"response,omitempty"`
	StatusCodes *[]string `json:"status_codes,omitempty"`
	TLS         *bool     `json:"tls,omitempty"`
}

type LoadBalancerActionUpdateServiceResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerDeleteServiceRequest struct {
	ListenPort int `json:"listen_port"`
}

type LoadBalancerDeleteServiceResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerCreateRequest struct {
	Name             string                              `json:"name"`
	LoadBalancerType interface{}                         `json:"load_balancer_type"` // int or string
	Algorithm        *LoadBalancerCreateRequestAlgorithm `json:"algorithm,omitempty"`
	Location         *string                             `json:"location,omitempty"`
	NetworkZone      *string                             `json:"network_zone,omitempty"`
	Labels           *map[string]string                  `json:"labels,omitempty"`
	Targets          []LoadBalancerCreateRequestTarget   `json:"targets,omitempty"`
	Services         []LoadBalancerCreateRequestService  `json:"services,omitempty"`
	PublicInterface  *bool                               `json:"public_interface,omitempty"`
	Network          *int                                `json:"network,omitempty"`
}

type LoadBalancerCreateRequestAlgorithm struct {
	Type string `json:"type"`
}

type LoadBalancerCreateRequestTarget struct {
	Type          string                                        `json:"type"`
	Server        *LoadBalancerCreateRequestTargetServer        `json:"server,omitempty"`
	LabelSelector *LoadBalancerCreateRequestTargetLabelSelector `json:"label_selector,omitempty"`
	IP            *LoadBalancerCreateRequestTargetIP            `json:"ip,omitempty"`
	UsePrivateIP  *bool                                         `json:"use_private_ip,omitempty"`
}

type LoadBalancerCreateRequestTargetServer struct {
	ID int `json:"id"`
}

type LoadBalancerCreateRequestTargetLabelSelector struct {
	Selector string `json:"selector"`
}

type LoadBalancerCreateRequestTargetIP struct {
	IP string `json:"ip"`
}

type LoadBalancerCreateRequestService struct {
	Protocol        string                                       `json:"protocol"`
	ListenPort      *int                                         `json:"listen_port,omitempty"`
	DestinationPort *int                                         `json:"destination_port,omitempty"`
	Proxyprotocol   *bool                                        `json:"proxyprotocol,omitempty"`
	HTTP            *LoadBalancerCreateRequestServiceHTTP        `json:"http,omitempty"`
	HealthCheck     *LoadBalancerCreateRequestServiceHealthCheck `json:"health_check,omitempty"`
}

type LoadBalancerCreateRequestServiceHTTP struct {
	CookieName     *string `json:"cookie_name,omitempty"`
	CookieLifetime *int    `json:"cookie_lifetime,omitempty"`
	Certificates   *[]int  `json:"certificates,omitempty"`
	RedirectHTTP   *bool   `json:"redirect_http,omitempty"`
	StickySessions *bool   `json:"sticky_sessions,omitempty"`
}

type LoadBalancerCreateRequestServiceHealthCheck struct {
	Protocol string                                           `json:"protocol"`
	Port     *int                                             `json:"port,omitempty"`
	Interval *int                                             `json:"interval,omitempty"`
	Timeout  *int                                             `json:"timeout,omitempty"`
	Retries  *int                                             `json:"retries,omitempty"`
	HTTP     *LoadBalancerCreateRequestServiceHealthCheckHTTP `json:"http,omitempty"`
}

type LoadBalancerCreateRequestServiceHealthCheckHTTP struct {
	Domain      *string   `json:"domain,omitempty"`
	Path        *string   `json:"path,omitempty"`
	Response    *string   `json:"response,omitempty"`
	StatusCodes *[]string `json:"status_codes,omitempty"`
	TLS         *bool     `json:"tls,omitempty"`
}

type LoadBalancerCreateResponse struct {
	LoadBalancer LoadBalancer `json:"load_balancer"`
	Action       Action       `json:"action"`
}

type LoadBalancerActionChangeProtectionRequest struct {
	Delete *bool `json:"delete,omitempty"`
}

type LoadBalancerActionChangeProtectionResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerUpdateRequest struct {
	Name   *string            `json:"name,omitempty"`
	Labels *map[string]string `json:"labels,omitempty"`
}

type LoadBalancerUpdateResponse struct {
	LoadBalancer LoadBalancer `json:"load_balancer"`
}

type LoadBalancerActionChangeAlgorithmRequest struct {
	Type string `json:"type"`
}

type LoadBalancerActionChangeAlgorithmResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerActionAttachToNetworkRequest struct {
	Network int     `json:"network"`
	IP      *string `json:"ip,omitempty"`
}

type LoadBalancerActionAttachToNetworkResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerActionDetachFromNetworkRequest struct {
	Network int `json:"network"`
}

type LoadBalancerActionDetachFromNetworkResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerActionEnablePublicInterfaceRequest struct{}

type LoadBalancerActionEnablePublicInterfaceResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerActionDisablePublicInterfaceRequest struct{}

type LoadBalancerActionDisablePublicInterfaceResponse struct {
	Action Action `json:"action"`
}

type LoadBalancerActionChangeTypeRequest struct {
	LoadBalancerType interface{} `json:"load_balancer_type"` // int or string
}

type LoadBalancerActionChangeTypeResponse struct {
	Action Action `json:"action"`
}

// LoadBalancerGetMetricsResponse defines the schema of the response when
// requesting metrics for a Load Balancer.
type LoadBalancerGetMetricsResponse struct {
	Metrics struct {
		Start      time.Time                             `json:"start"`
		End        time.Time                             `json:"end"`
		Step       float64                               `json:"step"`
		TimeSeries map[string]LoadBalancerTimeSeriesVals `json:"time_series"`
	} `json:"metrics"`
}

// LoadBalancerTimeSeriesVals contains the values for a Load Balancer time
// series.
type LoadBalancerTimeSeriesVals struct {
	Values []interface{} `json:"values"`
}
