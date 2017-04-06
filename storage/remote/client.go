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
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/util/httputil"
)

// Client allows reading and writing from/to a remote HTTP endpoint.
type Client struct {
	index   int // Used to differentiate metrics.
	url     *config.URL
	client  *http.Client
	timeout time.Duration
}

type clientConfig struct {
	url              *config.URL
	timeout          model.Duration
	httpClientConfig config.HTTPClientConfig
}

// NewClient creates a new Client.
func NewClient(index int, conf *clientConfig) (*Client, error) {
	httpClient, err := httputil.NewClientFromConfig(conf.httpClientConfig)
	if err != nil {
		return nil, err
	}

	return &Client{
		index:   index,
		url:     conf.url,
		client:  httpClient,
		timeout: time.Duration(conf.timeout),
	}, nil
}

type recoverableError struct {
	error
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
		// Errors from NewRequest are from unparseable URLs, so are not
		// recoverable.
		return err
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.0.1")

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	httpResp, err := ctxhttp.Do(ctx, c.client, httpReq)
	if err != nil {
		// Errors from client.Do are from (for example) network errors, so are
		// recoverable.
		return recoverableError{err}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		err = fmt.Errorf("server returned HTTP status %s", httpResp.Status)
	}
	if httpResp.StatusCode/100 == 5 {
		return recoverableError{err}
	}
	return nil
}

// Name identifies the client.
func (c Client) Name() string {
	return fmt.Sprintf("%d:%s", c.index, c.url)
}

// Read reads from a remote endpoint.
func (c *Client) Read(ctx context.Context, from, through model.Time, matchers metric.LabelMatchers) (model.Matrix, error) {
	req := &ReadRequest{
		// TODO: Support batching multiple queries into one read request,
		// as the protobuf interface allows for it.
		Queries: []*Query{{
			StartTimestampMs: int64(from),
			EndTimestampMs:   int64(through),
			Matchers:         labelMatchersToProto(matchers),
		}},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal read request: %v", err)
	}

	buf := bytes.Buffer{}
	if _, err := snappy.NewWriter(&buf).Write(data); err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", c.url.String(), &buf)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("X-Prometheus-Remote-Read-Version", "0.0.1")

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpResp, err := ctxhttp.Do(ctx, c.client, httpReq)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("server returned HTTP status %s", httpResp.Status)
	}

	if data, err = ioutil.ReadAll(snappy.NewReader(httpResp.Body)); err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var resp ReadResponse
	err = proto.Unmarshal(data, &resp)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response body: %v", err)
	}

	if len(resp.Results) != len(req.Queries) {
		return nil, fmt.Errorf("responses: want %d, got %d", len(req.Queries), len(resp.Results))
	}

	return matrixFromProto(resp.Results[0].Timeseries), nil
}

func labelMatchersToProto(matchers metric.LabelMatchers) []*LabelMatcher {
	pbMatchers := make([]*LabelMatcher, 0, len(matchers))
	for _, m := range matchers {
		var mType MatchType
		switch m.Type {
		case metric.Equal:
			mType = MatchType_EQUAL
		case metric.NotEqual:
			mType = MatchType_NOT_EQUAL
		case metric.RegexMatch:
			mType = MatchType_REGEX_MATCH
		case metric.RegexNoMatch:
			mType = MatchType_REGEX_NO_MATCH
		default:
			panic("invalid matcher type")
		}
		pbMatchers = append(pbMatchers, &LabelMatcher{
			Type:  mType,
			Name:  string(m.Name),
			Value: string(m.Value),
		})
	}
	return pbMatchers
}

func matrixFromProto(seriesSet []*TimeSeries) model.Matrix {
	m := make(model.Matrix, 0, len(seriesSet))
	for _, ts := range seriesSet {
		var ss model.SampleStream
		ss.Metric = labelPairsToMetric(ts.Labels)
		ss.Values = make([]model.SamplePair, 0, len(ts.Samples))
		for _, s := range ts.Samples {
			ss.Values = append(ss.Values, model.SamplePair{
				Value:     model.SampleValue(s.Value),
				Timestamp: model.Time(s.TimestampMs),
			})
		}
		m = append(m, &ss)
	}

	return m
}

func labelPairsToMetric(labelPairs []*LabelPair) model.Metric {
	metric := make(model.Metric, len(labelPairs))
	for _, l := range labelPairs {
		metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	}
	return metric
}
