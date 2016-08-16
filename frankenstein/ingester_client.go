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

package frankenstein

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/storage/remote/generic"
)

type IngesterClient struct {
	hostname string
	client   http.Client
}

// NewIngesterClient makes a new IngesterClient.  This client is careful to
// propagate the user ID from Distributor -> Ingestor.
func NewIngesterClient(hostname string, timeout time.Duration) *IngesterClient {
	client := http.Client{
		Timeout: timeout,
	}
	return &IngesterClient{
		hostname: hostname,
		client:   client,
	}
}

func (c *IngesterClient) Append(ctx context.Context, samples []*model.Sample) error {
	req := &generic.GenericWriteRequest{}
	for _, s := range samples {
		ts := &generic.TimeSeries{
			Name: proto.String(string(s.Metric[model.MetricNameLabel])),
		}
		for k, v := range s.Metric {
			if k != model.MetricNameLabel {
				ts.Labels = append(ts.Labels,
					&generic.LabelPair{
						Name:  proto.String(string(k)),
						Value: proto.String(string(v)),
					})
			}
		}
		ts.Samples = []*generic.Sample{
			&generic.Sample{
				Value:       proto.Float64(float64(s.Value)),
				TimestampMs: proto.Int64(int64(s.Timestamp)),
			},
		}
		req.Timeseries = append(req.Timeseries, ts)
	}
	return c.doRequest(ctx, "/push", req, nil)
}

// Query implements Querier.
func (c *IngesterClient) Query(ctx context.Context, from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
	req := &generic.GenericReadRequest{
		StartTimestampMs: proto.Int64(int64(from)),
		EndTimestampMs:   proto.Int64(int64(to)),
	}
	for _, matcher := range matchers {
		var mType generic.MatchType
		switch matcher.Type {
		case metric.Equal:
			mType = generic.MatchType_EQUAL
		case metric.NotEqual:
			mType = generic.MatchType_NOT_EQUAL
		case metric.RegexMatch:
			mType = generic.MatchType_REGEX_MATCH
		case metric.RegexNoMatch:
			mType = generic.MatchType_REGEX_NO_MATCH
		default:
			panic("invalid matcher type")
		}
		req.Matchers = append(req.Matchers, &generic.LabelMatcher{
			Type:  &mType,
			Name:  proto.String(string(matcher.Name)),
			Value: proto.String(string(matcher.Value)),
		})
	}

	resp := &generic.GenericReadResponse{}
	err := c.doRequest(ctx, "/query", req, resp)
	if err != nil {
		return nil, err
	}

	m := make(model.Matrix, 0, len(resp.Timeseries))
	for _, ts := range resp.Timeseries {
		var ss model.SampleStream
		ss.Metric = model.Metric{}
		if ts.Name != nil {
			ss.Metric[model.MetricNameLabel] = model.LabelValue(ts.GetName())
		}
		for _, l := range ts.Labels {
			ss.Metric[model.LabelName(l.GetName())] = model.LabelValue(l.GetValue())
		}

		ss.Values = make([]model.SamplePair, 0, len(ts.Samples))
		for _, s := range ts.Samples {
			ss.Values = append(ss.Values, model.SamplePair{
				Value:     model.SampleValue(s.GetValue()),
				Timestamp: model.Time(s.GetTimestampMs()),
			})
		}
		m = append(m, &ss)
	}

	return m, nil
}

// LabelValuesForLabelName returns all of the label values that are associated with a given label name.
func (c *IngesterClient) LabelValuesForLabelName(ctx context.Context, ln model.LabelName) (model.LabelValues, error) {
	req := &generic.GenericLabelValuesRequest{
		LabelName: proto.String(string(ln)),
	}
	resp := &generic.GenericLabelValuesResponse{}
	err := c.doRequest(ctx, "/label_values", req, resp)
	if err != nil {
		return nil, err
	}

	values := make(model.LabelValues, 0, len(resp.LabelValues))
	for _, v := range resp.LabelValues {
		values = append(values, model.LabelValue(v))
	}
	return values, nil
}

func (c *IngesterClient) doRequest(ctx context.Context, endpoint string, req proto.Message, resp proto.Message) error {
	userID, err := userID(ctx)
	if err != nil {
		return err
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to marshal request: %v", err)
	}
	buf := bytes.NewBuffer(data)

	httpReq, err := http.NewRequest("POST", fmt.Sprintf("http://%s%s", c.hostname, endpoint), buf)
	if err != nil {
		return fmt.Errorf("unable to create request: %v", err)
	}
	httpReq.Header.Add(userIDHeaderName, userID)
	// TODO: This isn't actually the correct Content-type.
	httpReq.Header.Set("Content-Type", string(expfmt.FmtProtoDelim))
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode/100 != 2 {
		return fmt.Errorf("server returned HTTP status %s", httpResp.Status)
	}

	if resp == nil {
		return nil
	}

	buf.Reset()
	_, err = buf.ReadFrom(httpResp.Body)
	if err != nil {
		return fmt.Errorf("unable to read response body: %v", err)
	}
	err = proto.Unmarshal(buf.Bytes(), resp)
	if err != nil {
		return fmt.Errorf("unable to unmarshal response body: %v", err)
	}
	return nil
}
