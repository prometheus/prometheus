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
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
)

// Client allows sending batches of Prometheus samples to an HTTP endpoint.
type Client struct {
	url    string
	client http.Client
}

// NewClient creates a new Client.
func NewClient(url string, timeout time.Duration) (*Client, error) {
	return &Client{
		url: url,
		client: http.Client{
			Timeout: timeout,
		},
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
			{
				Value:       float64(s.Value),
				TimestampMs: int64(s.Timestamp),
			},
		}
		req.Timeseries = append(req.Timeseries, ts)
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	buf := bytes.Buffer{}
	if _, err := snappy.NewWriter(&buf).Write(data); err != nil {
		return err
	}

	httpReq, err := http.NewRequest("POST", c.url, &buf)
	if err != nil {
		return err
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode/100 != 2 {
		return fmt.Errorf("server returned HTTP status %s", httpResp.Status)
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
