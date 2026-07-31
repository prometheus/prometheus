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
	"strconv"
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
	RoleEC2         Role = "ec2"
	RoleECS         Role = "ecs"
	RoleElasticache Role = "elasticache"
	RoleLightsail   Role = "lightsail"
	RoleMSK         Role = "msk"
	RoleRDS         Role = "rds"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(any) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case RoleEC2, RoleECS, RoleElasticache, RoleLightsail, RoleMSK, RoleRDS:
		return nil
	default:
		return fmt.Errorf("unknown AWS SD role %q", *c)
	}
}

func (c Role) String() string {
	return string(c)
}

// Filter is the configuration for filtering AWS resources.
type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
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
	ExternalID       string                  `yaml:"external_id,omitempty"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
	Port             int                     `yaml:"port,omitempty"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	// ec2, rds specific
	Filters []*Filter `yaml:"filters,omitempty"`

	// ecs, msk specific
	Clusters []string `yaml:"clusters,omitempty"`

	// Embedded sub-configs (internal use only, not serialized)
	*EC2SDConfig         `yaml:"-"`
	*ECSSDConfig         `yaml:"-"`
	*ElasticacheSDConfig `yaml:"-"`
	*LightsailSDConfig   `yaml:"-"`
	*MSKSDConfig         `yaml:"-"`
	*RDSSDConfig         `yaml:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SDConfig.
// Region resolution is deferred to each concrete discovery's xxxClient
// method; see loadRegion.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Alias to avoid recursion
	type plain SDConfig
	var aux plain
	// Unmarshal into aux
	if err := unmarshal(&aux); err != nil {
		return err
	}
	*c = SDConfig(aux)

	switch c.Role {
	case RoleEC2:
		if c.EC2SDConfig == nil {
			ec2Config := DefaultEC2SDConfig
			c.EC2SDConfig = &ec2Config
		}
		c.EC2SDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.EC2SDConfig.Region = c.Region
		setIfNonZero(&c.EC2SDConfig.Endpoint, c.Endpoint)
		setIfNonZero(&c.EC2SDConfig.AccessKey, c.AccessKey)
		setIfNonZero(&c.EC2SDConfig.SecretKey, c.SecretKey)
		setIfNonZero(&c.EC2SDConfig.Profile, c.Profile)
		setIfNonZero(&c.EC2SDConfig.RoleARN, c.RoleARN)
		setIfNonZero(&c.EC2SDConfig.ExternalID, c.ExternalID)
		setIfNonZero(&c.EC2SDConfig.Port, c.Port)
		setIfNonZero(&c.EC2SDConfig.RefreshInterval, c.RefreshInterval)
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
		setIfNonZero(&c.ECSSDConfig.Endpoint, c.Endpoint)
		setIfNonZero(&c.ECSSDConfig.AccessKey, c.AccessKey)
		setIfNonZero(&c.ECSSDConfig.SecretKey, c.SecretKey)
		setIfNonZero(&c.ECSSDConfig.Profile, c.Profile)
		setIfNonZero(&c.ECSSDConfig.RoleARN, c.RoleARN)
		setIfNonZero(&c.ECSSDConfig.ExternalID, c.ExternalID)
		setIfNonZero(&c.ECSSDConfig.Port, c.Port)
		setIfNonZero(&c.ECSSDConfig.RefreshInterval, c.RefreshInterval)
		if c.Clusters != nil {
			c.ECSSDConfig.Clusters = c.Clusters
		}
	case RoleElasticache:
		if c.ElasticacheSDConfig == nil {
			elasticacheConfig := DefaultElasticacheSDConfig
			c.ElasticacheSDConfig = &elasticacheConfig
		}
		c.ElasticacheSDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.ElasticacheSDConfig.Region = c.Region
		setIfNonZero(&c.ElasticacheSDConfig.Endpoint, c.Endpoint)
		setIfNonZero(&c.ElasticacheSDConfig.AccessKey, c.AccessKey)
		setIfNonZero(&c.ElasticacheSDConfig.SecretKey, c.SecretKey)
		setIfNonZero(&c.ElasticacheSDConfig.Profile, c.Profile)
		setIfNonZero(&c.ElasticacheSDConfig.RoleARN, c.RoleARN)
		setIfNonZero(&c.ElasticacheSDConfig.ExternalID, c.ExternalID)
		setIfNonZero(&c.ElasticacheSDConfig.Port, c.Port)
		setIfNonZero(&c.ElasticacheSDConfig.RefreshInterval, c.RefreshInterval)
		if c.Clusters != nil {
			c.ElasticacheSDConfig.Clusters = c.Clusters
		}
	case RoleLightsail:
		if c.LightsailSDConfig == nil {
			lightsailConfig := DefaultLightsailSDConfig
			c.LightsailSDConfig = &lightsailConfig
		}
		c.LightsailSDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.LightsailSDConfig.Region = c.Region
		setIfNonZero(&c.LightsailSDConfig.Endpoint, c.Endpoint)
		setIfNonZero(&c.LightsailSDConfig.AccessKey, c.AccessKey)
		setIfNonZero(&c.LightsailSDConfig.SecretKey, c.SecretKey)
		setIfNonZero(&c.LightsailSDConfig.Profile, c.Profile)
		setIfNonZero(&c.LightsailSDConfig.RoleARN, c.RoleARN)
		setIfNonZero(&c.LightsailSDConfig.ExternalID, c.ExternalID)
		setIfNonZero(&c.LightsailSDConfig.Port, c.Port)
		setIfNonZero(&c.LightsailSDConfig.RefreshInterval, c.RefreshInterval)
	case RoleMSK:
		if c.MSKSDConfig == nil {
			mskConfig := DefaultMSKSDConfig
			c.MSKSDConfig = &mskConfig
		}
		c.MSKSDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.MSKSDConfig.Region = c.Region
		setIfNonZero(&c.MSKSDConfig.Endpoint, c.Endpoint)
		setIfNonZero(&c.MSKSDConfig.AccessKey, c.AccessKey)
		setIfNonZero(&c.MSKSDConfig.SecretKey, c.SecretKey)
		setIfNonZero(&c.MSKSDConfig.Profile, c.Profile)
		setIfNonZero(&c.MSKSDConfig.RoleARN, c.RoleARN)
		setIfNonZero(&c.MSKSDConfig.ExternalID, c.ExternalID)
		setIfNonZero(&c.MSKSDConfig.Port, c.Port)
		setIfNonZero(&c.MSKSDConfig.RefreshInterval, c.RefreshInterval)
		if c.Clusters != nil {
			c.MSKSDConfig.Clusters = c.Clusters
		}
	case RoleRDS:
		if c.RDSSDConfig == nil {
			rdsConfig := DefaultRDSSDConfig
			c.RDSSDConfig = &rdsConfig
		}
		c.RDSSDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.RDSSDConfig.Region = c.Region
		setIfNonZero(&c.RDSSDConfig.Endpoint, c.Endpoint)
		setIfNonZero(&c.RDSSDConfig.AccessKey, c.AccessKey)
		setIfNonZero(&c.RDSSDConfig.SecretKey, c.SecretKey)
		setIfNonZero(&c.RDSSDConfig.Profile, c.Profile)
		setIfNonZero(&c.RDSSDConfig.RoleARN, c.RoleARN)
		setIfNonZero(&c.RDSSDConfig.ExternalID, c.ExternalID)
		setIfNonZero(&c.RDSSDConfig.Port, c.Port)
		setIfNonZero(&c.RDSSDConfig.RefreshInterval, c.RefreshInterval)
		if c.Filters != nil {
			c.RDSSDConfig.Filters = c.Filters
		}
		if c.Clusters != nil {
			c.RDSSDConfig.Clusters = c.Clusters
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
	case RoleElasticache:
		opts.Metrics = &elasticacheMetrics{refreshMetrics: awsMetrics.refreshMetrics}
		return NewElasticacheDiscovery(c.ElasticacheSDConfig, opts)
	case RoleLightsail:
		opts.Metrics = &lightsailMetrics{refreshMetrics: awsMetrics.refreshMetrics}
		return NewLightsailDiscovery(c.LightsailSDConfig, opts)
	case RoleMSK:
		opts.Metrics = &mskMetrics{refreshMetrics: awsMetrics.refreshMetrics}
		return NewMSKDiscovery(c.MSKSDConfig, opts)
	case RoleRDS:
		opts.Metrics = &rdsMetrics{refreshMetrics: awsMetrics.refreshMetrics}
		return NewRDSDiscovery(c.RDSSDConfig, opts)
	default:
		return nil, fmt.Errorf("unknown AWS SD role %q", c.Role)
	}
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	switch c.Role {
	case RoleEC2:
		setDirectory(c.EC2SDConfig, dir)
	case RoleECS:
		setDirectory(c.ECSSDConfig, dir)
	case RoleElasticache:
		setDirectory(c.ElasticacheSDConfig, dir)
	case RoleLightsail:
		setDirectory(c.LightsailSDConfig, dir)
	case RoleMSK:
		setDirectory(c.MSKSDConfig, dir)
	case RoleRDS:
		setDirectory(c.RDSSDConfig, dir)
	}
}

// setDirectory calls cfg.SetDirectory, unless cfg is nil. It replaces the nil
// check every role repeats in SDConfig.SetDirectory, since the role-specific
// config for the role that is not active is left nil.
func setDirectory[T interface {
	comparable
	SetDirectory(string)
}](cfg T, dir string) {
	var zero T
	if cfg != zero {
		cfg.SetDirectory(dir)
	}
}

// loadRegion finds the region in order: configured region -> AWS config/env vars -> IMDS.
// Region resolution is intentionally deferred to SD init so config-only operations
// (e.g. `promtool check config`) stay free of network I/O. See the UnmarshalYAML
// docstrings in this package.
func loadRegion(ctx context.Context, specifiedRegion string) (region string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("could not determine AWS region: %w", err)
		}
	}()

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
	imdsRegion, err := imdsClient.GetRegion(ctx, &imds.GetRegionInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get region from IMDS: %w", err)
	}

	if imdsRegion.Region == "" {
		return "", errors.New("region not found in AWS config or IMDS")
	}

	return imdsRegion.Region, nil
}

// setIfNonZero assigns *dst = src, unless src is the zero value of T. It
// replaces the repeated "if the parent config set this field, copy it into
// the role-specific config" checks in SDConfig.UnmarshalYAML.
func setIfNonZero[T comparable](dst *T, src T) {
	var zero T
	if src != zero {
		*dst = src
	}
}

// setIfNotNil assigns *dst = *v, unless v is nil. It replaces the nil checks
// AWS discoveries repeat when copying an optional AWS SDK pointer field into
// a plain struct field.
func setIfNotNil[T any](dst, v *T) {
	if v != nil {
		*dst = *v
	}
}

// setNonEmptyField assigns *dst = string(v), unless v is empty. It is meant
// for the AWS SDK enumeration types, which are string types without a nil
// value, when the destination is a plain struct field rather than a label.
func setNonEmptyField[T ~string](dst *string, v T) {
	if v != "" {
		*dst = string(v)
	}
}

// The setXLabel helpers below replace the nil and empty value checks that every
// AWS discovery repeats for each meta label it builds. They leave labels
// untouched for values AWS did not report, so a target never carries a label
// with a placeholder value.

// setStringLabel sets name in labels to the value of v, unless v is nil.
func setStringLabel(labels model.LabelSet, name model.LabelName, v *string) {
	if v != nil {
		labels[name] = model.LabelValue(*v)
	}
}

// setNonEmptyLabel sets name in labels to v, unless v is empty. It is meant for
// the AWS SDK enumeration types, which are string types without a nil value.
func setNonEmptyLabel[T ~string](labels model.LabelSet, name model.LabelName, v T) {
	if v != "" {
		labels[name] = model.LabelValue(v)
	}
}

// setBoolLabel sets name in labels to "true" or "false", unless v is nil.
func setBoolLabel(labels model.LabelSet, name model.LabelName, v *bool) {
	if v != nil {
		labels[name] = model.LabelValue(strconv.FormatBool(*v))
	}
}

// setIntLabel sets name in labels to the decimal representation of v, unless v is nil.
func setIntLabel[T ~int32 | ~int64](labels model.LabelSet, name model.LabelName, v *T) {
	if v != nil {
		labels[name] = model.LabelValue(strconv.FormatInt(int64(*v), 10))
	}
}

// setTimeLabel sets name in labels to v formatted as RFC 3339, unless v is nil.
func setTimeLabel(labels model.LabelSet, name model.LabelName, v *time.Time) {
	if v != nil {
		labels[name] = model.LabelValue(v.Format(time.RFC3339))
	}
}
