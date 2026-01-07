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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	envoy_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
)

var userAgent = version.PrometheusUserAgent()

// ResourceClient exposes the xDS protocol for a single resource type.
// See https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#rest-json-polling-subscriptions .
type ResourceClient interface {
	// ResourceTypeURL is the type URL of the resource.
	ResourceTypeURL() string

	// Server is the xDS Management server.
	Server() string

	// Fetch requests the latest view of the entire resource state.
	// If no updates have been made since the last request, the response will be nil.
	Fetch(ctx context.Context) (*v3.DiscoveryResponse, error)

	// ID returns the ID of the client that is sent to the xDS server.
	ID() string

	// Close releases currently held resources.
	Close()
}

type HTTPResourceClient struct {
	client *http.Client
	config *HTTPResourceClientConfig

	// endpoint is the fully-constructed xDS HTTP endpoint.
	endpoint string
	// Caching.
	latestVersion string
	latestNonce   string
}

type HTTPResourceClientConfig struct {
	// HTTP config.
	config.HTTPClientConfig

	Name string

	// ExtraQueryParams are extra query parameters to attach to the request URL.
	ExtraQueryParams url.Values

	// General xDS config.

	// The timeout for a single fetch request.
	Timeout time.Duration

	// Type is the xds type, e.g., clusters
	// which is used in the discovery POST request.
	ResourceType string
	// ResourceTypeURL is the Google type url for the resource, e.g., type.googleapis.com/envoy.api.v2.Cluster.
	ResourceTypeURL string
	// Server is the xDS management server.
	Server string
	// ClientID is used to identify the client with the management server.
	ClientID string
}

func NewHTTPResourceClient(conf *HTTPResourceClientConfig, protocolVersion ProtocolVersion) (*HTTPResourceClient, error) {
	if protocolVersion != ProtocolV3 {
		return nil, errors.New("only the v3 protocol is supported")
	}

	if len(conf.Server) == 0 {
		return nil, errors.New("empty xDS server")
	}

	serverURL, err := url.Parse(conf.Server)
	if err != nil {
		return nil, err
	}

	endpointURL, err := makeXDSResourceHTTPEndpointURL(protocolVersion, serverURL, conf.ResourceType)
	if err != nil {
		return nil, err
	}

	if conf.ExtraQueryParams != nil {
		endpointURL.RawQuery = conf.ExtraQueryParams.Encode()
	}

	client, err := config.NewClientFromConfig(conf.HTTPClientConfig, conf.Name, config.WithIdleConnTimeout(conf.Timeout))
	if err != nil {
		return nil, err
	}

	client.Timeout = conf.Timeout

	return &HTTPResourceClient{
		client:        client,
		config:        conf,
		endpoint:      endpointURL.String(),
		latestVersion: "",
		latestNonce:   "",
	}, nil
}

func makeXDSResourceHTTPEndpointURL(protocolVersion ProtocolVersion, serverURL *url.URL, resourceType string) (*url.URL, error) {
	if serverURL == nil {
		return nil, errors.New("empty xDS server URL")
	}

	if len(serverURL.Scheme) == 0 || len(serverURL.Host) == 0 {
		return nil, errors.New("invalid xDS server URL")
	}

	if serverURL.Scheme != "http" && serverURL.Scheme != "https" {
		return nil, errors.New("invalid xDS server URL protocol. must be either 'http' or 'https'")
	}

	serverURL.Path = path.Join(serverURL.Path, string(protocolVersion), fmt.Sprintf("discovery:%s", resourceType))

	return serverURL, nil
}

func (rc *HTTPResourceClient) Server() string {
	return rc.config.Server
}

func (rc *HTTPResourceClient) ResourceTypeURL() string {
	return rc.config.ResourceTypeURL
}

func (rc *HTTPResourceClient) ID() string {
	return rc.config.ClientID
}

func (rc *HTTPResourceClient) Close() {
	rc.client.CloseIdleConnections()
}

// Fetch requests the latest state of the resources from the xDS server and cache the version.
// Returns a nil response if the current local version is up to date.
func (rc *HTTPResourceClient) Fetch(ctx context.Context) (*v3.DiscoveryResponse, error) {
	discoveryReq := &v3.DiscoveryRequest{
		VersionInfo:   rc.latestVersion,
		ResponseNonce: rc.latestNonce,
		TypeUrl:       rc.ResourceTypeURL(),
		ResourceNames: []string{},
		Node: &envoy_core.Node{
			Id: rc.ID(),
		},
	}

	reqBody, err := protoJSONMarshalOptions.Marshal(discoveryReq)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodPost, rc.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	request = request.WithContext(ctx)

	request.Header.Add("User-Agent", userAgent)
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")

	resp, err := rc.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotModified {
		// Empty response, already have the latest.
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non 200 status '%d' response during xDS fetch", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	discoveryRes := &v3.DiscoveryResponse{}
	if err = protoJSONUnmarshalOptions.Unmarshal(respBody, discoveryRes); err != nil {
		return nil, err
	}

	// Cache the latest nonce + version info.
	rc.latestNonce = discoveryRes.Nonce
	rc.latestVersion = discoveryRes.VersionInfo

	return discoveryRes, nil
}
