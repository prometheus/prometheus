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

package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
)

// DefaultSDConfig is the default AWS SD configuration.
var DefaultSDConfig = SDConfig{
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// Role is role of the service in AWS.
type Role string

// The valid options for Role.
const (
	RoleEC2       Role = "ec2"
	RoleECS       Role = "ecs"
	RoleLightsail Role = "lightsail"
	RoleMSK       Role = "msk"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(any) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case RoleEC2, RoleECS, RoleLightsail, RoleMSK:
		return nil
	default:
		return fmt.Errorf("unknown AWS SD role %q", *c)
	}
}

func (c Role) String() string {
	return string(c)
}

// SDConfig is the configuration for AWS service discovery.
type SDConfig struct {
	Role             Role                    `yaml:"role"`
	Region           string                  `yaml:"region,omitempty"`
	Endpoint         string                  `yaml:"endpoint,omitempty"`
	AccessKey        string                  `yaml:"access_key,omitempty"`
	SecretKey        config.Secret           `yaml:"secret_key,omitempty"`
	Profile          string                  `yaml:"profile,omitempty"`
	RoleARN          string                  `yaml:"role_arn,omitempty"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
	Port             int                     `yaml:"port,omitempty"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	// ec2 specific
	Filters []*EC2Filter `yaml:"filters,omitempty"`

	// ecs, msk specific
	Clusters []string `yaml:"clusters,omitempty"`

	// Embedded sub-configs (internal use only, not serialized)
	*EC2SDConfig       `yaml:"-"`
	*ECSSDConfig       `yaml:"-"`
	*LightsailSDConfig `yaml:"-"`
	*MSKSDConfig       `yaml:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SDConfig.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Alias to avoid recursion
	type plain SDConfig
	var aux plain
	// Unmarshal into aux
	if err := unmarshal(&aux); err != nil {
		return err
	}
	*c = SDConfig(aux)

	var err error
	c.Region, err = loadRegion(context.Background(), c.Region)
	if err != nil {
		return fmt.Errorf("could not determine AWS region: %w", err)
	}

	switch c.Role {
	case RoleEC2:
		if c.EC2SDConfig == nil {
			ec2Config := DefaultEC2SDConfig
			c.EC2SDConfig = &ec2Config
		}
		c.EC2SDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.EC2SDConfig.Region = c.Region
		if c.Endpoint != "" {
			c.EC2SDConfig.Endpoint = c.Endpoint
		}
		if c.AccessKey != "" {
			c.EC2SDConfig.AccessKey = c.AccessKey
		}
		if c.SecretKey != "" {
			c.EC2SDConfig.SecretKey = c.SecretKey
		}
		if c.Profile != "" {
			c.EC2SDConfig.Profile = c.Profile
		}
		if c.RoleARN != "" {
			c.EC2SDConfig.RoleARN = c.RoleARN
		}
		if c.Port != 0 {
			c.EC2SDConfig.Port = c.Port
		}
		if c.RefreshInterval != 0 {
			c.EC2SDConfig.RefreshInterval = c.RefreshInterval
		}
		if c.Filters != nil {
			c.EC2SDConfig.Filters = c.Filters
		}
	case RoleECS:
		if c.ECSSDConfig == nil {
			ecsConfig := DefaultECSSDConfig
			c.ECSSDConfig = &ecsConfig
		}
		c.ECSSDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.ECSSDConfig.Region = c.Region
		if c.Endpoint != "" {
			c.ECSSDConfig.Endpoint = c.Endpoint
		}
		if c.AccessKey != "" {
			c.ECSSDConfig.AccessKey = c.AccessKey
		}
		if c.SecretKey != "" {
			c.ECSSDConfig.SecretKey = c.SecretKey
		}
		if c.Profile != "" {
			c.ECSSDConfig.Profile = c.Profile
		}
		if c.RoleARN != "" {
			c.ECSSDConfig.RoleARN = c.RoleARN
		}
		if c.Port != 0 {
			c.ECSSDConfig.Port = c.Port
		}
		if c.RefreshInterval != 0 {
			c.ECSSDConfig.RefreshInterval = c.RefreshInterval
		}
		if c.Clusters != nil {
			c.ECSSDConfig.Clusters = c.Clusters
		}
	case RoleLightsail:
		if c.LightsailSDConfig == nil {
			lightsailConfig := DefaultLightsailSDConfig
			c.LightsailSDConfig = &lightsailConfig
		}
		c.LightsailSDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.LightsailSDConfig.Region = c.Region
		if c.Endpoint != "" {
			c.LightsailSDConfig.Endpoint = c.Endpoint
		}
		if c.AccessKey != "" {
			c.LightsailSDConfig.AccessKey = c.AccessKey
		}
		if c.SecretKey != "" {
			c.LightsailSDConfig.SecretKey = c.SecretKey
		}
		if c.Profile != "" {
			c.LightsailSDConfig.Profile = c.Profile
		}
		if c.RoleARN != "" {
			c.LightsailSDConfig.RoleARN = c.RoleARN
		}
		if c.Port != 0 {
			c.LightsailSDConfig.Port = c.Port
		}
		if c.RefreshInterval != 0 {
			c.LightsailSDConfig.RefreshInterval = c.RefreshInterval
		}
	case RoleMSK:
		if c.MSKSDConfig == nil {
			mskConfig := DefaultMSKSDConfig
			c.MSKSDConfig = &mskConfig
		}
		c.MSKSDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.MSKSDConfig.Region = c.Region
		if c.Endpoint != "" {
			c.MSKSDConfig.Endpoint = c.Endpoint
		}
		if c.AccessKey != "" {
			c.MSKSDConfig.AccessKey = c.AccessKey
		}
		if c.SecretKey != "" {
			c.MSKSDConfig.SecretKey = c.SecretKey
		}
		if c.Profile != "" {
			c.MSKSDConfig.Profile = c.Profile
		}
		if c.RoleARN != "" {
			c.MSKSDConfig.RoleARN = c.RoleARN
		}
		if c.Port != 0 {
			c.MSKSDConfig.Port = c.Port
		}
		if c.RefreshInterval != 0 {
			c.MSKSDConfig.RefreshInterval = c.RefreshInterval
		}
		if c.Clusters != nil {
			c.MSKSDConfig.Clusters = c.Clusters
		}
	default:
		return fmt.Errorf("unknown AWS SD role %q", c.Role)
	}
	return nil
}

// Name returns the name of the AWS Config.
func (*SDConfig) Name() string { return "aws" }

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &awsMetrics{refreshMetrics: rmi}
}

// NewDiscoverer returns a Discoverer for the AWS Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	awsMetrics, ok := opts.Metrics.(*awsMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type for AWS SD")
	}

	switch c.Role {
	case RoleEC2:
		opts.Metrics = &ec2Metrics{refreshMetrics: awsMetrics.refreshMetrics}
		return NewEC2Discovery(c.EC2SDConfig, opts)
	case RoleECS:
		opts.Metrics = &ecsMetrics{refreshMetrics: awsMetrics.refreshMetrics}
		return NewECSDiscovery(c.ECSSDConfig, opts)
	case RoleLightsail:
		opts.Metrics = &lightsailMetrics{refreshMetrics: awsMetrics.refreshMetrics}
		return NewLightsailDiscovery(c.LightsailSDConfig, opts)
	case RoleMSK:
		opts.Metrics = &mskMetrics{refreshMetrics: awsMetrics.refreshMetrics}
		return NewMSKDiscovery(c.MSKSDConfig, opts)
	default:
		return nil, fmt.Errorf("unknown AWS SD role %q", c.Role)
	}
}

// loadRegion finds the region in order: AWS config/env vars ->IMDS.
func loadRegion(ctx context.Context, specifiedRegion string) (string, error) {
	if specifiedRegion != "" {
		return specifiedRegion, nil
	}

	cfg, err := awsConfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	if cfg.Region != "" {
		return cfg.Region, nil
	}

	// Fallback (may fail in non-AWS environments)
	imdsClient := imds.NewFromConfig(cfg)
	region, err := imdsClient.GetRegion(ctx, &imds.GetRegionInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get region from IMDS: %w", err)
	}

	if region.Region == "" {
		return "", errors.New("region not found in AWS config or IMDS")
	}

	return region.Region, nil
}
