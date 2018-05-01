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

package warp10

import (
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	w "github.com/miton18/go-warp10/base"
	"github.com/prometheus/common/model"
)

const (
	updateEndpoint = "/api/v0/update"
)

// Client is a Warp10 client
type Client struct {
	client  *w.Client
	logger  log.Logger
	timeout time.Duration
}

// NewClient return a new Warp10 client
func NewClient(logger log.Logger, host, token string, timeout time.Duration) *Client {
	c := &Client{
		logger:  logger,
		timeout: timeout,
		client:  w.NewClient(host + updateEndpoint),
	}
	c.client.WriteToken = token
	return c
}

// Name identifies the client as a Warp10 client.
func (c Client) Name() string {
	return "warp10"
}

// Write sends a batch of samples to Warp10 via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	batch := w.GTSList{}

	for _, sample := range samples {
		gts := &w.GTS{
			ClassName: string(sample.Metric[model.MetricNameLabel]),
			Labels:    w.Labels{},
			Values: w.Datapoints{{
				sample.Timestamp.UnixNano() / 1e3, float64(sample.Value),
			}},
		}
		for k, v := range sample.Metric {
			if k == model.MetricNameLabel {
				continue
			}

			gts.Labels[string(k)] = string(v)
		}

		batch = append(batch, gts)
	}

	if err := c.client.Update(batch); err != nil {
		fmt.Println(err.Error())
		return err
	}
	return nil
}
