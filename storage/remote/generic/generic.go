// Copyright 2016 The Prometheus Authors
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

package generic

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// Client allows sending batches of Prometheus samples to an HTTP endpoint.
type Client struct {
	url     string
	headers http.Header
	client  http.Client
}

// NewClient creates a new Client.
func NewClient(url string, headers http.Header, timeout time.Duration) *Client {
	return &Client{
		url:     url,
		headers: headers,
		client: http.Client{
			Timeout: timeout,
		},
	}
}

// Store sends a batch of samples to the HTTP endpoint.
func (c *Client) Store(samples model.Samples) error {
	req := &GenericWriteRequest{}
	for _, s := range samples {
		ts := &TimeSeries{
			Name: proto.String(string(s.Metric[model.MetricNameLabel])),
		}
		for k, v := range s.Metric {
			if k != model.MetricNameLabel {
				ts.Labels = append(ts.Labels,
					&LabelPair{
						Name:  proto.String(string(k)),
						Value: proto.String(string(v)),
					})
			}
		}
		ts.Samples = []*Sample{
			&Sample{
				Value:       proto.Float64(float64(s.Value)),
				TimestampMs: proto.Int64(int64(s.Timestamp)),
			},
		}
		req.Timeseries = append(req.Timeseries, ts)
	}
	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(data)

	httpReq, err := http.NewRequest("POST", c.url, buf)
	for name, values := range c.headers {
		for _, val := range values {
			httpReq.Header.Add(name, val)
		}
	}
	httpReq.Header.Set("Content-Type", string(expfmt.FmtProtoDelim))
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error sending samples: %v", err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("sending samples failed with HTTP status %s", resp.Status)
	}
	return nil
}

// Name identifies the client as a generic client.
func (c Client) Name() string {
	return "generic"
}
