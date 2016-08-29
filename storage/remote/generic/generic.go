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
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/prometheus/common/model"
)

// Client allows sending batches of Prometheus samples to a http endpoint.
type Client struct {
	conn    *grpc.ClientConn
	timeout time.Duration
}

// NewClient creates a new Client.
func NewClient(address string, timeout time.Duration) *Client {
	// TODO: Do something with this error.
	conn, _ := grpc.Dial(address, grpc.WithInsecure())
	return &Client{
		conn:    conn,
		timeout: timeout,
	}
}

// Store sends a batch of samples to the http endpoint.
func (c *Client) Store(samples model.Samples) error {
	req := &GenericWriteRequest{}
	for _, s := range samples {
		ts := &TimeSeries{
			Name: string(s.Metric[model.MetricNameLabel]),
		}
		for k, v := range s.Metric {
			if k != model.MetricNameLabel {
				ts.Labels = append(ts.Labels,
					&LabelPair{
						Name:  string(k),
						Value: string(v),
					})
			}
		}
		ts.Samples = []*Sample{
			&Sample{
				Value:       float64(s.Value),
				TimestampMs: int64(s.Timestamp),
			},
		}
		req.Timeseries = append(req.Timeseries, ts)
	}
	client := NewGenericWriteClient(c.conn)
	ctxt, _ := context.WithTimeout(context.Background(), c.timeout)
	_, err := client.Write(ctxt, req)
	if err != nil {
		return err
	}
	return nil
}

// Name identifies the client as a genric client.
func (c Client) Name() string {
	return "generic"
}
