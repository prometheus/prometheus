// Copyright 2025 The Prometheus Authors
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

package upcloud

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery"
)

type invalidMetricsType struct{}

func (*invalidMetricsType) Register() error {
	return nil
}

func (*invalidMetricsType) Unregister() {
}

type mockRefreshMetricsInstantiator struct{}

func (*mockRefreshMetricsInstantiator) Instantiate(_ string) *discovery.RefreshMetrics {
	return &discovery.RefreshMetrics{}
}

func TestNewDiscovery(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		name        string
		config      *SDConfig
		metrics     discovery.DiscovererMetrics
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			config: &SDConfig{
				Port:             9100,
				RefreshInterval:  model.Duration(30 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			metrics: &upcloudMetrics{
				refreshMetrics: &mockRefreshMetricsInstantiator{},
			},
			wantErr: false,
		},
		{
			name: "invalid metrics type",
			config: &SDConfig{
				Port:             9100,
				RefreshInterval:  model.Duration(30 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			metrics:     &invalidMetricsType{},
			wantErr:     true,
			errContains: "invalid discovery metrics type",
		},
		{
			name: "invalid TLS config",
			config: &SDConfig{
				Port:            9100,
				RefreshInterval: model.Duration(30 * time.Second),
				HTTPClientConfig: config.HTTPClientConfig{
					TLSConfig: config.TLSConfig{
						CertFile: "/nonexistent/cert.pem",
						KeyFile:  "/nonexistent/key.pem",
					},
				},
			},
			metrics: &upcloudMetrics{
				refreshMetrics: &mockRefreshMetricsInstantiator{},
			},
			wantErr: true,
		},
		{
			name: "default port",
			config: &SDConfig{
				RefreshInterval:  model.Duration(60 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			metrics: &upcloudMetrics{
				refreshMetrics: &mockRefreshMetricsInstantiator{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("UPCLOUD_TOKEN", "test-token")
			discovery, err := NewDiscovery(tt.config, logger, tt.metrics)

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, discovery)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, discovery)
			require.NotNil(t, discovery.client)
			require.NotNil(t, discovery.Discovery)

			expectedPort := tt.config.Port
			if expectedPort == 0 {
				expectedPort = DefaultSDConfig.Port
			}
			require.Equal(t, expectedPort, discovery.port)
		})
	}
}

func TestDiscovery_refresh(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(*MockUpCloudClient)
		expectedTargets   int
		expectedLabels    map[string]model.LabelValue
		wantErr           bool
		errContains       string
		expectedAddresses []string
	}{
		{
			name: "successful discovery with multiple servers",
			setupMock: func(mock *MockUpCloudClient) {
				servers := CreateTestServers()
				serverDetails := CreateTestServerDetails()
				mock.SetServers(servers)
				for uuid, details := range serverDetails {
					mock.SetServerDetails(uuid, details)
				}
			},
			expectedTargets: 3,
			expectedLabels: map[string]model.LabelValue{
				"__meta_upcloud_server_uuid":              "00798b85-efdc-41ca-8021-f6ef457b8531",
				"__meta_upcloud_server_title":             "web-server-1",
				"__meta_upcloud_server_hostname":          "web1.example.com",
				"__meta_upcloud_server_zone":              "fi-hel1",
				"__meta_upcloud_server_plan":              "1xCPU-1GB",
				"__meta_upcloud_server_state":             "started",
				"__meta_upcloud_server_core_number":       "1",
				"__meta_upcloud_server_memory_amount":     "1024",
				"__meta_upcloud_server_public_ipv4":       "192.0.2.1",
				"__meta_upcloud_server_private_ipv4":      "10.0.0.1",
				"__meta_upcloud_server_public_ipv6":       "2001:db8::1",
				"__meta_upcloud_server_tag_production":    "true",
				"__meta_upcloud_server_tag_web":           "true",
				"__meta_upcloud_server_label_environment": "production",
				"__meta_upcloud_server_label_team":        "platform",
			},
			expectedAddresses: []string{"192.0.2.1:80", "192.0.2.2:80", "10.0.0.3:80"},
		},
		{
			name: "GetServers API error",
			setupMock: func(mock *MockUpCloudClient) {
				mock.SetError(errors.New("API error"))
			},
			wantErr:     true,
			errContains: "API error",
		},
		{
			name: "empty server list",
			setupMock: func(mock *MockUpCloudClient) {
				mock.SetServers(CreateEmptyServers())
			},
			expectedTargets: 0,
		},
		{
			name: "server without IP addresses",
			setupMock: func(mock *MockUpCloudClient) {
				servers := CreateServersWithoutIPs()
				serverDetails := CreateServerDetailsWithoutIPs()
				mock.SetServers(servers)
				for uuid, details := range serverDetails {
					mock.SetServerDetails(uuid, details)
				}
			},
			expectedTargets: 0, // Should skip servers without IPs
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("UPCLOUD_TOKEN", "test-token")
			mock := NewMockUpCloudClient()
			tt.setupMock(mock)

			d := &Discovery{
				client: mock,
				port:   80,
			}

			ctx := context.Background()
			tgs, err := d.refresh(ctx)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.Len(t, tgs, 1)

			tg := tgs[0]
			require.Equal(t, "UpCloud", tg.Source)
			require.Len(t, tg.Targets, tt.expectedTargets)

			if tt.expectedTargets > 0 && tt.expectedLabels != nil {
				target := tg.Targets[0]
				for labelName, expectedValue := range tt.expectedLabels {
					actualValue, exists := target[model.LabelName(labelName)]
					require.True(t, exists, "Label %s should exist", labelName)
					require.Equal(t, expectedValue, actualValue, "Label %s mismatch", labelName)
				}
			}

			if len(tt.expectedAddresses) > 0 {
				for i, expectedAddr := range tt.expectedAddresses {
					require.Equal(t, model.LabelValue(expectedAddr), tg.Targets[i][model.AddressLabel])
				}
			}
		})
	}
}

func TestSDConfig_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *SDConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid minimal config",
			input: `
refresh_interval: 30s
port: 9090
`,
			expected: &SDConfig{
				RefreshInterval:  model.Duration(30 * time.Second),
				Port:             9090,
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
		},
		{
			name:     "valid config with defaults",
			input:    `{}`,
			expected: &DefaultSDConfig,
		},
		{
			name: "invalid refresh interval",
			input: `
refresh_interval: invalid
port: 9090
`,
			wantErr: true,
		},
		{
			name: "invalid port type",
			input: `
refresh_interval: 30s
port: "not_a_number"
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config SDConfig
			err := yaml.Unmarshal([]byte(tt.input), &config)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			if tt.expected != nil {
				require.Equal(t, tt.expected.RefreshInterval, config.RefreshInterval)
				require.Equal(t, tt.expected.Port, config.Port)
			}
		})
	}
}

func TestSDConfig_Methods(t *testing.T) {
	t.Run("Name", func(t *testing.T) {
		config := &SDConfig{}
		require.Equal(t, "upcloud", config.Name())
	})

	t.Run("NewDiscovererMetrics", func(t *testing.T) {
		config := &SDConfig{}
		metrics := config.NewDiscovererMetrics(nil, &mockRefreshMetricsInstantiator{})
		require.NotNil(t, metrics)
		require.IsType(t, &upcloudMetrics{}, metrics)
	})

	t.Run("SetDirectory", func(t *testing.T) {
		config := &SDConfig{
			HTTPClientConfig: config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					CertFile: "cert.pem",
					KeyFile:  "key.pem",
				},
			},
		}

		config.SetDirectory("/test/dir")
		require.Equal(t, filepath.Join("/test/dir", "cert.pem"), config.HTTPClientConfig.TLSConfig.CertFile)
		require.Equal(t, filepath.Join("/test/dir", "key.pem"), config.HTTPClientConfig.TLSConfig.KeyFile)
	})
}

func TestDiscovery_IPAddressPriority(t *testing.T) {
	tests := []struct {
		name               string
		setupServerDetails func() map[string]*upcloud.ServerDetails
		expectedAddress    string
	}{
		{
			name: "prefers public IPv4",
			setupServerDetails: func() map[string]*upcloud.ServerDetails {
				details := CreateTestServerDetails()
				return map[string]*upcloud.ServerDetails{
					"test-server": details["00798b85-efdc-41ca-8021-f6ef457b8531"],
				}
			},
			expectedAddress: "192.0.2.1:80",
		},
		{
			name:               "falls back to public IPv6 when no IPv4",
			setupServerDetails: CreateServerDetailsWithIPv6Only,
			expectedAddress:    "[2001:db8::1]:80",
		},
		{
			name:               "falls back to private IPv4 when no public",
			setupServerDetails: CreateServerDetailsWithPrivateOnly,
			expectedAddress:    "10.0.0.1:80",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockUpCloudClient()
			serverDetails := tt.setupServerDetails()

			servers := CreateSingleTestServer()
			mock.SetServers(servers)
			for uuid, details := range serverDetails {
				mock.SetServerDetails(uuid, details)
			}

			d := &Discovery{
				client: mock,
				port:   80,
			}

			ctx := context.Background()
			tgs, err := d.refresh(ctx)
			require.NoError(t, err)
			require.Len(t, tgs, 1)
			require.Len(t, tgs[0].Targets, 1)

			target := tgs[0].Targets[0]
			require.Equal(t, model.LabelValue(tt.expectedAddress), target[model.AddressLabel])
		})
	}
}

func TestSDConfig_NewDiscoverer(t *testing.T) {
	tests := []struct {
		name        string
		config      *SDConfig
		options     discovery.DiscovererOptions
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config and options",
			config: &SDConfig{
				Port:             9100,
				RefreshInterval:  model.Duration(30 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			options: discovery.DiscovererOptions{
				Logger: slog.Default(),
				Metrics: &upcloudMetrics{
					refreshMetrics: &mockRefreshMetricsInstantiator{},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid metrics type in options",
			config: &SDConfig{
				Port:             9100,
				RefreshInterval:  model.Duration(30 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			options: discovery.DiscovererOptions{
				Logger:  slog.Default(),
				Metrics: &invalidMetricsType{},
			},
			wantErr:     true,
			errContains: "invalid discovery metrics type",
		},
		{
			name: "invalid TLS config",
			config: &SDConfig{
				Port:            9100,
				RefreshInterval: model.Duration(30 * time.Second),
				HTTPClientConfig: config.HTTPClientConfig{
					TLSConfig: config.TLSConfig{
						CertFile: "/nonexistent/cert.pem",
						KeyFile:  "/nonexistent/key.pem",
					},
				},
			},
			options: discovery.DiscovererOptions{
				Logger: slog.Default(),
				Metrics: &upcloudMetrics{
					refreshMetrics: &mockRefreshMetricsInstantiator{},
				},
			},
			wantErr: true,
		},
		{
			name: "nil logger in options",
			config: &SDConfig{
				Port:             9100,
				RefreshInterval:  model.Duration(30 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			options: discovery.DiscovererOptions{
				Logger: nil,
				Metrics: &upcloudMetrics{
					refreshMetrics: &mockRefreshMetricsInstantiator{},
				},
			},
			wantErr: false, // Should handle nil logger gracefully
		},
		{
			name: "with HTTP client options",
			config: &SDConfig{
				Port:             9100,
				RefreshInterval:  model.Duration(30 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			options: discovery.DiscovererOptions{
				Logger: slog.Default(),
				Metrics: &upcloudMetrics{
					refreshMetrics: &mockRefreshMetricsInstantiator{},
				},
				HTTPClientOptions: []config.HTTPClientOption{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("UPCLOUD_TOKEN", "test-token")
			discoverer, err := tt.config.NewDiscoverer(tt.options)

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, discoverer)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, discoverer)

			// Verify the discoverer is of the correct type
			discovery, ok := discoverer.(*Discovery)
			require.True(t, ok, "NewDiscoverer should return a *Discovery")
			require.NotNil(t, discovery.client)
			require.NotNil(t, discovery.Discovery)

			// Verify the port is set correctly
			expectedPort := tt.config.Port
			if expectedPort == 0 {
				expectedPort = DefaultSDConfig.Port
			}
			require.Equal(t, expectedPort, discovery.port)
		})
	}
}

func TestNewDiscovery_EnvironmentVariables(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		config      *SDConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid token environment variable",
			envVars: map[string]string{
				"UPCLOUD_TOKEN": "test-token-123",
			},
			config: &SDConfig{
				Port:             9100,
				RefreshInterval:  model.Duration(30 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			wantErr: false,
		},
		{
			name: "invalid token environment variable",
			envVars: map[string]string{
				"UPCLOUD_TOKEN": "",
			},
			config: &SDConfig{
				Port:             9100,
				RefreshInterval:  model.Duration(30 * time.Second),
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("UPCLOUD_TOKEN", "")
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			logger := slog.Default()
			metrics := &upcloudMetrics{
				refreshMetrics: &mockRefreshMetricsInstantiator{},
			}

			discovery, err := NewDiscovery(tt.config, logger, metrics)

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, discovery)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, discovery)
			require.NotNil(t, discovery.client)
			require.NotNil(t, discovery.Discovery)

			expectedPort := tt.config.Port
			if expectedPort == 0 {
				expectedPort = DefaultSDConfig.Port
			}
			require.Equal(t, expectedPort, discovery.port)
		})
	}
}
