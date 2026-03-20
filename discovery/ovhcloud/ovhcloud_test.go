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

package ovhcloud

import (
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery"
)

var (
	ovhcloudApplicationKeyTest    = "TDPKJdwZwAQPwKX2"
	ovhcloudApplicationSecretTest = config.Secret("9ufkBmLaTQ9nz5yMUlg79taH0GNnzDjk")
	ovhcloudConsumerKeyTest       = config.Secret("5mBuy6SUQcRw2ZUxg0cG68BoDKpED4KY")
)

const (
	mockURL = "https://localhost:1234"
)

func getMockConf(service string) (SDConfig, error) {
	confString := fmt.Sprintf(`
endpoint: %s
application_key: %s
application_secret: %s
consumer_key: %s
refresh_interval: 1m
service: %s
`, mockURL, ovhcloudApplicationKeyTest, ovhcloudApplicationSecretTest, ovhcloudConsumerKeyTest, service)

	return getMockConfFromString(confString)
}

func getMockConfFromString(confString string) (SDConfig, error) {
	var conf SDConfig
	err := yaml.UnmarshalStrict([]byte(confString), &conf)
	return conf, err
}

func TestErrorInitClient(t *testing.T) {
	confString := fmt.Sprintf(`
endpoint: %s

`, mockURL)

	conf, _ := getMockConfFromString(confString)

	_, err := createClient(&conf)

	require.ErrorContains(t, err, "missing authentication information")
}

func TestParseIPs(t *testing.T) {
	testCases := []struct {
		name  string
		input []string
		want  error
	}{
		{
			name:  "Parse IPv4 failed.",
			input: []string{"A.b"},
			want:  errors.New("could not parse IP addresses from list"),
		},
		{
			name:  "Parse unspecified failed.",
			input: []string{"0.0.0.0"},
			want:  errors.New("could not parse IP addresses from list"),
		},
		{
			name:  "Parse void IP failed.",
			input: []string{""},
			want:  errors.New("could not parse IP addresses from list"),
		},
		{
			name:  "Parse IPv6 ok.",
			input: []string{"2001:0db8:0000:0000:0000:0000:0000:0001"},
			want:  nil,
		},
		{
			name:  "Parse IPv6 failed.",
			input: []string{"bbb:cccc:1111"},
			want:  errors.New("could not parse IP addresses from list"),
		},
		{
			name:  "Parse IPv4 bad mask.",
			input: []string{"192.0.2.1/23"},
			want:  errors.New("could not parse IP addresses from list"),
		},
		{
			name:  "Parse IPv4 ok.",
			input: []string{"192.0.2.1/32"},
			want:  nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseIPList(tc.input)
			require.Equal(t, tc.want, err)
		})
	}
}

func TestDiscoverer(t *testing.T) {
	conf, _ := getMockConf("vps")
	logger := promslog.NewNopLogger()

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := conf.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	_, err := conf.NewDiscoverer(discovery.DiscovererOptions{
		Logger:  logger,
		Metrics: metrics,
	})

	require.NoError(t, err)
}
