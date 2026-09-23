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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestECSSDConfigName(t *testing.T) {
	conf := DefaultECSSDConfig
	require.Equal(t, "aliyun_ecs", conf.Name())
}

func TestECSSDConfigUnmarshalYAML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		yaml         string
		wantErr      bool
		validateFunc func(t *testing.T, cfg *ECSSDConfig)
	}{
		{
			name: "WithFlatFields",
			yaml: `region: cn-beijing
endpoint: ecs.aliyuncs.com
port: 9100
tags:
  - key: ack.aliyun.com
    value: cd386715790e44917bxxxxxxxb7e782e2`,
			validateFunc: func(t *testing.T, cfg *ECSSDConfig) {
				require.NotNil(t, cfg)
				require.Equal(t, "cn-beijing", cfg.Region)
				require.Equal(t, "ecs.aliyuncs.com", cfg.Endpoint)
				require.Equal(t, 9100, cfg.Port)
				require.Len(t, cfg.Tags, 1)
				require.Equal(t, "ack.aliyun.com", cfg.Tags[0].Key)
				require.Equal(t, "cd386715790e44917bxxxxxxxb7e782e2", cfg.Tags[0].Value)
			},
		},
		{
			name: "WithCredentialFields",
			yaml: `region: cn-beijing
port: 9100
credential:
  type: access_key
  access_key_id: access-key-id
  access_key_secret: access-key-secret`,
			validateFunc: func(t *testing.T, cfg *ECSSDConfig) {
				require.NotNil(t, cfg)
				require.Equal(t, "cn-beijing", cfg.Region)
				require.Equal(t, 9100, cfg.Port)
				require.NotNil(t, cfg.CredentialConfig)
				require.NotNil(t, cfg.CredentialConfig.Type)
				require.Equal(t, CredentialTypeAccessKey, *cfg.CredentialConfig.Type)
				require.NotNil(t, cfg.CredentialConfig.AccessKeyId)
				require.Equal(t, config.Secret("access-key-id"), *cfg.CredentialConfig.AccessKeyId)
				require.NotNil(t, cfg.CredentialConfig.AccessKeySecret)
				require.Equal(t, config.Secret("access-key-secret"), *cfg.CredentialConfig.AccessKeySecret)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg ECSSDConfig
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

func TestNewECSDiscovery(t *testing.T) {
	t.Parallel()

	refreshMetrics := discovery.NewRefreshMetrics(prometheus.NewRegistry())
	ecsMetrics := DefaultECSSDConfig.NewDiscovererMetrics(nil, refreshMetrics)

	tests := []struct {
		name    string
		conf    *ECSSDConfig
		opts    discovery.DiscovererOptions
		wantErr bool
	}{
		{
			name: "InvalidDiscoveryMetrics",
			conf: &ECSSDConfig{
				Region: "cn-beijing",
			},
			wantErr: true,
		},
		{
			name: "ValidDiscoveryMetrics",
			conf: &ECSSDConfig{
				Region: "cn-beijing",
			},
			opts: discovery.DiscovererOptions{
				Metrics: ecsMetrics,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discoverer, err := NewECSDiscovery(tt.conf, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, discoverer)
		})
	}
}
