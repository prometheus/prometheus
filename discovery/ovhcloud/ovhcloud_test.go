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
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
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

func initMockAuth(time int) {
	gock.New(mockURL).
		Get("/auth/time").
		Reply(200).
		JSON(time)
}

func initMockAuthDetails(time int, result map[string]string) {
	initMockAuth(time)

	gock.New(mockURL).
		MatchHeader("Accept", "application/json").
		Get("/auth/details").
		Reply(200).
		JSON(result)
}

func initErrorMockAuthDetails(err error) {
	initMockAuth(123456)

	gock.New(mockURL).
		Get("/auth/details").
		ReplyError(err)
}

func TestErrorCallAuthDetails(t *testing.T) {
	errTest := errors.New("There is an error on get /auth/details")
	initErrorMockAuthDetails(errTest)

	_, err := getMockConf("vps")
	require.ErrorIs(t, err, errTest)
	require.Equal(t, gock.IsDone(), true)
}

func TestOvhcloudRefreshFailedOnDedicatedServer(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	errTest := errors.New("error on get dedicated server list")
	initMockErrorDedicatedServerList(errTest)

	conf, err := getMockConf("dedicated_server")
	require.NoError(t, err)

	//	conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d, err := newRefresher(&conf, logger)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.ErrorContains(t, err, "error on get dedicated server list")
}

func TestOvhcloudMissingAuthArguments(t *testing.T) {
	confString := `
refresh_interval: 30s
`

	_, err := getMockConfFromString(confString)
	for _, params := range []string{"ApplicationKey", "ApplicationSecret", "ConsumerKey", "Endpoint"} {
		require.ErrorContains(t, err, fmt.Sprintf("Key: 'SDConfig.%s' Error:Field validation for '%s' failed on the 'required' tag", params, params))
	}

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestOvhcloudBadRefreshInterval(t *testing.T) {
	confString := `
refresh_interval: 30
`

	_, err := getMockConfFromString(confString)
	require.ErrorContains(t, err, "not a valid duration string")

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestOvhcloudAllArgumentsOk(t *testing.T) {
	initMockAuthDetails(132456, map[string]string{"name": "test_name"})
	_, err := getMockConf("vps")

	require.NoError(t, err)

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestBadService(t *testing.T) {
	initMockAuthDetails(1235234, map[string]string{"name": "test_name"})
	_, err := getMockConf("test")
	require.ErrorContains(t, err, "Error:Field validation for 'Service' failed on the 'oneof' tag")
}

func TestVps(t *testing.T) {
	defer gock.Off()

	vps := Vps{}

	vpsMap := map[string]VpsData{"abc": {Vps: vps, IPs: []string{"192.0.2.1", "2001:0db8:0000:0000:0000:0000:0000:0001"}}}
	initMockVps(vpsMap)

	conf, err := getMockConf("vps")

	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d, err := newRefresher(&conf, logger)
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	// 1 tgs for VPS
	require.Equal(t, 1, len(tgs))
	tgVps := tgs[0]
	require.NotNil(t, tgVps)
	require.NotNil(t, tgVps.Targets)
	require.Equal(t, "ovhcloud_vps", tgVps.Source)

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestDedicatedServer(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12352354, map[string]string{"name": "test_name"})

	dedicatedServer := DedicatedServer{}

	dedicatedServerMap := map[string]DedicatedServerData{"abc": {DedicatedServer: dedicatedServer, IPs: []string{"192.0.2.2"}}}
	initMockDedicatedServer(dedicatedServerMap)

	conf, err := getMockConf("dedicated_server")
	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d, err := newRefresher(&conf, logger)
	require.NoError(t, err)
	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	// 1 tgs for DedicatedServer
	require.Equal(t, 1, len(tgs))
	tgDedicatedServer := tgs[0]
	require.NotNil(t, tgDedicatedServer)
	require.NotNil(t, tgDedicatedServer.Targets)
	require.Equal(t, "ovhcloud_dedicated_server", tgDedicatedServer.Source)

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestErrorInitClientOnUnmarshal(t *testing.T) {
	confString := fmt.Sprintf(`
endpoint: test-fail
application_key: %s
application_secret: %s
consumer_key: %s
`, testApplicationKey, testApplicationSecret, testConsumerKey)

	conf, _ := getMockConfFromString(confString)

	_, err := conf.CreateClient()

	require.ErrorContains(t, err, "unknown endpoint")
}

func TestErrorInitClient(t *testing.T) {
	confString := fmt.Sprintf(`
endpoint: %s

`, mockURL)

	conf, _ := getMockConfFromString(confString)

	_, err := conf.CreateClient()

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
