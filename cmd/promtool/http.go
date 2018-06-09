// Copyright 2015 The Prometheus Authors
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

package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
)

const DEFAULT_TIMEOUT = 2 * time.Minute

type HTTPClientConfig struct {
	ServerURL string
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, []byte, error)
	URLJoin(path string) string
}

func NewHTTPClient(cfg HTTPClientConfig) (HTTPClient, error) {
	hc, err := api.NewClient(api.Config{
		Address: cfg.ServerURL,
	})
	if err != nil {
		return nil, fmt.Errorf("error of creating http client: %s", err)
	}
	return &PrometheusHTTPClient{
		RequestTimeout: DEFAULT_TIMEOUT,
		HTTPClient:     hc,
	}, nil
}

type PrometheusHTTPClient struct {
	RequestTimeout time.Duration
	HTTPClient     api.Client
	ServerURL      string
}

func (c *PrometheusHTTPClient) Do(req *http.Request) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.RequestTimeout)
	defer cancel()
	return c.HTTPClient.Do(ctx, req)
}

func (c *PrometheusHTTPClient) URLJoin(path string) string {
	return c.HTTPClient.URL(path, nil).String()
}
