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
	"errors"
	"fmt"
	"time"

	"github.com/aliyun/credentials-go/credentials"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
)

// DefaultSDConfig is the default Aliyun SD configuration.
var DefaultSDConfig = SDConfig{
	RefreshInterval:  model.Duration(60 * time.Second),
	HTTPClientConfig: config.DefaultHTTPClientConfig,
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// Role is role of the service in Aliyun.
type Role string

// The valid options for Role.
const (
	RoleECS Role = "ecs"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(any) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case RoleECS:
		return nil
	default:
		return fmt.Errorf("unknown Aliyun SD role %q", *c)
	}
}

func (c Role) String() string {
	return string(c)
}

// Tag is the configuration for filtering Aliyun resources.
type Tag struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

type CredentialType string

const (
	CredentialTypeAccessKey   CredentialType = "access_key"
	CredentialTypeSTS         CredentialType = "sts"
	CredentialTypeBearer      CredentialType = "bearer"
	CredentialTypeECSRamRole  CredentialType = "ecs_ram_role"
	CredentialTypeRAMRoleARN  CredentialType = "ram_role_arn"
	CredentialTypeOIDCRoleARN CredentialType = "oidc_role_arn"
	CredentialTypeURI         CredentialType = "credentials_uri"
)

// type CredentialConfig credentials.Config
type CredentialConfig struct {
	Type            *CredentialType `yaml:"type,omitempty"`
	AccessKeyId     *config.Secret  `yaml:"access_key_id,omitempty"`
	AccessKeySecret *config.Secret  `yaml:"access_key_secret,omitempty"`
	SecurityToken   *config.Secret  `yaml:"security_token,omitempty"`
	BearerToken     *config.Secret  `yaml:"bearer_token,omitempty"`

	// Used when the type is ram_role_arn or oidc_role_arn
	OIDCProviderArn       *string `yaml:"oidc_provider_arn,omitempty"`
	OIDCTokenFilePath     *string `yaml:"oidc_token,omitempty"`
	RoleArn               *string `yaml:"role_arn,omitempty"`
	RoleSessionName       *string `yaml:"role_session_name,omitempty"`
	RoleSessionExpiration *int    `yaml:"role_session_expiration,omitempty"`
	Policy                *string `yaml:"policy,omitempty"`
	ExternalId            *string `yaml:"external_id,omitempty"`
	STSEndpoint           *string `yaml:"sts_endpoint,omitempty"`

	// Used when the type is ecs_ram_role
	RoleName *string `yaml:"role_name,omitempty"`

	// Used when the type is credentials_uri
	Url *string `yaml:"url,omitempty"`
}

func (config *CredentialConfig) Convert() *credentials.Config {
	if config == nil {
		return nil
	}
	return &credentials.Config{
		// Include access_key, sts, bearer, ecs_ram_role, ram_role_arn, oidc_role_arn, credentials_uri
		Type:            (*string)(config.Type),
		AccessKeyId:     (*string)(config.AccessKeyId),
		AccessKeySecret: (*string)(config.AccessKeySecret),
		SecurityToken:   (*string)(config.SecurityToken),
		BearerToken:     (*string)(config.BearerToken),

		// Used when the type is ram_role_arn or oidc_role_arn
		OIDCProviderArn:       config.OIDCProviderArn,
		OIDCTokenFilePath:     config.OIDCTokenFilePath,
		RoleArn:               config.RoleArn,
		RoleSessionName:       config.RoleSessionName,
		RoleSessionExpiration: config.RoleSessionExpiration,
		Policy:                config.Policy,
		ExternalId:            config.ExternalId,
		STSEndpoint:           config.STSEndpoint,

		// Used when the type is ecs_ram_role
		RoleName: config.RoleName,

		// Used when the type is credentials_uri
		Url: config.Url,
	}
}

// SDConfig is the configuration for Aliyun service discovery.
type SDConfig struct {
	Role             Role                    `yaml:"role"`
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

	// Embedded sub-configs (internal use only, not serialized)
	ECSSDConfig *ECSSDConfig `yaml:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SDConfig.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	switch c.Role {
	case RoleECS:
		if c.ECSSDConfig == nil {
			ecsConfig := DefaultECSSDConfig
			c.ECSSDConfig = &ecsConfig
		}
		c.ECSSDConfig.HTTPClientConfig = c.HTTPClientConfig
		c.ECSSDConfig.Region = c.Region
		c.ECSSDConfig.CredentialConfig = c.CredentialConfig
		if c.Endpoint != "" {
			c.ECSSDConfig.Endpoint = c.Endpoint
		}
		if c.RefreshInterval != 0 {
			c.ECSSDConfig.RefreshInterval = c.RefreshInterval
		}
		if c.Port != 0 {
			c.ECSSDConfig.Port = c.Port
		}
		if len(c.Tags) != 0 {
			c.ECSSDConfig.Tags = c.Tags
		}
	default:
		return fmt.Errorf("unknown Aliyun SD role %q", c.Role)
	}
	return nil
}

// Name returns the name of the Aliyun Config.
func (*SDConfig) Name() string { return "aliyun" }

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(_ prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &aliyunMetrics{refreshMetrics: rmi}
}

// NewDiscoverer returns a Discoverer for the Aliyun Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	aliyunMetrics, ok := opts.Metrics.(*aliyunMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type for Aliyun SD")
	}

	switch c.Role {
	case RoleECS:
		opts.Metrics = &ecsMetrics{refreshMetrics: aliyunMetrics.refreshMetrics}
		return NewECSDiscovery(c.ECSSDConfig, opts)
	default:
		return nil, fmt.Errorf("unknown Aliyun SD role %q", c.Role)
	}
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	switch c.Role {
	case RoleECS:
		if c.ECSSDConfig != nil {
			c.ECSSDConfig.SetDirectory(dir)
		}
	}
}
