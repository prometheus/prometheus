// Copyright 2021 The Prometheus Authors
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

package tencentcloud

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tencenterr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

const (
	cvmLabel              = model.MetaLabelPrefix + "tencentcloud_"
	cvmLabelRegion        = cvmLabel + "region"
	cvmLabelZone          = cvmLabel + "zone"
	cvmLabelPublicIP      = cvmLabel + "public_ip"
	cvmLabelPrivateIP     = cvmLabel + "private_ip"
	cvmLabelInstanceID    = cvmLabel + "instance_id"
	cvmLabelInstanceName  = cvmLabel + "instance_name"
	cvmLabelInstanceState = cvmLabel + "instance_state"
)

var (
	// DefaultSDConfig is the default EC2 SD configuration.
	DefaultSDConfig = SDConfig{
		Port:            80,
		RefreshInterval: model.Duration(60 * time.Second),
	}
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for tencentcloud based service discovery.
type SDConfig struct {
	Region          string         `yaml:"region"`
	AccessKey       string         `yaml:"access_key"`
	SecretKey       config.Secret  `yaml:"secret_key"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Port            int            `yaml:"port"`
	Filters         []*Filter      `yaml:"filters,omitempty"`
}

// Filter is the configuration for filtering tencentcloud instances.
type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// Name returns the name of the tencentcloud Config.
func (*SDConfig) Name() string { return "tencentcloud" }

// NewDiscoverer returns a Discoverer for the tencentcloud Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the tencentcloud Config.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Region == "" {
		return errors.New("TencentCloud SD configuration requires a region")
	}

	if c.AccessKey == "" {
		return errors.New("TencentCloud SD configuration requires a access_key")
	}
	if c.SecretKey == "" {
		return errors.New("TencentCloud SD configuration requires a secret_key")
	}
	return nil
}

// Discovery periodically performs tencentcloud-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	cfg    *SDConfig
	client *cvm.Client
	logger log.Logger
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	d := &Discovery{
		cfg:    conf,
		logger: logger,
	}

	credential := common.NewCredential(d.cfg.AccessKey, string(d.cfg.SecretKey))
	d.client, _ = cvm.NewClient(credential, d.cfg.Region, profile.NewClientProfile())
	d.Discovery = refresh.NewDiscovery(
		logger,
		"tencentcloud",
		time.Duration(d.cfg.RefreshInterval),
		d.refresh,
	)

	return d, nil
}

func (d *Discovery) getNodes() ([]*cvm.Instance, error) {
	var filters []*cvm.Filter
	for _, f := range d.cfg.Filters {
		filters = append(filters, &cvm.Filter{
			Name:   common.StringPtr(f.Name),
			Values: common.StringPtrs(f.Values),
		})
	}

	var instances []*cvm.Instance

	for {
		request := cvm.NewDescribeInstancesRequest()
		offset := len(instances)
		request.Offset = common.Int64Ptr(int64(offset))
		request.Filters = filters
		response, err := d.client.DescribeInstances(request)
		if _, ok := err.(*tencenterr.TencentCloudSDKError); ok {
			return nil, fmt.Errorf("tencentcloud api error has returned: %s", err)
		}
		if err != nil {
			return nil, err
		}

		instances = append(instances, response.Response.InstanceSet...)
		if int64(len(instances)) == *response.Response.TotalCount {
			break
		}
	}

	return instances, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	tg := &targetgroup.Group{
		Source: d.cfg.Region,
	}

	instances, err := d.getNodes()
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		labels := model.LabelSet{
			cvmLabelInstanceID:    model.LabelValue(*instance.InstanceId),
			cvmLabelInstanceName:  model.LabelValue(*instance.InstanceName),
			cvmLabelInstanceState: model.LabelValue(*instance.InstanceState),
			cvmLabelRegion:        model.LabelValue(d.cfg.Region),
			cvmLabelZone:          model.LabelValue(*instance.Placement.Zone),
		}

		// private IPs
		var privateIPs []string
		for _, privateIP := range instance.PrivateIpAddresses {
			privateIPs = append(privateIPs, *privateIP)
		}
		labels[cvmLabelPrivateIP] = model.LabelValue(strings.Join(privateIPs, ","))

		addr := fmt.Sprintf("%s:%d", privateIPs[0], d.cfg.Port)
		labels[model.AddressLabel] = model.LabelValue(addr)

		// public IPs
		var publicIPs []string
		for _, publicIP := range instance.PublicIpAddresses {
			publicIPs = append(publicIPs, *publicIP)
		}
		labels[cvmLabelPublicIP] = model.LabelValue(strings.Join(publicIPs, ","))

		tg.Targets = append(tg.Targets, labels)
	}

	return []*targetgroup.Group{tg}, nil
}
