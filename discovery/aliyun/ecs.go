// Copyright The Prometheus Authors
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

package aliyun

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs20140526 "github.com/alibabacloud-go/ecs-20140526/v7/client"
	"github.com/aliyun/credentials-go/credentials"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	ecsLabel                       = model.MetaLabelPrefix + "aliyun_ecs_"
	ecsLabelECSInstanceID          = ecsLabel + "instance_id"
	ecsLabelECSInstanceName        = ecsLabel + "instance_name"
	ecsLabelECSInstanceType        = ecsLabel + "instance_type"
	ecsLabelECSInstanceTypeFamily  = ecsLabel + "instance_type_family"
	ecsLabelECSInstanceStatus      = ecsLabel + "instance_status"
	ecsLabelECSInstanceNetworkType = ecsLabel + "instance_network_type"
	ecsLabelInnerIP                = ecsLabel + "inner_ip"
	ecsLabelEIP                    = ecsLabel + "eip"
	ecsLabelVPCPrivateIP           = ecsLabel + "vpc_private_ip"
	ecsLabelPublicIP               = ecsLabel + "public_ip"
	ecsLabelOSType                 = ecsLabel + "os_type"
	ecsLabelECSImageID             = ecsLabel + "image_id"
	ecsLabelRegion                 = ecsLabel + "region"
	ecsLabelZone                   = ecsLabel + "zone"
	ecsLabelTag                    = ecsLabel + "tag_"
	ecsLabelSeparator              = ","
)

// DefaultECSSDConfig is the default ECS SD configuration.
var DefaultECSSDConfig = ECSSDConfig{
	Port:             80,
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&ECSSDConfig{})
}

// ECSSDConfig is the configuration for ECS based service discovery.
type ECSSDConfig struct {
	Region           string                  `yaml:"region,omitempty"`
	Endpoint         string                  `yaml:"endpoint,omitempty"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
	Port             int                     `yaml:"port,omitempty"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	Tags             []*Tag                  `yaml:"tags,omitempty"`

	// Option ,inline needs a struct value field
	// https://github.com/go-yaml/yaml/issues/55
	// *CredentialConfig `yaml:",inline"`
	CredentialConfig *CredentialConfig `yaml:"credential"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*ECSSDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &ecsMetrics{
		refreshMetrics: rmi,
	}
}

// Name returns the name of the ECS Config.
// The name `aliyun_ecs` is used to avoid conflicts with AWS ECS.
func (*ECSSDConfig) Name() string { return "aliyun_ecs" }

// NewDiscoverer returns a Discoverer for the EC2 Config.
func (c *ECSSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewECSDiscovery(c, opts)
}

// SetDirectory joins any relative file paths with dir.
func (c *ECSSDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for the ECS Config.
func (c *ECSSDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultECSSDConfig
	type plain ECSSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	return c.HTTPClientConfig.Validate()
}

type ecsClient interface {
	DescribeInstances(*ecs20140526.DescribeInstancesRequest) (*ecs20140526.DescribeInstancesResponse, error)
}

// ECSDiscovery periodically performs ECS-SD requests. It implements
// the Discoverer interface.
type ECSDiscovery struct {
	*refresh.Discovery
	logger *slog.Logger
	cfg    *ECSSDConfig
	ecs    ecsClient

	// region is the resolved region used for the Aliyun client and for the
	// Source / __meta_aliyun_ecs_region labels.
	region string
}

// NewECSDiscovery returns a new ECSDiscovery which periodically refreshes its targets.
func NewECSDiscovery(conf *ECSSDConfig, opts discovery.DiscovererOptions) (*ECSDiscovery, error) {
	m, ok := opts.Metrics.(*ecsMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}
	d := &ECSDiscovery{
		logger: opts.Logger,
		cfg:    conf,
		region: conf.Region,
	}
	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                conf.Name(),
			Interval:            time.Duration(d.cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *ECSDiscovery) initECSClient() error {
	if d.ecs != nil {
		return nil
	}

	credential, err := credentials.NewCredential(d.cfg.CredentialConfig.Convert())
	if err != nil {
		return fmt.Errorf("create credential failed: %w", err)
	}

	config := &openapi.Config{
		Credential: credential,
	}
	if d.cfg.Endpoint != "" {
		config.SetEndpoint(d.cfg.Endpoint)
	}
	if d.cfg.Region != "" {
		config.SetRegionId(d.cfg.Region)
	}
	client, err := ecs20140526.NewClient(config)
	if err != nil {
		return fmt.Errorf("create client failed: %w", err)
	}

	d.ecs = client

	// Test credentials by making a simple API call
	_, err = d.ecs.DescribeInstances(&ecs20140526.DescribeInstancesRequest{
		RegionId: &d.region,
	})
	if err != nil {
		d.logger.Error("Failed to test ECS credentials", "error", err)
		return fmt.Errorf("ECS credential test failed: %w", err)
	}
	return nil
}

func (d *ECSDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	err := d.initECSClient()
	if err != nil {
		return nil, err
	}

	tg := &targetgroup.Group{
		Source: d.region,
	}

	ecsTags := []*ecs20140526.DescribeInstancesRequestTag{}
	for _, tag := range d.cfg.Tags {
		if tag == nil {
			continue
		}
		ecsTag := &ecs20140526.DescribeInstancesRequestTag{}
		ecsTag.SetKey(tag.Key)
		ecsTag.SetValue(tag.Value)
		ecsTags = append(ecsTags, ecsTag)
	}

	req := &ecs20140526.DescribeInstancesRequest{
		RegionId: &d.region,
	}
	if ecsTags != nil {
		req.Tag = ecsTags
	}

	paginator := newDescribeInstancesPaginator(d.ecs, req)
	for paginator.hasMorePages() {
		resp, err := paginator.nextPage()
		if err != nil {
			return nil, fmt.Errorf("refresh target groups, err: %w", err)
		}
		instances := resp.Body.Instances.Instance
		for _, inst := range instances {
			labels := model.LabelSet{
				ecsLabelRegion: model.LabelValue(d.region),
			}
			if inst.VpcAttributes != nil && inst.VpcAttributes.PrivateIpAddress != nil && len(inst.VpcAttributes.PrivateIpAddress.IpAddress) != 0 {
				addrs := []string{}
				for _, addr := range inst.VpcAttributes.PrivateIpAddress.IpAddress {
					if addr == nil {
						continue
					}
					addrs = append(addrs, *addr)
				}
				labels[ecsLabelVPCPrivateIP] = model.LabelValue(strings.Join(addrs, ecsLabelSeparator))
			}
			if inst.EipAddress != nil && inst.EipAddress.IpAddress != nil {
				labels[ecsLabelEIP] = model.LabelValue(*inst.EipAddress.IpAddress)
			}
			if inst.InnerIpAddress != nil && len(inst.InnerIpAddress.IpAddress) != 0 {
				addrs := []string{}
				for _, addr := range inst.InnerIpAddress.IpAddress {
					if addr == nil {
						continue
					}
					addrs = append(addrs, *addr)
				}
				labels[ecsLabelInnerIP] = model.LabelValue(strings.Join(addrs, ecsLabelSeparator))
			}
			if inst.PublicIpAddress != nil && len(inst.PublicIpAddress.IpAddress) != 0 {
				addrs := []string{}
				for _, addr := range inst.PublicIpAddress.IpAddress {
					if addr == nil {
						continue
					}
					addrs = append(addrs, *addr)
				}
				labels[ecsLabelPublicIP] = model.LabelValue(strings.Join(addrs, ecsLabelSeparator))
			}

			if inst.InstanceId != nil {
				labels[ecsLabelECSInstanceID] = model.LabelValue(*inst.InstanceId)
			}
			if inst.InstanceName != nil {
				labels[ecsLabelECSInstanceName] = model.LabelValue(*inst.InstanceName)
			}
			if inst.OSType != nil {
				labels[ecsLabelOSType] = model.LabelValue(*inst.OSType)
			}
			if inst.InstanceType != nil {
				labels[ecsLabelECSInstanceType] = model.LabelValue(*inst.InstanceType)
			}
			if inst.InstanceTypeFamily != nil {
				labels[ecsLabelECSInstanceTypeFamily] = model.LabelValue(*inst.InstanceTypeFamily)
			}

			if inst.ImageId != nil {
				labels[ecsLabelECSImageID] = model.LabelValue(*inst.ImageId)
			}
			if inst.ZoneId != nil {
				labels[ecsLabelZone] = model.LabelValue(*inst.ZoneId)
			}
			if inst.Status != nil {
				labels[ecsLabelECSInstanceStatus] = model.LabelValue(*inst.Status)
			}
			if inst.InstanceNetworkType != nil {
				labels[ecsLabelECSInstanceNetworkType] = model.LabelValue(*inst.InstanceNetworkType)
			}

			address := ""
			if inst.NetworkInterfaces != nil && len(inst.NetworkInterfaces.NetworkInterface) != 0 {
				for _, eni := range inst.NetworkInterfaces.NetworkInterface {
					if eni == nil || eni.Type == nil || *eni.Type != "Primary" || eni.PrimaryIpAddress == nil {
						continue
					}
					address = *eni.PrimaryIpAddress
				}
			}
			labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(address, strconv.Itoa(d.cfg.Port)))

			if inst.Tags != nil && len(inst.Tags.Tag) != 0 {
				for _, tag := range inst.Tags.Tag {
					if tag.TagKey == nil || tag.TagValue == nil {
						continue
					}
					name := strutil.SanitizeLabelName(*tag.TagKey)
					labels[ecsLabelTag+model.LabelName(name)] = model.LabelValue(*tag.TagValue)
				}
			}
			tg.Targets = append(tg.Targets, labels)
		}
	}

	return []*targetgroup.Group{tg}, nil
}

type describeInstancesPaginator struct {
	client    ecsClient
	request   *ecs20140526.DescribeInstancesRequest
	firstPage bool
	nextToken *string
}

// NewDescribeInstancesPaginator returns a new DescribeInstancesPaginator
func newDescribeInstancesPaginator(client ecsClient, req *ecs20140526.DescribeInstancesRequest) *describeInstancesPaginator {
	if req == nil {
		req = &ecs20140526.DescribeInstancesRequest{}
	}

	return &describeInstancesPaginator{
		client:    client,
		request:   req,
		firstPage: true,
	}
}

func (p *describeInstancesPaginator) hasMorePages() bool {
	return p.firstPage || (p.nextToken != nil && len(*p.nextToken) != 0)
}

// NextPage retrieves the next DescribeInstances page.
func (p *describeInstancesPaginator) nextPage() (*ecs20140526.DescribeInstancesResponse, error) {
	if !p.hasMorePages() {
		return nil, fmt.Errorf("no more pages available")
	}

	req := p.request
	req.NextToken = p.nextToken

	resp, err := p.client.DescribeInstances(req)
	if err != nil {
		return nil, err
	}
	if resp.Body == nil {
		if resp.StatusCode == nil {
			return nil, fmt.Errorf("both the error code and the response body are nil")
		}
		return nil, fmt.Errorf("error code: %d (ref: https://help.aliyun.com/en/ecs/developer-reference/api-ecs-2014-05-26-errorcodes)", resp.StatusCode)
	}
	p.firstPage = false

	prevToken := p.nextToken
	p.nextToken = resp.Body.NextToken

	if prevToken != nil &&
		p.nextToken != nil &&
		*prevToken == *p.nextToken {
		p.nextToken = nil
	}

	return resp, nil
}
