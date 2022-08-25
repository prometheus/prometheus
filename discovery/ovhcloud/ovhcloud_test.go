// Copyright 2021 The Prometheus Authors
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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/util/testutil"
)

var (
	testApplicationKey    = "appKeyTest"
	testApplicationSecret = "appSecretTest"
	testConsumerKey       = "consumerTest"
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
`, mockURL, testApplicationKey, testApplicationSecret, testConsumerKey, service)

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

	_, err := CreateClient(&conf)

	require.ErrorContains(t, err, "missing application key")
}

func TestParseIPv4Failed(t *testing.T) {
	_, err := ParseIPList([]string{"A.b"})
	require.ErrorContains(t, err, "could not parse IP addresses from list")
}

func TestParseZeroIPFailed(t *testing.T) {
	_, err := ParseIPList([]string{"0.0.0.0"})
	require.ErrorContains(t, err, "could not parse IP addresses from list")
}

func TestParseVoidIPFailed(t *testing.T) {
	_, err := ParseIPList([]string{""})
	require.ErrorContains(t, err, "could not parse IP addresses from list")
}

func TestParseIPv6Ok(t *testing.T) {
	_, err := ParseIPList([]string{"2001:0db8:0000:0000:0000:0000:0000:0001"})
	require.NoError(t, err)
}

func TestParseIPv6Failed(t *testing.T) {
	_, err := ParseIPList([]string{"bbb:cccc:1111"})
	require.ErrorContains(t, err, "could not parse IP addresses from list")
}

func TestParseIPv4BadMask(t *testing.T) {
	_, err := ParseIPList([]string{"192.0.2.1/23"})
	require.ErrorContains(t, err, "could not parse IP addresses from list")
}

func TestParseIPv4MaskOk(t *testing.T) {
	_, err := ParseIPList([]string{"192.0.2.1/32"})
	require.NoError(t, err)
}

func TestDiscoverer(t *testing.T) {
	conf, _ := getMockConf("vps")
	logger := testutil.NewLogger(t)
	_, err := conf.NewDiscoverer(discovery.DiscovererOptions{
		Logger: logger,
	})

	require.NoError(t, err)
}
