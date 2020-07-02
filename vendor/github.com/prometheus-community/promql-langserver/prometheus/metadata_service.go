// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.  // You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/blang/semver"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

var (
	// defining this global variable will avoid to initialized it each time
	// and it will crash immediately the server during the initialization in case the version is not well defined
	requiredVersion = semver.MustParse("2.15.0") // nolint: gochecknoglobals
)

func buildGenericRoundTripper(connectionTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   connectionTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 30 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, // nolint: gas, gosec
	}
}

func buildStatusRequest(prometheusURL string) (*http.Request, error) {
	finalURL, err := url.Parse(prometheusURL)
	if err != nil {
		return nil, err
	}

	// define the path of the buildInfo
	// using this way will remove any issue that could be caused by a wrong URL set by the user
	finalURL.Path = "/api/v1/status/buildinfo"

	httpRequest, err := http.NewRequest(http.MethodGet, finalURL.String(), nil)
	if err != nil {
		return nil, err
	}
	// set the accept content type
	httpRequest.Header.Set("Accept", "application/json")
	return httpRequest, nil
}

type buildInfoResponse struct {
	Status    string        `json:"status"`
	Data      buildInfoData `json:"data,omitempty"`
	ErrorType string        `json:"errorType,omitempty"`
	Error     string        `json:"error,omitempty"`
	Warnings  []string      `json:"warnings,omitempty"`
}

// buildInfoData contains build information about Prometheus.
type buildInfoData struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	BuildUser string `json:"buildUser"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

// MetadataService is a light prometheus client used by LSP to get metric or label information from prometheus Server.
type MetadataService interface {
	// MetricMetadata returns the first occurrence of metadata about metrics currently scraped by the metric name.
	MetricMetadata(ctx context.Context, metric string) (v1.Metadata, error)
	// AllMetricMetadata returns metadata about metrics currently scraped for all existing metrics.
	AllMetricMetadata(ctx context.Context) (map[string][]v1.Metadata, error)
	// LabelNames returns all the unique label names present in the block in sorted order.
	// If a metric is provided, then it will return all unique label names linked to the metric during a predefined period of time
	LabelNames(ctx context.Context, metricName string) ([]string, error)
	// LabelValues performs a query for the values of the given label.
	LabelValues(ctx context.Context, label string) ([]model.LabelValue, error)
	// ChangeDataSource is used if the prometheusURL is changing.
	// The client should re init its own parameter accordingly if necessary
	ChangeDataSource(prometheusURL string) error
	// SetLookbackInterval is a method to use to change the interval that then will be used to retrieve data such as label and metrics from prometheus.
	SetLookbackInterval(interval time.Duration)
	// GetURL is returning the url used to contact the prometheus server
	// In case the instance is used directly in Prometheus, it should be the externalURL
	GetURL() string
}

// httpClient is an implementation of the interface Client.
// You should use this instance directly and not the other one (compatibleHTTPClient and notCompatibleHTTPClient)
// because it will manage which sub instance of the Client to use (like a factory).
type httpClient struct {
	MetadataService
	requestTimeout   time.Duration
	mutex            sync.RWMutex
	subClient        MetadataService
	url              string
	lookbackInterval time.Duration
}

func NewClient(prometheusURL string, lookbackInterval time.Duration) (MetadataService, error) {
	c := &httpClient{
		requestTimeout:   30,
		lookbackInterval: lookbackInterval,
	}
	if err := c.ChangeDataSource(prometheusURL); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *httpClient) MetricMetadata(ctx context.Context, metric string) (v1.Metadata, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.subClient.MetricMetadata(ctx, metric)
}

func (c *httpClient) AllMetricMetadata(ctx context.Context) (map[string][]v1.Metadata, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.subClient.AllMetricMetadata(ctx)
}

func (c *httpClient) LabelNames(ctx context.Context, name string) ([]string, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.subClient.LabelNames(ctx, name)
}

func (c *httpClient) LabelValues(ctx context.Context, label string) ([]model.LabelValue, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.subClient.LabelValues(ctx, label)
}

func (c *httpClient) GetURL() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.url
}

func (c *httpClient) SetLookbackInterval(interval time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.lookbackInterval = interval
	c.subClient.SetLookbackInterval(interval)
}

func (c *httpClient) ChangeDataSource(prometheusURL string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// in case the url given is the same as before, we don't need to initialize again the client
	if c.url == prometheusURL && c.subClient != nil {
		return nil
	}
	c.url = prometheusURL
	if len(prometheusURL) == 0 {
		// having an empty URL is a valid use case. So we should just initialized a "fake" http client
		c.subClient = &emptyHTTPClient{}
		return nil
	}
	prometheusHTTPClient, err := api.NewClient(api.Config{
		RoundTripper: buildGenericRoundTripper(c.requestTimeout * time.Second),
		Address:      prometheusURL,
	})
	if err != nil {
		// always initialized the sub client to avoid any nil pointer usage
		if c.subClient == nil {
			c.subClient = &emptyHTTPClient{}
		}
		return err
	}

	isCompatible, err := c.isCompatible(prometheusURL)
	if err != nil {
		// always initialized the sub client to avoid any nil pointer usage
		if c.subClient == nil {
			c.subClient = &emptyHTTPClient{}
		}
		return err
	}
	if isCompatible {
		c.subClient = &compatibleHTTPClient{
			prometheusClient: v1.NewAPI(prometheusHTTPClient),
			lookbackInterval: c.lookbackInterval,
		}
	} else {
		c.subClient = &notCompatibleHTTPClient{
			prometheusClient: v1.NewAPI(prometheusHTTPClient),
			lookbackInterval: c.lookbackInterval,
		}
	}

	return nil
}

func (c *httpClient) isCompatible(prometheusURL string) (bool, error) {
	httpRequest, err := buildStatusRequest(prometheusURL)
	if err != nil {
		return false, err
	}
	httpClient := &http.Client{
		Transport: buildGenericRoundTripper(c.requestTimeout * time.Second),
		Timeout:   c.requestTimeout * time.Second,
	}
	resp, err := httpClient.Do(httpRequest)
	if err != nil {
		return false, err
	}

	// For prometheus version less than 2.14 `api/v1/status/buildinfo` was not supported this can
	// break many function which solely depends on version comparing like `hover`, etc.
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	defer resp.Body.Close()
	if resp.Body != nil {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}
		jsonResponse := buildInfoResponse{}
		err = json.Unmarshal(data, &jsonResponse)
		if err != nil {
			return false, err
		}
		currentVersion, err := semver.New(jsonResponse.Data.Version)
		if err != nil {
			return false, err
		}
		return currentVersion.GTE(requiredVersion), nil
	}
	return false, nil
}
