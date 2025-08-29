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
	"errors"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRoleUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Role
		wantErr  bool
	}{
		{
			name:     "EC2Role",
			input:    "ec2",
			expected: RoleEC2,
			wantErr:  false,
		},
		{
			name:     "LightsailRole",
			input:    "lightsail",
			expected: RoleLightsail,
			wantErr:  false,
		},
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
			} else {
				require.NoError(t, err, "unexpected error for input %q", tt.input)
				require.Equal(t, tt.expected, r, "unexpected role for input %q", tt.input)
			}
		})
	}
}

func TestRoleString(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected string
	}{
		{
			name:     "EC2",
			role:     RoleEC2,
			expected: "ec2",
		},
		{
			name:     "Lightsail",
			role:     RoleLightsail,
			expected: "lightsail",
		},
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

func TestSDConfigName(t *testing.T) {
	cfg := &SDConfig{}
	require.Equal(t, "aws", cfg.Name())
}

func TestDefaultSDConfig(t *testing.T) {
	require.Equal(t, Role(""), DefaultSDConfig.Role)
	require.Equal(t, model.Duration(60*time.Second), DefaultSDConfig.RefreshInterval)
}

func TestSDConfigUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		validateFunc func(t *testing.T, cfg *SDConfig)
	}{
		{
			name: "EC2WithFlatFields",
			yaml: `role: ec2
region: us-west-2
port: 9100
filters:
  - name: instance-state-name
    values: [running]`,
			validateFunc: func(t *testing.T, cfg *SDConfig) {
				require.Equal(t, RoleEC2, cfg.Role)
				require.NotNil(t, cfg.EC2SDConfig)
				require.Equal(t, "us-west-2", cfg.EC2SDConfig.Region)
				require.Equal(t, 9100, cfg.EC2SDConfig.Port)
				require.Len(t, cfg.EC2SDConfig.Filters, 1)
				require.Equal(t, "instance-state-name", cfg.EC2SDConfig.Filters[0].Name)
				require.Equal(t, []string{"running"}, cfg.EC2SDConfig.Filters[0].Values)
			},
		},
		{
			name: "ECSWithFlatFields",
			yaml: `role: ecs
region: us-east-1
port: 9200
clusters: ["some-cluster"]`,
			validateFunc: func(t *testing.T, cfg *SDConfig) {
				require.Equal(t, RoleECS, cfg.Role)
				require.NotNil(t, cfg.ECSSDConfig)
				require.Equal(t, "us-east-1", cfg.ECSSDConfig.Region)
				require.Equal(t, 9200, cfg.ECSSDConfig.Port)
				require.Equal(t, []string{"some-cluster"}, cfg.ECSSDConfig.Clusters)
			},
		},
		{
			name: "LightsailWithFlatFields",
			yaml: `role: lightsail
region: eu-central-1
port: 9300`,
			validateFunc: func(t *testing.T, cfg *SDConfig) {
				require.Equal(t, RoleLightsail, cfg.Role)
				require.NotNil(t, cfg.LightsailSDConfig)
				require.Equal(t, "eu-central-1", cfg.LightsailSDConfig.Region)
				require.Equal(t, 9300, cfg.LightsailSDConfig.Port)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg SDConfig
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &cfg))
			tt.validateFunc(t, &cfg)
		})
	}
}
