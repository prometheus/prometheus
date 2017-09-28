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
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// DecodeReadRequest reads a remote.Request from a http.Request.
func DecodeReadRequest(r *http.Request) (*ReadRequest, error) {
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	var req ReadRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

// EncodeReadResponse writes a remote.Response to a http.ResponseWriter.
func EncodeReadResponse(resp *ReadResponse, w http.ResponseWriter) error {
	data, err := proto.Marshal(resp)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Content-Encoding", "snappy")

	compressed := snappy.Encode(nil, data)
	_, err = w.Write(compressed)
	return err
}

// ToWriteRequest converts an array of samples into a WriteRequest proto.
func ToWriteRequest(samples []*model.Sample) *WriteRequest {
	req := &WriteRequest{
		Timeseries: make([]*TimeSeries, 0, len(samples)),
	}

	for _, s := range samples {
		ts := TimeSeries{
			Labels: ToLabelPairs(s.Metric),
			Samples: []*Sample{
				{
					Value:       float64(s.Value),
					TimestampMs: int64(s.Timestamp),
				},
			},
		}
		req.Timeseries = append(req.Timeseries, &ts)
	}

	return req
}

// ToQuery builds a Query proto.
func ToQuery(from, to model.Time, matchers []*metric.LabelMatcher) (*Query, error) {
	ms, err := toLabelMatchers(matchers)
	if err != nil {
		return nil, err
	}

	return &Query{
		StartTimestampMs: int64(from),
		EndTimestampMs:   int64(to),
		Matchers:         ms,
	}, nil
}

// FromQuery unpacks a Query proto.
func FromQuery(req *Query) (model.Time, model.Time, []*metric.LabelMatcher, error) {
	matchers, err := fromLabelMatchers(req.Matchers)
	if err != nil {
		return 0, 0, nil, err
	}
	from := model.Time(req.StartTimestampMs)
	to := model.Time(req.EndTimestampMs)
	return from, to, matchers, nil
}

// ToQueryResult builds a QueryResult proto.
func ToQueryResult(matrix model.Matrix) *QueryResult {
	resp := &QueryResult{}
	for _, ss := range matrix {
		ts := TimeSeries{
			Labels:  ToLabelPairs(ss.Metric),
			Samples: make([]*Sample, 0, len(ss.Values)),
		}
		for _, s := range ss.Values {
			ts.Samples = append(ts.Samples, &Sample{
				Value:       float64(s.Value),
				TimestampMs: int64(s.Timestamp),
			})
		}
		resp.Timeseries = append(resp.Timeseries, &ts)
	}
	return resp
}

// FromQueryResult unpacks a QueryResult proto.
func FromQueryResult(resp *QueryResult) model.Matrix {
	m := make(model.Matrix, 0, len(resp.Timeseries))
	for _, ts := range resp.Timeseries {
		var ss model.SampleStream
		ss.Metric = FromLabelPairs(ts.Labels)
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

func toLabelMatchers(matchers []*metric.LabelMatcher) ([]*LabelMatcher, error) {
	result := make([]*LabelMatcher, 0, len(matchers))
	for _, matcher := range matchers {
		var mType MatchType
		switch matcher.Type {
		case metric.Equal:
			mType = MatchType_EQUAL
		case metric.NotEqual:
			mType = MatchType_NOT_EQUAL
		case metric.RegexMatch:
			mType = MatchType_REGEX_MATCH
		case metric.RegexNoMatch:
			mType = MatchType_REGEX_NO_MATCH
		default:
			return nil, fmt.Errorf("invalid matcher type")
		}
		result = append(result, &LabelMatcher{
			Type:  mType,
			Name:  string(matcher.Name),
			Value: string(matcher.Value),
		})
	}
	return result, nil
}

func fromLabelMatchers(matchers []*LabelMatcher) ([]*metric.LabelMatcher, error) {
	result := make(metric.LabelMatchers, 0, len(matchers))
	for _, matcher := range matchers {
		var mtype metric.MatchType
		switch matcher.Type {
		case MatchType_EQUAL:
			mtype = metric.Equal
		case MatchType_NOT_EQUAL:
			mtype = metric.NotEqual
		case MatchType_REGEX_MATCH:
			mtype = metric.RegexMatch
		case MatchType_REGEX_NO_MATCH:
			mtype = metric.RegexNoMatch
		default:
			return nil, fmt.Errorf("invalid matcher type")
		}
		matcher, err := metric.NewLabelMatcher(mtype, model.LabelName(matcher.Name), model.LabelValue(matcher.Value))
		if err != nil {
			return nil, err
		}
		result = append(result, matcher)
	}
	return result, nil
}

// ToLabelPairs builds a []LabelPair from a model.Metric
func ToLabelPairs(metric model.Metric) []*LabelPair {
	labelPairs := make([]*LabelPair, 0, len(metric))
	for k, v := range metric {
		labelPairs = append(labelPairs, &LabelPair{
			Name:  string(k),
			Value: string(v),
		})
	}
	return labelPairs
}

// FromLabelPairs unpack a []LabelPair to a model.Metric
func FromLabelPairs(labelPairs []*LabelPair) model.Metric {
	metric := make(model.Metric, len(labelPairs))
	for _, l := range labelPairs {
		metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	}
	return metric
}
