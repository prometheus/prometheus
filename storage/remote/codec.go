// Copyright 2017 The Prometheus Authors
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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
)

// decodeReadLimit is the maximum size of a read request body in bytes (32MB).
const decodeReadLimit = 32 * 1024 * 1024

type HTTPError struct {
	msg    string
	status int
}

func (e HTTPError) Error() string {
	return e.msg
}

func (e HTTPError) Status() int {
	return e.status
}

// DecodeReadRequest reads a remote.Request from a http.Request.
func DecodeReadRequest(r *http.Request) (*prompb.ReadRequest, error) {
	compressed, err := ioutil.ReadAll(io.LimitReader(r.Body, decodeReadLimit))
	if err != nil {
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	var req prompb.ReadRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

// ToWriteRequest converts an array of samples into a WriteRequest proto.
func ToWriteRequest(samples []*model.Sample) *prompb.WriteRequest {
	req := &prompb.WriteRequest{
		Timeseries: make([]*prompb.TimeSeries, 0, len(samples)),
	}

	for _, s := range samples {
		ts := prompb.TimeSeries{
			Labels: MetricToLabelProtos(s.Metric),
			Samples: []*prompb.Sample{
				{
					Value:     float64(s.Value),
					Timestamp: int64(s.Timestamp),
				},
			},
		}
		req.Timeseries = append(req.Timeseries, &ts)
	}

	return req
}

// ToQuery builds a Query proto.
func ToQuery(from, to int64, matchers []*labels.Matcher, p *storage.SelectParams) (*prompb.Query, error) {
	ms, err := toLabelMatchers(matchers)
	if err != nil {
		return nil, err
	}

	var rp *prompb.ReadHints
	if p != nil {
		rp = &prompb.ReadHints{
			StepMs:  p.Step,
			Func:    p.Func,
			StartMs: p.Start,
			EndMs:   p.End,
		}
	}

	return &prompb.Query{
		StartTimestampMs: from,
		EndTimestampMs:   to,
		Matchers:         ms,
		Hints:            rp,
	}, nil
}

// FromQuery unpacks a Query proto.
func FromQuery(req *prompb.Query) (int64, int64, []*labels.Matcher, *storage.SelectParams, error) {
	matchers, err := fromLabelMatchers(req.Matchers)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	var selectParams *storage.SelectParams
	if req.Hints != nil {
		selectParams = &storage.SelectParams{
			Start: req.Hints.StartMs,
			End:   req.Hints.EndMs,
			Step:  req.Hints.StepMs,
			Func:  req.Hints.Func,
		}
	}

	return req.StartTimestampMs, req.EndTimestampMs, matchers, selectParams, nil
}

// ToTimeSeries builds a repeated TimeSeries proto.
func ToTimeSeries(ss storage.SeriesSet) ([]*prompb.TimeSeries, error) {
	var resp []*prompb.TimeSeries

	for ss.Next() {
		ts, err := toSingleTimeSeries(ss.At())
		if err != nil {
			return nil, err
		}

		resp = append(resp, ts)
	}
	if err := ss.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

// toSingleTimeSeries builds a TimeSeries proto.
func toSingleTimeSeries(series storage.Series) (*prompb.TimeSeries, error) {
	iter := series.Iterator()
	var samples []*prompb.Sample

	for iter.Next() {
		ts, val := iter.At()
		samples = append(samples, &prompb.Sample{
			Timestamp: ts,
			Value:     val,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	return &prompb.TimeSeries{
		Labels:  labelsToLabelsProto(series.Labels()),
		Samples: samples,
	}, nil
}

// validateLabelsAndMetricName validates the label names/values and metric names returned from remote read.
func validateLabelsAndMetricName(ls labels.Labels) error {
	for _, l := range ls {
		if l.Name == labels.MetricName && !model.IsValidMetricName(model.LabelValue(l.Value)) {
			return fmt.Errorf("Invalid metric name: %v", l.Value)
		}
		if !model.LabelName(l.Name).IsValid() {
			return fmt.Errorf("Invalid label name: %v", l.Name)
		}
		if !model.LabelValue(l.Value).IsValid() {
			return fmt.Errorf("Invalid label value: %v", l.Value)
		}
	}
	return nil
}

func toLabelMatchers(matchers []*labels.Matcher) ([]*prompb.LabelMatcher, error) {
	pbMatchers := make([]*prompb.LabelMatcher, 0, len(matchers))
	for _, m := range matchers {
		var mType prompb.LabelMatcher_Type
		switch m.Type {
		case labels.MatchEqual:
			mType = prompb.LabelMatcher_EQ
		case labels.MatchNotEqual:
			mType = prompb.LabelMatcher_NEQ
		case labels.MatchRegexp:
			mType = prompb.LabelMatcher_RE
		case labels.MatchNotRegexp:
			mType = prompb.LabelMatcher_NRE
		default:
			return nil, fmt.Errorf("invalid matcher type")
		}
		pbMatchers = append(pbMatchers, &prompb.LabelMatcher{
			Type:  mType,
			Name:  m.Name,
			Value: m.Value,
		})
	}
	return pbMatchers, nil
}

func fromLabelMatchers(matchers []*prompb.LabelMatcher) ([]*labels.Matcher, error) {
	result := make([]*labels.Matcher, 0, len(matchers))
	for _, matcher := range matchers {
		var mtype labels.MatchType
		switch matcher.Type {
		case prompb.LabelMatcher_EQ:
			mtype = labels.MatchEqual
		case prompb.LabelMatcher_NEQ:
			mtype = labels.MatchNotEqual
		case prompb.LabelMatcher_RE:
			mtype = labels.MatchRegexp
		case prompb.LabelMatcher_NRE:
			mtype = labels.MatchNotRegexp
		default:
			return nil, fmt.Errorf("invalid matcher type")
		}
		matcher, err := labels.NewMatcher(mtype, matcher.Name, matcher.Value)
		if err != nil {
			return nil, err
		}
		result = append(result, matcher)
	}
	return result, nil
}

// MetricToLabelProtos builds a []*prompb.Label from a model.Metric
func MetricToLabelProtos(metric model.Metric) []*prompb.Label {
	labels := make([]*prompb.Label, 0, len(metric))
	for k, v := range metric {
		labels = append(labels, &prompb.Label{
			Name:  string(k),
			Value: string(v),
		})
	}
	sort.Slice(labels, func(i int, j int) bool {
		return labels[i].Name < labels[j].Name
	})
	return labels
}

// LabelProtosToMetric unpack a []*prompb.Label to a model.Metric
func LabelProtosToMetric(labelPairs []*prompb.Label) model.Metric {
	metric := make(model.Metric, len(labelPairs))
	for _, l := range labelPairs {
		metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	}
	return metric
}

func labelProtosToLabels(labelPairs []*prompb.Label) labels.Labels {
	result := make(labels.Labels, 0, len(labelPairs))
	for _, l := range labelPairs {
		result = append(result, labels.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	sort.Sort(result)
	return result
}

func labelsToLabelsProto(labels labels.Labels) []*prompb.Label {
	result := make([]*prompb.Label, 0, len(labels))
	for _, l := range labels {
		result = append(result, &prompb.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	return result
}

func labelsToMetric(ls labels.Labels) model.Metric {
	metric := make(model.Metric, len(ls))
	for _, l := range ls {
		metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	}
	return metric
}
