// Copyright 2018 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// vSphere Service Discovery label prefix
const (
	vsphereLabel = model.MetaLabelPrefix + "vsphere_"
)

// Role is role of the target in vSphere.
type Role string

// The valid options for vSphereRole.
const (
	vSphereRoleEsx            Role = "esx"
	vSphereRoleDatastore      Role = "datastore"
	vSphereRoleVirtualMachine Role = "virtualmachine"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case vSphereRoleEsx, vSphereRoleDatastore, vSphereRoleVirtualMachine:
		return nil
	default:
		return fmt.Errorf("Unknown vSphere SD role %q", *c)
	}
}

var (
	vsphereSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_vsphere_esx_refresh_failures_total",
			Help: "The number of vSphere-SD scrape failures.",
		})
	vsphereSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_vsphere_esx_refresh_duration_seconds",
			Help: "The duration of a vSphere-SD refresh in seconds.",
		})

	//errClientParamsNil for not providing the correct params
	errClientParamsNil = errors.New("The govmomi client parameters are nil. Need to re-init")

	// DefaultSDConfig is the default vSphere SD configuration.
	DefaultSDConfig = SDConfig{
		MetricsProxyPort: 9444,
		RefreshInterval:  model.Duration(60 * time.Second),
	}
)

func init() {
	prometheus.MustRegister(vsphereSDRefreshFailuresCount)
	prometheus.MustRegister(vsphereSDRefreshDuration)
}

// Filter is the configuration for filtering vSphere instances.
/*
type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}
*/

// SDConfig is the configuration for vSphere based service discovery.
type SDConfig struct {
	Role                Role               `yaml:"role"`
	VCenterAddress      string             `yaml:"vcenter_address"`
	VCenterPort         int                `yaml:"vcenter_port,omitempty"`
	Insecure            bool               `yaml:"vcenter_insecure"`
	Username            string             `yaml:"vcenter_username"`
	Password            config_util.Secret `yaml:"vcenter_password"`
	MetricsProxyAddress string             `yaml:"metrics_proxy_address"`
	MetricsProxyPort    int                `yaml:"metrics_proxy_port,omitempty"`
	NewConnections      bool               `yaml:"new_connections,omitempty"`
	RefreshInterval     model.Duration     `yaml:"refresh_interval,omitempty"`
	//Filters []*Filter `yaml:"filters"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.VCenterAddress == "" {
		return fmt.Errorf("vSphere SD requires you provide a vCenter URI")
	}
	if c.Username == "" {
		return fmt.Errorf("vSphere SD requires you provide a vCenter Username")
	}
	if c.Password == "" {
		return fmt.Errorf("vSphere SD requires you provide a vCenter Password")
	}
	return nil
}

//Client represents a vSphere client
type Client struct {
	config  *SDConfig
	ctx     *context.Context
	vClient *govmomi.Client
}

func (c *Client) isValid() error {
	if c.vClient == nil || c.ctx == nil {
		return errClientParamsNil
	}

	var mgr mo.SessionManager
	err := mo.RetrieveProperties(context.Background(), c.vClient, c.vClient.ServiceContent.PropertyCollector, *c.vClient.ServiceContent.SessionManager, &mgr)
	return err
}

func (c *Client) logout() {
	if c.vClient == nil || c.ctx == nil {
		return
	}

	c.vClient.Logout(*c.ctx)

	c.ctx = nil
	c.vClient = nil
}

func (c *Client) getClient() error {
	// Does a connection already exist? Then reuse it!
	if c.isValid() == nil {
		return nil
	}

	//Logout out just in case and clear out state in Client
	c.logout()

	// Default context
	ctx := context.Background()
	c.ctx = &ctx

	// Setup URL object
	urlBase := "https://username:password@host/sdk"
	u, err := soap.ParseURL(urlBase)
	if err != nil {
		return err
	}

	u.User = url.UserPassword(c.config.Username, string(c.config.Password))
	if c.config.VCenterPort > 0 {
		u.Host = c.config.VCenterAddress + ":" + strconv.Itoa(c.config.VCenterPort)
	} else {
		u.Host = c.config.VCenterAddress
	}

	// Connect and login to ESX or vCenter
	c.vClient, err = govmomi.NewClient(ctx, u, c.config.Insecure)
	return err
}

// Discovery periodically performs OpenStack-SD requests. It implements
// the Discoverer interface.
type Discovery interface {
	Run(ctx context.Context, ch chan<- []*targetgroup.Group)
	refresh() (tg *targetgroup.Group, err error)
}

// NewDiscovery returns a new VSphere Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	switch conf.Role {
	case vSphereRoleEsx:
		level.Info(logger).Log("msg", "Creating vSphere ESX role discovery")
		esx := NewEsxDiscovery(conf, logger)
		return esx, nil

	case vSphereRoleDatastore:
		level.Info(logger).Log("msg", "Creating vSphere Datastore role discovery")
		instance := NewDatastoreDiscovery(conf, logger)
		return instance, nil

	case vSphereRoleVirtualMachine:
		level.Info(logger).Log("msg", "Creating vSphere VirtualMachine role discovery")
		instance := NewVirtualMachineDiscovery(conf, logger)
		return instance, nil

	default:
		level.Warn(logger).Log("msg", "Unknown vSphere role discovery")
		return nil, errors.New("unknown vSphere discovery role")
	}
}
