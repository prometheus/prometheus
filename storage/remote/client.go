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

package remote

import (
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/prometheus/common/model"
)

// Client allows sending batches of Prometheus samples to an HTTP endpoint.
type Client struct {
	client  WriteClient
	timeout time.Duration
}

// NewClient creates a new Client.
func NewClient(address string, timeout time.Duration) (*Client, error) {
	conn, err := grpc.Dial(
		address,
		grpc.WithInsecure(),
		grpc.WithTimeout(timeout),
		grpc.WithCompressor(&snappyCompressor{}),
	)
	if err != nil {
		// grpc.Dial() returns immediately and doesn't error when the server is
		// unreachable when not passing in the WithBlock() option. The client then
		// will continuously try to (re)establish the connection in the background.
		// So this will only return here if some other uncommon error occured.
		return nil, err
	}
	return &Client{
		client:  NewWriteClient(conn),
		timeout: timeout,
	}, nil
}

// Store sends a batch of samples to the HTTP endpoint.
func (c *Client) Store(samples model.Samples) error {
	req := &WriteRequest{
		Timeseries: make([]*TimeSeries, 0, len(samples)),
	}
	for _, s := range samples {
		ts := &TimeSeries{
			Labels: make([]*LabelPair, 0, len(s.Metric)),
		}
		for k, v := range s.Metric {
			ts.Labels = append(ts.Labels,
				&LabelPair{
					Name:  string(k),
					Value: string(v),
				})
		}
		ts.Samples = []*Sample{
			&Sample{
				Value:       float64(s.Value),
				TimestampMs: int64(s.Timestamp),
			},
		}
		req.Timeseries = append(req.Timeseries, ts)
	}

	ctxt, cancel := context.WithTimeout(context.TODO(), c.timeout)
	defer cancel()

	_, err := c.client.Write(ctxt, req)
	if err != nil {
		return err
	}
	return nil
}

// Name identifies the client as a generic client.
//
// TODO: This client is going to be the only one soon - then this method
// will simply be removed in the restructuring and the whole "generic" naming
// will be gone for good.
func (c Client) Name() string {
	return "generic"
}
