// Copyright 2020 The Prometheus Authors
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

package huawei

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/huaweicloud/golangsdk"
	"github.com/huaweicloud/golangsdk/openstack"
	"github.com/huaweicloud/golangsdk/openstack/ecs/v1/cloudservers"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	addressTypeFixed     = "fixed"
	addressTypeFloating  = "floating"
	ecsLabel             = model.MetaLabelPrefix + "ecs_"
	ecsLabelID           = ecsLabel + "id"
	ecsLabelName         = ecsLabel + "name"
	ecsLabelHostID       = ecsLabel + "host_id"
	ecsLabelStatus       = ecsLabel + "status"
	ecsLabelFixedIP      = ecsLabel + "fixed_ip_"
	ecsLabelFloatingIP   = ecsLabel + "floating_ip_"
	ecsLabelHostStatus   = ecsLabel + "host_status"
	ecsLabelFlavorID     = ecsLabel + "flavor_id"
	ecsLabelFlavorName   = ecsLabel + "flavor_name"
	ecsLabelFlavorCPU    = ecsLabel + "flavor_cpu"
	ecsLabelFlavorRAM    = ecsLabel + "flavor_ram"
	ecsLabelVpcID        = ecsLabel + "vpc_id"
	ecsLabelImageName    = ecsLabel + "image_name"
	ecsLabelOsBit        = ecsLabel + "os_bit"
	ecsLabelAz           = ecsLabel + "availability_zone"
	ecsLabelHostName     = ecsLabel + "host_name"
	ecsLabelInstanceName = ecsLabel + "instance_name"
	ecsLabelTag          = ecsLabel + "tag_"
)

var DefaultSDConfig = SDConfig{
	Port:            80,
	RefreshInterval: model.Duration(1 * time.Minute),
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

type SDConfig struct {
	ProjectName     string         `yaml:"project_name"`
	DomainName      string         `yaml:"domain_name,omitempty"`
	Region          string         `yaml:"region"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       string         `yaml:"secret_key,omitempty"`
	AuthURL         string         `yaml:"auth_url"`
	UserName        string         `yaml:"username,omitempty"`
	Password        string         `yaml:"password,omitempty"`
	Port            int            `yaml:"port"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
}

func (*SDConfig) Name() string {
	return "huawei"
}

func (c *SDConfig) NewDiscoverer(options discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, options.Logger), nil
}

func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	if check, err := validateAuthParamWithErr(c.Region, "region"); !check {
		return err
	}
	if check, err := validateAuthParamWithErr(c.AuthURL, "auth_url"); !check {
		return err
	}
	if check, err := validateAuthParamWithErr(c.ProjectName, "project_name"); !check {
		return err
	}
	if !(validateAuthParam(c.AccessKey) && validateAuthParam(c.SecretKey)) &&
		!(validateAuthParam(c.UserName) && validateAuthParam(c.Password) && validateAuthParam(c.DomainName)) {
		return errors.Errorf("unknown authentication_type, huawei support aksk with [access_key, secret_key] "+
			"or password with [domain_name, user_name, password]")
	}
	return nil
}

func validateAuthParamWithErr(param, name string) (bool, error) {
	if len(param) == 0 {
		return false, errors.Errorf("huawei SD configuration requires a %s", name)
	}
	return true, nil
}

func validateAuthParam(param string) bool {
	return len(param) != 0
}

type Discovery struct {
	*refresh.Discovery
	cfg     *SDConfig
	logger  log.Logger
	hwCloud *golangsdk.ProviderClient
}

func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &Discovery{
		cfg:    conf,
		logger: logger,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"huawei",
		time.Duration(d.cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *Discovery) buildHWClient() (*golangsdk.ProviderClient, error) {
	if d.hwCloud != nil {
		return d.hwCloud, nil
	}
	if validateAuthParam(d.cfg.AccessKey) && validateAuthParam(d.cfg.SecretKey) {
		return buildClientByAKSK(d.cfg)
	} else if validateAuthParam(d.cfg.UserName) && validateAuthParam(d.cfg.Password) && validateAuthParam(d.cfg.DomainName) {
		return buildClientByPassword(d.cfg)
	}
	return nil, errors.Errorf("build huawei client fail , please check your huawei SD configuration")
}

func buildClientByAKSK(c *SDConfig) (*golangsdk.ProviderClient, error) {
	var pao, dao golangsdk.AKSKAuthOptions

	pao = golangsdk.AKSKAuthOptions{
		ProjectName: c.ProjectName,
	}

	dao = golangsdk.AKSKAuthOptions{
		Domain: c.DomainName,
	}

	for _, ao := range []*golangsdk.AKSKAuthOptions{&pao, &dao} {
		ao.IdentityEndpoint = c.AuthURL
		ao.AccessKey = c.AccessKey
		ao.SecretKey = c.SecretKey
	}
	return genClient(pao)
}

func buildClientByPassword(c *SDConfig) (*golangsdk.ProviderClient, error) {
	var pao, dao golangsdk.AuthOptions

	pao = golangsdk.AuthOptions{
		DomainName: c.DomainName,
		TenantName: c.ProjectName,
	}

	dao = golangsdk.AuthOptions{
		DomainName: c.DomainName,
	}

	for _, ao := range []*golangsdk.AuthOptions{&pao, &dao} {
		ao.IdentityEndpoint = c.AuthURL
		ao.Password = c.Password
		ao.Username = c.UserName
	}

	return genClient(pao)
}

func genClient(pao golangsdk.AuthOptionsProvider) (*golangsdk.ProviderClient, error) {
	client, err := openstack.NewClient(pao.GetIdentityEndpoint())
	if err != nil {
		return nil, err
	}

	client.HTTPClient = http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if client.AKSKAuthOptions.AccessKey != "" {
				golangsdk.ReSign(req, golangsdk.SignOptions{
					AccessKey: client.AKSKAuthOptions.AccessKey,
					SecretKey: client.AKSKAuthOptions.SecretKey,
				})
			}
			return nil
		},
	}

	err = openstack.Authenticate(client, pao)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	hwClient, err := d.buildHWClient()
	if err != nil {
		return nil, err
	}
	client, err := openstack.NewEcsV1(hwClient, golangsdk.EndpointOpts{
		Region: d.cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	allPages, err := cloudservers.List(client, cloudservers.ListOpts{}).AllPages()
	if err != nil {
		level.Error(d.logger).Log("List Servers error: %s", err.Error())
	}
	servers, err := cloudservers.ExtractServers(allPages)
	if err != nil {
		level.Error(d.logger).Log("Extract servers all pages error: %s", err.Error())
		return nil, err
	}

	targets := make([]model.LabelSet, len(servers))
	for sid, server := range servers {
		level.Info(d.logger).Log("msg", "Discovery Server", "Server Name", server.Name)
		labels := model.LabelSet{
			ecsLabelID:           model.LabelValue(server.ID),
			ecsLabelName:         model.LabelValue(server.Name),
			ecsLabelHostID:       model.LabelValue(server.HostID),
			ecsLabelStatus:       model.LabelValue(server.Status),
			ecsLabelHostStatus:   model.LabelValue(server.HostStatus),
			ecsLabelFlavorID:     model.LabelValue(server.Flavor.ID),
			ecsLabelFlavorName:   model.LabelValue(server.Flavor.Name),
			ecsLabelFlavorCPU:    model.LabelValue(server.Flavor.Vcpus),
			ecsLabelFlavorRAM:    model.LabelValue(server.Flavor.RAM),
			ecsLabelVpcID:        model.LabelValue(server.Metadata.VpcID),
			ecsLabelImageName:    model.LabelValue(server.Metadata.ImageName),
			ecsLabelOsBit:        model.LabelValue(server.Metadata.OsBit),
			ecsLabelAz:           model.LabelValue(server.AvailabilityZone),
			ecsLabelHostName:     model.LabelValue(server.Hostname),
			ecsLabelInstanceName: model.LabelValue(server.InstanceName),
		}
		if len(server.Tags) > 0 {
			for _, tagKV := range server.Tags {
				kv := strings.Split(tagKV, "=")
				if len(kv) == 1 {
					labels[ecsLabelTag+model.LabelName(kv[0])] = model.LabelValue("")
					continue
				}
				labels[ecsLabelTag+model.LabelName(kv[0])] = model.LabelValue(kv[1])
			}
		}
		var fixed, floating int = 0, 0
		for _, addresss := range server.Addresses {
			for _, address := range addresss {
				if addressTypeFixed == address.Type {
					labels[model.LabelName(ecsLabelFixedIP+strconv.Itoa(fixed))] = model.LabelValue(address.Addr)
					if fixed == 0 {
						address := net.JoinHostPort(address.Addr, fmt.Sprintf("%d", d.cfg.Port))
						labels[model.AddressLabel] = model.LabelValue(address)
					}
					fixed++
				} else if addressTypeFloating == address.Type {
					labels[model.LabelName(ecsLabelFloatingIP+strconv.Itoa(floating))] = model.LabelValue(address.Addr)
					floating++
				}
			}
		}
		targets[sid] = labels
	}
	return []*targetgroup.Group{{Source: d.cfg.Region, Targets: targets}}, nil
}
