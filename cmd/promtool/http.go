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

type httpClientConfig struct {
	serverURL string
}

type httpClient interface {
	do(req *http.Request) (*http.Response, []byte, error)
	urlJoin(path string) string
}

func newHTTPClient(cfg httpClientConfig) (httpClient, error) {
	hc, err := api.NewClient(api.Config{
		Address: cfg.serverURL,
	})
	if err != nil {
		return nil, fmt.Errorf("error of creating http client: %s", err)
	}
	return &prometheusHTTPClient{
		requestTimeout: DEFAULT_TIMEOUT,
		httpClient:     hc,
	}, nil
}

type prometheusHTTPClient struct {
	requestTimeout time.Duration
	httpClient     api.Client
}

func (c *prometheusHTTPClient) do(req *http.Request) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()
	return c.httpClient.Do(ctx, req)
}

func (c *prometheusHTTPClient) urlJoin(path string) string {
	return c.httpClient.URL(path, nil).String()
}
