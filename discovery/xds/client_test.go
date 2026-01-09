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

package xds

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func testHTTPResourceConfig() *HTTPResourceClientConfig {
	return &HTTPResourceClientConfig{
		HTTPClientConfig: config.HTTPClientConfig{
			TLSConfig: config.TLSConfig{InsecureSkipVerify: true},
		},
		ResourceType: "monitoring",
		// Some known type.
		ResourceTypeURL: "type.googleapis.com/envoy.service.discovery.v3.DiscoveryRequest",
		Server:          "http://localhost",
		ClientID:        "test-id",
	}
}

func urlMustParse(str string) *url.URL {
	parsed, err := url.Parse(str)
	if err != nil {
		panic(err)
	}

	return parsed
}

func TestMakeXDSResourceHttpEndpointEmptyServerURLScheme(t *testing.T) {
	t.Parallel()

	endpointURL, err := makeXDSResourceHTTPEndpointURL(ProtocolV3, urlMustParse("127.0.0.1"), "monitoring")

	require.Empty(t, endpointURL)
	require.EqualError(t, err, "invalid xDS server URL")
}

func TestMakeXDSResourceHttpEndpointEmptyServerURLHost(t *testing.T) {
	t.Parallel()

	endpointURL, err := makeXDSResourceHTTPEndpointURL(ProtocolV3, urlMustParse("grpc://127.0.0.1"), "monitoring")

	require.Empty(t, endpointURL)
	require.ErrorContains(t, err, "must be either 'http' or 'https'")
}

func TestMakeXDSResourceHttpEndpoint(t *testing.T) {
	t.Parallel()

	endpointURL, err := makeXDSResourceHTTPEndpointURL(ProtocolV3, urlMustParse("http://127.0.0.1:5000"), "monitoring")

	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:5000/v3/discovery:monitoring", endpointURL.String())
}

func TestCreateNewHTTPResourceClient(t *testing.T) {
	t.Parallel()

	c := &HTTPResourceClientConfig{
		HTTPClientConfig: sdConf.HTTPClientConfig,
		Name:             "test",
		ExtraQueryParams: url.Values{
			"param1": {"v1"},
		},
		Timeout:         1 * time.Minute,
		ResourceType:    "monitoring",
		ResourceTypeURL: "type.googleapis.com/envoy.service.discovery.v3.DiscoveryRequest",
		Server:          "http://127.0.0.1:5000",
		ClientID:        "client",
	}

	client, err := NewHTTPResourceClient(c, ProtocolV3)

	require.NoError(t, err)

	require.Equal(t, "http://127.0.0.1:5000/v3/discovery:monitoring?param1=v1", client.endpoint)
	require.Equal(t, 1*time.Minute, client.client.Timeout)
}

func createTestHTTPResourceClient(t *testing.T, conf *HTTPResourceClientConfig, protocolVersion ProtocolVersion, responder discoveryResponder) (*HTTPResourceClient, func()) {
	s := createTestHTTPServer(t, func(request *v3.DiscoveryRequest) (*v3.DiscoveryResponse, error) {
		require.Equal(t, conf.ResourceTypeURL, request.TypeUrl)
		require.Equal(t, conf.ClientID, request.Node.Id)
		return responder(request)
	})

	conf.Server = s.URL
	client, err := NewHTTPResourceClient(conf, protocolVersion)
	require.NoError(t, err)

	return client, s.Close
}

func TestHTTPResourceClientFetchEmptyResponse(t *testing.T) {
	t.Parallel()

	client, cleanup := createTestHTTPResourceClient(t, testHTTPResourceConfig(), ProtocolV3, func(*v3.DiscoveryRequest) (*v3.DiscoveryResponse, error) {
		return nil, nil
	})
	defer cleanup()

	res, err := client.Fetch(context.Background())
	require.Nil(t, res)
	require.NoError(t, err)
}

func TestHTTPResourceClientFetchFullResponse(t *testing.T) {
	t.Parallel()

	client, cleanup := createTestHTTPResourceClient(t, testHTTPResourceConfig(), ProtocolV3, func(request *v3.DiscoveryRequest) (*v3.DiscoveryResponse, error) {
		if request.VersionInfo == "1" {
			return nil, nil
		}

		return &v3.DiscoveryResponse{
			TypeUrl:     request.TypeUrl,
			VersionInfo: "1",
			Nonce:       "abc",
			Resources:   []*anypb.Any{},
		}, nil
	})
	defer cleanup()

	res, err := client.Fetch(context.Background())
	require.NoError(t, err)
	require.NotNil(t, res)

	require.Equal(t, client.ResourceTypeURL(), res.TypeUrl)
	require.Empty(t, res.Resources)
	require.Equal(t, "abc", client.latestNonce, "Nonce not cached")
	require.Equal(t, "1", client.latestVersion, "Version not cached")

	res, err = client.Fetch(context.Background())
	require.Nil(t, res, "Update not expected")
	require.NoError(t, err)
}

func TestHTTPResourceClientServerError(t *testing.T) {
	t.Parallel()

	client, cleanup := createTestHTTPResourceClient(t, testHTTPResourceConfig(), ProtocolV3, func(*v3.DiscoveryRequest) (*v3.DiscoveryResponse, error) {
		return nil, errors.New("server error")
	})
	defer cleanup()

	res, err := client.Fetch(context.Background())
	require.Nil(t, res)
	require.Error(t, err)
}
