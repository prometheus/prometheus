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
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/httputil"
)

// Client allows sending batches of Prometheus samples to an HTTP endpoint.
type Client struct {
	index   int // Used to differentiate metrics.
	url     config.URL
	client  *http.Client
	timeout time.Duration
}

// NewClient creates a new Client.
func NewClient(index int, conf *config.RemoteWriteConfig) (*Client, error) {
	tlsConfig, err := httputil.NewTLSConfig(conf.TLSConfig)
	if err != nil {
		return nil, err
	}

	// The only timeout we care about is the configured push timeout.
	// It is applied on request. So we leave out any timings here.
	var rt http.RoundTripper = &http.Transport{
		Proxy:           http.ProxyURL(conf.ProxyURL.URL),
		TLSClientConfig: tlsConfig,
	}

	if conf.BasicAuth != nil {
		rt = httputil.NewBasicAuthRoundTripper(conf.BasicAuth.Username, conf.BasicAuth.Password, rt)
	}

	return &Client{
		index:   index,
		url:     *conf.URL,
		client:  httputil.NewClient(rt),
		timeout: time.Duration(conf.RemoteTimeout),
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

	httpReq, err := http.NewRequest("POST", c.url.String(), &buf)
	if err != nil {
		return err
	}
	httpReq.Header.Add("Content-Encoding", "snappy")

	ctx, _ := context.WithTimeout(context.Background(), c.timeout)
	httpResp, err := ctxhttp.Do(ctx, c.client, httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode/100 != 2 {
		return fmt.Errorf("server returned HTTP status %s", httpResp.Status)
	}
	return nil
}

// Name identifies the client.
func (c Client) Name() string {
	return fmt.Sprintf("%d:%s", c.index, c.url)
}
