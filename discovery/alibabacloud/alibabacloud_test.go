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

package alibabacloud

import (
	"errors"
	"testing"

	"github.com/aliyun/credentials-go/credentials"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestRoleUnmarshalYAML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected Role
		wantErr  bool
	}{
		{
			name:     "ECSRole",
			input:    "ecs",
			expected: RoleECS,
			wantErr:  false,
		},
		{
			name:     "InvalidRole",
			input:    "invalid",
			expected: "invalid",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r Role
			err := r.UnmarshalYAML(func(v any) error {
				ptr, ok := v.(*string)
				if !ok {
					return errors.New("not a string pointer")
				}
				*ptr = tt.input
				return nil
			})
			if tt.wantErr {
				require.Error(t, err, "expected error for input %q", tt.input)
				return
			}
			require.NoError(t, err, "unexpected error for input %q", tt.input)
			require.Equal(t, tt.expected, r, "unexpected role for input %q", tt.input)
		})
	}
}

func TestSDConfigName(t *testing.T) {
	t.Parallel()
	cfg := &SDConfig{}
	require.Equal(t, "alibabacloud", cfg.Name())
}

func TestRoleString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		role     Role
		expected string
	}{
		{
			name:     "ECS",
			role:     RoleECS,
			expected: "ecs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.role.String())
		})
	}
}

func TestCredentialConfigConvert(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    *CredentialConfig
		expected *credentials.Config
	}{
		{
			name:     "Nil",
			input:    nil,
			expected: nil,
		},
		{
			name: "STS",
			input: &CredentialConfig{
				Type:            new(CredentialTypeSTS),
				AccessKeyId:     new(config.Secret("access-key-id")),
				AccessKeySecret: new(config.Secret("access-key-secret")),
				BearerToken:     new(config.Secret("token")),
				STSEndpoint:     new("endpoint"),
			},
			expected: &credentials.Config{
				Type:            new("sts"),
				AccessKeyId:     new("access-key-id"),
				AccessKeySecret: new("access-key-secret"),
				BearerToken:     new("token"),
				STSEndpoint:     new("endpoint"),
			},
		},
		{
			name: "RAMRoleARN",
			input: &CredentialConfig{
				Type:                  new(CredentialTypeRAMRoleARN),
				RoleArn:               new("role-arn"),
				RoleSessionName:       new("session-name"),
				RoleSessionExpiration: new(60),
			},
			expected: &credentials.Config{
				Type:                  new("ram_role_arn"),
				RoleArn:               new("role-arn"),
				RoleSessionName:       new("session-name"),
				RoleSessionExpiration: new(60),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.input.Convert()
			require.Equal(t, tt.expected, actual, "unexpected role for input %q", tt.input)
		})
	}
}

func TestSDConfigUnmarshalYAML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		yaml         string
		wantErr      bool
		validateFunc func(t *testing.T, cfg *SDConfig)
	}{
		{
			name: "ECSWithFlatFields",
			yaml: `role: ecs
region: cn-beijing
endpoint: ecs.aliyuncs.com
port: 9100
tags:
  - key: ack.aliyun.com
    value: cd386715790e44917bxxxxxxxb7e782e2`,
			validateFunc: func(t *testing.T, cfg *SDConfig) {
				require.Equal(t, RoleECS, cfg.Role)
				require.NotNil(t, cfg.ECSSDConfig)
				require.Equal(t, "cn-beijing", cfg.ECSSDConfig.Region)
				require.Equal(t, "ecs.aliyuncs.com", cfg.ECSSDConfig.Endpoint)
				require.Equal(t, 9100, cfg.ECSSDConfig.Port)
				require.Len(t, cfg.ECSSDConfig.Tags, 1)
				require.Equal(t, "ack.aliyun.com", cfg.ECSSDConfig.Tags[0].Key)
				require.Equal(t, "cd386715790e44917bxxxxxxxb7e782e2", cfg.ECSSDConfig.Tags[0].Value)
			},
		},
		{
			name: "ECSWithCredentialFields",
			yaml: `role: ecs
region: cn-beijing
port: 9100
credential:
  type: access_key
  access_key_id: access-key-id
  access_key_secret: access-key-secret`,
			validateFunc: func(t *testing.T, cfg *SDConfig) {
				require.Equal(t, RoleECS, cfg.Role)
				require.NotNil(t, cfg.ECSSDConfig)
				require.Equal(t, "cn-beijing", cfg.ECSSDConfig.Region)
				require.Equal(t, 9100, cfg.ECSSDConfig.Port)
				require.NotNil(t, cfg.ECSSDConfig.CredentialConfig)
				require.NotNil(t, cfg.ECSSDConfig.CredentialConfig.Type)
				require.Equal(t, CredentialTypeAccessKey, *cfg.ECSSDConfig.CredentialConfig.Type)
				require.NotNil(t, cfg.ECSSDConfig.CredentialConfig.AccessKeyId)
				require.Equal(t, config.Secret("access-key-id"), *cfg.CredentialConfig.AccessKeyId)
				require.NotNil(t, cfg.ECSSDConfig.CredentialConfig.AccessKeySecret)
				require.Equal(t, config.Secret("access-key-secret"), *cfg.CredentialConfig.AccessKeySecret)
			},
		},
		{
			name:    "InvalidRole",
			yaml:    `role: invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg SDConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &cfg)
			if tt.wantErr {
				require.Error(t, err, "expected error for yaml: %q", tt.yaml)
				return
			}
			require.NoError(t, err)
			tt.validateFunc(t, &cfg)
		})
	}
}

func TestNewDiscovery(t *testing.T) {
	t.Parallel()

	refreshMetrics := discovery.NewRefreshMetrics(prometheus.NewRegistry())
	alibabacloudMetrics := DefaultSDConfig.NewDiscovererMetrics(nil, refreshMetrics)

	tests := []struct {
		name    string
		conf    *SDConfig
		opts    discovery.DiscovererOptions
		wantErr bool
	}{
		{
			name: "InvalidDiscoveryMetrics",
			conf: &SDConfig{
				Role:   RoleECS,
				Region: "cn-beijing",
				ECSSDConfig: &ECSSDConfig{
					Region: "cn-beijing",
				},
			},
			wantErr: true,
		},
		{
			name: "ValidDiscoveryMetrics",
			conf: &SDConfig{
				Role:   RoleECS,
				Region: "cn-beijing",
				ECSSDConfig: &ECSSDConfig{
					Region: "cn-beijing",
				},
			},
			opts: discovery.DiscovererOptions{
				Metrics: alibabacloudMetrics,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discoverer, err := tt.conf.NewDiscoverer(tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, discoverer)
		})
	}
}
