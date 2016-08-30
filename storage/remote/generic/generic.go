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
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// Client allows sending batches of Prometheus samples to a http endpoint.
type Client struct {
	url     string
	timeout time.Duration
}

// NewClient creates a new Client.
func NewClient(url string, timeout time.Duration) *Client {
	return &Client{
		url:     url,
		timeout: timeout,
	}
}

// Store sends a batch of samples to the http endpoint.
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

	client := http.Client{
		Timeout: c.timeout,
	}
	_, err = client.Post(c.url, string(expfmt.FmtProtoDelim), buf)
	if err != nil {
		return err
	}

	return nil
}

// Name identifies the client as a genric client.
func (c Client) Name() string {
	return "generic"
}
