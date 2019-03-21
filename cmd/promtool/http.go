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
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
)

const defaultTimeout = 2 * time.Minute

type prometheusHTTPClient struct {
	requestTimeout time.Duration
	httpClient     api.Client
}

func newPrometheusHTTPClient(serverURL string) (*prometheusHTTPClient, error) {
	hc, err := api.NewClient(api.Config{
		Address: serverURL,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating HTTP client")
	}
	return &prometheusHTTPClient{
		requestTimeout: defaultTimeout,
		httpClient:     hc,
	}, nil
}

func (c *prometheusHTTPClient) do(req *http.Request) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()
	return c.httpClient.Do(ctx, req)
}

func (c *prometheusHTTPClient) urlJoin(path string) string {
	return c.httpClient.URL(path, nil).String()
}
