package api

import (
	"encoding/json"
	"time"
)

type ServiceRouterConfigEntry struct {
	Kind      string
	Name      string
	Namespace string `json:",omitempty"`

	Routes []ServiceRoute `json:",omitempty"`

	Meta        map[string]string `json:",omitempty"`
	CreateIndex uint64
	ModifyIndex uint64
}

func (e *ServiceRouterConfigEntry) GetKind() string        { return e.Kind }
func (e *ServiceRouterConfigEntry) GetName() string        { return e.Name }
func (e *ServiceRouterConfigEntry) GetCreateIndex() uint64 { return e.CreateIndex }
func (e *ServiceRouterConfigEntry) GetModifyIndex() uint64 { return e.ModifyIndex }

type ServiceRoute struct {
	Match       *ServiceRouteMatch       `json:",omitempty"`
	Destination *ServiceRouteDestination `json:",omitempty"`
}

type ServiceRouteMatch struct {
	HTTP *ServiceRouteHTTPMatch `json:",omitempty"`
}

type ServiceRouteHTTPMatch struct {
	PathExact  string `json:",omitempty" alias:"path_exact"`
	PathPrefix string `json:",omitempty" alias:"path_prefix"`
	PathRegex  string `json:",omitempty" alias:"path_regex"`

	Header     []ServiceRouteHTTPMatchHeader     `json:",omitempty"`
	QueryParam []ServiceRouteHTTPMatchQueryParam `json:",omitempty" alias:"query_param"`
	Methods    []string                          `json:",omitempty"`
}

type ServiceRouteHTTPMatchHeader struct {
	Name    string
	Present bool   `json:",omitempty"`
	Exact   string `json:",omitempty"`
	Prefix  string `json:",omitempty"`
	Suffix  string `json:",omitempty"`
	Regex   string `json:",omitempty"`
	Invert  bool   `json:",omitempty"`
}

type ServiceRouteHTTPMatchQueryParam struct {
	Name    string
	Present bool   `json:",omitempty"`
	Exact   string `json:",omitempty"`
	Regex   string `json:",omitempty"`
}

type ServiceRouteDestination struct {
	Service               string        `json:",omitempty"`
	ServiceSubset         string        `json:",omitempty" alias:"service_subset"`
	Namespace             string        `json:",omitempty"`
	PrefixRewrite         string        `json:",omitempty" alias:"prefix_rewrite"`
	RequestTimeout        time.Duration `json:",omitempty" alias:"request_timeout"`
	NumRetries            uint32        `json:",omitempty" alias:"num_retries"`
	RetryOnConnectFailure bool          `json:",omitempty" alias:"retry_on_connect_failure"`
	RetryOnStatusCodes    []uint32      `json:",omitempty" alias:"retry_on_status_codes"`
}

func (e *ServiceRouteDestination) MarshalJSON() ([]byte, error) {
	type Alias ServiceRouteDestination
	exported := &struct {
		RequestTimeout string `json:",omitempty"`
		*Alias
	}{
		RequestTimeout: e.RequestTimeout.String(),
		Alias:          (*Alias)(e),
	}
	if e.RequestTimeout == 0 {
		exported.RequestTimeout = ""
	}

	return json.Marshal(exported)
}

func (e *ServiceRouteDestination) UnmarshalJSON(data []byte) error {
	type Alias ServiceRouteDestination
	aux := &struct {
		RequestTimeout string
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	if aux.RequestTimeout != "" {
		if e.RequestTimeout, err = time.ParseDuration(aux.RequestTimeout); err != nil {
			return err
		}
	}
	return nil
}

type ServiceSplitterConfigEntry struct {
	Kind      string
	Name      string
	Namespace string `json:",omitempty"`

	Splits []ServiceSplit `json:",omitempty"`

	Meta        map[string]string `json:",omitempty"`
	CreateIndex uint64
	ModifyIndex uint64
}

func (e *ServiceSplitterConfigEntry) GetKind() string        { return e.Kind }
func (e *ServiceSplitterConfigEntry) GetName() string        { return e.Name }
func (e *ServiceSplitterConfigEntry) GetCreateIndex() uint64 { return e.CreateIndex }
func (e *ServiceSplitterConfigEntry) GetModifyIndex() uint64 { return e.ModifyIndex }

type ServiceSplit struct {
	Weight        float32
	Service       string `json:",omitempty"`
	ServiceSubset string `json:",omitempty" alias:"service_subset"`
	Namespace     string `json:",omitempty"`
}

type ServiceResolverConfigEntry struct {
	Kind      string
	Name      string
	Namespace string `json:",omitempty"`

	DefaultSubset  string                             `json:",omitempty" alias:"default_subset"`
	Subsets        map[string]ServiceResolverSubset   `json:",omitempty"`
	Redirect       *ServiceResolverRedirect           `json:",omitempty"`
	Failover       map[string]ServiceResolverFailover `json:",omitempty"`
	ConnectTimeout time.Duration                      `json:",omitempty" alias:"connect_timeout"`

	Meta        map[string]string `json:",omitempty"`
	CreateIndex uint64
	ModifyIndex uint64
}

func (e *ServiceResolverConfigEntry) MarshalJSON() ([]byte, error) {
	type Alias ServiceResolverConfigEntry
	exported := &struct {
		ConnectTimeout string `json:",omitempty"`
		*Alias
	}{
		ConnectTimeout: e.ConnectTimeout.String(),
		Alias:          (*Alias)(e),
	}
	if e.ConnectTimeout == 0 {
		exported.ConnectTimeout = ""
	}

	return json.Marshal(exported)
}

func (e *ServiceResolverConfigEntry) UnmarshalJSON(data []byte) error {
	type Alias ServiceResolverConfigEntry
	aux := &struct {
		ConnectTimeout string
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	if aux.ConnectTimeout != "" {
		if e.ConnectTimeout, err = time.ParseDuration(aux.ConnectTimeout); err != nil {
			return err
		}
	}
	return nil
}

func (e *ServiceResolverConfigEntry) GetKind() string        { return e.Kind }
func (e *ServiceResolverConfigEntry) GetName() string        { return e.Name }
func (e *ServiceResolverConfigEntry) GetCreateIndex() uint64 { return e.CreateIndex }
func (e *ServiceResolverConfigEntry) GetModifyIndex() uint64 { return e.ModifyIndex }

type ServiceResolverSubset struct {
	Filter      string `json:",omitempty"`
	OnlyPassing bool   `json:",omitempty" alias:"only_passing"`
}

type ServiceResolverRedirect struct {
	Service       string `json:",omitempty"`
	ServiceSubset string `json:",omitempty" alias:"service_subset"`
	Namespace     string `json:",omitempty"`
	Datacenter    string `json:",omitempty"`
}

type ServiceResolverFailover struct {
	Service       string   `json:",omitempty"`
	ServiceSubset string   `json:",omitempty" alias:"service_subset"`
	Namespace     string   `json:",omitempty"`
	Datacenters   []string `json:",omitempty"`
}
