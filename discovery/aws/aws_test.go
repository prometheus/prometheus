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
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
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

// TestMultipleSDConfigsDoNotShareState verifies that multiple AWS SD configs
// don't share the same underlying configuration object. This was a bug where
// all configs pointed to the same global default, causing port and other
// settings from one job to overwrite settings in another job.
func TestMultipleSDConfigsDoNotShareState(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		validateFunc func(t *testing.T, cfg1, cfg2 *SDConfig)
	}{
		{
			name: "EC2MultipleJobsDifferentPorts",
			yaml: `
- role: ec2
  region: us-west-2
  port: 9100
  filters:
    - name: tag:Name
      values: [host-1]
- role: ec2
  region: us-west-2
  port: 9101
  filters:
    - name: tag:Name
      values: [host-2]`,
			validateFunc: func(t *testing.T, cfg1, cfg2 *SDConfig) {
				require.Equal(t, RoleEC2, cfg1.Role)
				require.Equal(t, RoleEC2, cfg2.Role)
				require.NotNil(t, cfg1.EC2SDConfig)
				require.NotNil(t, cfg2.EC2SDConfig)

				// Verify ports are different and not shared
				require.Equal(t, 9100, cfg1.EC2SDConfig.Port)
				require.Equal(t, 9101, cfg2.EC2SDConfig.Port)

				// Verify filters are different and not shared
				require.Len(t, cfg1.EC2SDConfig.Filters, 1)
				require.Len(t, cfg2.EC2SDConfig.Filters, 1)
				require.Equal(t, []string{"host-1"}, cfg1.EC2SDConfig.Filters[0].Values)
				require.Equal(t, []string{"host-2"}, cfg2.EC2SDConfig.Filters[0].Values)

				// Most importantly: verify they're not the same pointer
				require.NotSame(t, cfg1.EC2SDConfig, cfg2.EC2SDConfig,
					"EC2SDConfig objects should not share the same memory address")
			},
		},
		{
			name: "ECSMultipleJobsDifferentPorts",
			yaml: `
- role: ecs
  region: us-east-1
  port: 8080
  clusters: [cluster-a]
- role: ecs
  region: us-east-1
  port: 8081
  clusters: [cluster-b]`,
			validateFunc: func(t *testing.T, cfg1, cfg2 *SDConfig) {
				require.Equal(t, RoleECS, cfg1.Role)
				require.Equal(t, RoleECS, cfg2.Role)
				require.NotNil(t, cfg1.ECSSDConfig)
				require.NotNil(t, cfg2.ECSSDConfig)

				require.Equal(t, 8080, cfg1.ECSSDConfig.Port)
				require.Equal(t, 8081, cfg2.ECSSDConfig.Port)
				require.Equal(t, []string{"cluster-a"}, cfg1.ECSSDConfig.Clusters)
				require.Equal(t, []string{"cluster-b"}, cfg2.ECSSDConfig.Clusters)

				require.NotSame(t, cfg1.ECSSDConfig, cfg2.ECSSDConfig,
					"ECSSDConfig objects should not share the same memory address")
			},
		},
		{
			name: "LightsailMultipleJobsDifferentPorts",
			yaml: `
- role: lightsail
  region: eu-west-1
  port: 7070
- role: lightsail
  region: eu-west-1
  port: 7071`,
			validateFunc: func(t *testing.T, cfg1, cfg2 *SDConfig) {
				require.Equal(t, RoleLightsail, cfg1.Role)
				require.Equal(t, RoleLightsail, cfg2.Role)
				require.NotNil(t, cfg1.LightsailSDConfig)
				require.NotNil(t, cfg2.LightsailSDConfig)

				require.Equal(t, 7070, cfg1.LightsailSDConfig.Port)
				require.Equal(t, 7071, cfg2.LightsailSDConfig.Port)

				require.NotSame(t, cfg1.LightsailSDConfig, cfg2.LightsailSDConfig,
					"LightsailSDConfig objects should not share the same memory address")
			},
		},
		{
			name: "MSKMultipleJobsDifferentPorts",
			yaml: `
- role: msk
  region: ap-south-1
  port: 6060
  clusters: ["cluster-1"]
- role: msk
  region: ap-south-1
  port: 6061
  clusters: ["cluster-2"]`,
			validateFunc: func(t *testing.T, cfg1, cfg2 *SDConfig) {
				require.Equal(t, RoleMSK, cfg1.Role)
				require.Equal(t, RoleMSK, cfg2.Role)
				require.NotNil(t, cfg1.MSKSDConfig)
				require.NotNil(t, cfg2.MSKSDConfig)

				require.Equal(t, 6060, cfg1.MSKSDConfig.Port)
				require.Equal(t, []string{"cluster-1"}, cfg1.MSKSDConfig.Clusters)
				require.Equal(t, 6061, cfg2.MSKSDConfig.Port)
				require.Equal(t, []string{"cluster-2"}, cfg2.MSKSDConfig.Clusters)

				require.NotSame(t, cfg1.MSKSDConfig, cfg2.MSKSDConfig,
					"MSKSDConfig objects should not share the same memory address")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configs []SDConfig
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &configs))
			require.Len(t, configs, 2)
			tt.validateFunc(t, &configs[0], &configs[1])
		})
	}
}

// getRandomRegion is a helper to return a pseudo-random AWS region for testing.
func getRandomRegion() string {
	regions := []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"eu-west-1",
		"eu-west-2",
		"ap-southeast-1",
		"ap-southeast-2",
		"ap-northeast-1",
		"ap-northeast-2",
	}

	return regions[rand.IntN(len(regions))]
}

func TestLoadRegion(t *testing.T) {
	t.Run("with_env_region", func(t *testing.T) {
		randomRegion := getRandomRegion()
		t.Setenv("AWS_REGION", randomRegion)
		t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
		t.Setenv("AWS_CONFIG_FILE", "") // Ensure no config file is used
		t.Setenv("AWS_PROFILE", "")     // Ensure no profile file is used

		region, err := loadRegion(context.Background(), "")
		require.NoError(t, err)
		require.Equal(t, randomRegion, region)
	})

	t.Run("with_config_file_default_profile", func(t *testing.T) {
		randomRegion := getRandomRegion()

		// Create a temporary AWS config file
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config")

		configContent := `[default]
region = ` + randomRegion + `
`

		err := os.WriteFile(configFile, []byte(configContent), 0o644)
		require.NoError(t, err)
		defer os.Remove(configFile)

		// Set up environment to use the config file
		t.Setenv("AWS_CONFIG_FILE", configFile)
		t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
		// Clear any region environment variables to force config file usage
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_PROFILE", "") // Ensure no profile file is used
		t.Setenv("AWS_DEFAULT_REGION", "")

		region, err := loadRegion(context.Background(), "")
		require.NoError(t, err)
		require.Equal(t, randomRegion, region)
	})

	t.Run("with_config_file_named_profile", func(t *testing.T) {
		randomRegion := getRandomRegion()

		// Create a temporary AWS config file
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config")

		configContent := `[default]
region = ` + getRandomRegion() + `

[profile ` + randomRegion + `-profile]
region = ` + randomRegion + `
`

		err := os.WriteFile(configFile, []byte(configContent), 0o644)
		require.NoError(t, err)
		defer os.Remove(configFile)

		// Set up environment to use the config file
		t.Setenv("AWS_CONFIG_FILE", configFile)
		t.Setenv("AWS_PROFILE", randomRegion+"-profile")
		t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
		// Clear any region environment variables to force config file usage
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")

		region, err := loadRegion(context.Background(), "")
		require.NoError(t, err)
		require.Equal(t, randomRegion, region)
	})

	t.Run("with_specified_region", func(t *testing.T) {
		specifiedRegion := getRandomRegion()

		// Even with environment region set differently, specified region should take precedence
		t.Setenv("AWS_REGION", getRandomRegion())
		t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")

		region, err := loadRegion(context.Background(), specifiedRegion)
		require.NoError(t, err)
		require.Equal(t, specifiedRegion, region)
	})

	t.Run("imds_fallback", func(t *testing.T) {
		randomRegion := getRandomRegion()

		// Mock IMDS server that returns a region
		mockIMDS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle instance identity document (contains region info)
			if r.URL.Path == "/latest/dynamic/instance-identity/document" {
				imdsPayload := `{"region": "` + randomRegion + `"}`
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(imdsPayload))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer mockIMDS.Close()

		// Set up environment with no region but valid credentials
		// This will force fallback to IMDS
		t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
		// Unset any existing region
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		t.Setenv("AWS_CONFIG_FILE", "") // Ensure no config file is used
		t.Setenv("AWS_PROFILE", "")     // Ensure no profile file is used
		// Point IMDS to our mock server
		t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", mockIMDS.URL)

		region, err := loadRegion(context.Background(), "")
		require.NoError(t, err)
		require.Equal(t, randomRegion, region)
	})

	t.Run("imds_empty_region", func(t *testing.T) {
		// Mock IMDS server that returns empty region
		mockIMDS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle instance identity document with empty region
			if r.URL.Path == "/latest/dynamic/instance-identity/document" {
				imdsPayload := `{"region": ""}`
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(imdsPayload))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer mockIMDS.Close()

		// Set up environment with no region but valid credentials
		t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
		// Unset any existing region
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		t.Setenv("AWS_CONFIG_FILE", "") // Ensure no config file is used
		t.Setenv("AWS_PROFILE", "")     // Ensure no profile file is used
		// Point IMDS to our mock server
		t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", mockIMDS.URL)

		_, err := loadRegion(context.Background(), "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get region from IMDS")
	})
}
