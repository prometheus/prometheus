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

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/storage/remote/generic"
)

// SampleAppender is the interface to append samples to both, local and remote
// storage. All methods are goroutine-safe.
type SampleAppender interface {
	Append(context.Context, []*model.Sample) error
}

// AppenderHandler returns a http.Handler that accepts protobuf formatted
// metrics and sends them to the supplied appender.
func AppenderHandler(appender SampleAppender) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(context.Background(), UserIDContextKey, r.Header.Get(UserIDContextKey))
		req := &generic.GenericWriteRequest{}
		buf := bytes.Buffer{}
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			log.Errorf(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = proto.Unmarshal(buf.Bytes(), req)
		if err != nil {
			log.Errorf(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var samples []*model.Sample
		for _, ts := range req.Timeseries {
			metric := model.Metric{}
			if ts.Name != nil {
				metric[model.MetricNameLabel] = model.LabelValue(ts.GetName())
			}
			for _, l := range ts.Labels {
				metric[model.LabelName(l.GetName())] = model.LabelValue(l.GetValue())
			}

			for _, s := range ts.Samples {
				samples = append(samples, &model.Sample{
					Metric:    metric,
					Value:     model.SampleValue(s.GetValue()),
					Timestamp: model.Time(s.GetTimestampMs()),
				})
			}
		}

		if err := appender.Append(ctx, samples); err != nil {
			log.Errorf(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// QueryHandler returns a http.Handler that accepts protobuf formatted
// query requests and serves them.
func QueryHandler(querier Querier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(context.Background(), UserIDContextKey, r.Header.Get(UserIDContextKey))
		req := &generic.GenericReadRequest{}
		buf := bytes.Buffer{}
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			log.Errorf(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = proto.Unmarshal(buf.Bytes(), req)
		if err != nil {
			log.Errorf(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		matchers := make(metric.LabelMatchers, 0, len(req.Matchers))
		for _, matcher := range req.Matchers {
			var mtype metric.MatchType
			switch matcher.GetType() {
			case generic.MatchType_EQUAL:
				mtype = metric.Equal
			case generic.MatchType_NOT_EQUAL:
				mtype = metric.NotEqual
			case generic.MatchType_REGEX_MATCH:
				mtype = metric.RegexMatch
			case generic.MatchType_REGEX_NO_MATCH:
				mtype = metric.RegexNoMatch
			default:
				http.Error(w, "invalid matcher type", http.StatusBadRequest)
				return
			}
			matcher, err := metric.NewLabelMatcher(mtype, model.LabelName(matcher.GetName()), model.LabelValue(matcher.GetValue()))
			if err != nil {
				http.Error(w, fmt.Sprintf("error creating matcher: %v", err), http.StatusBadRequest)
				return
			}
			matchers = append(matchers, matcher)
		}

		start := model.Time(req.GetStartTimestampMs())
		end := model.Time(req.GetEndTimestampMs())

		res, err := querier.Query(ctx, start, end, matchers...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := &generic.GenericReadResponse{}
		for _, ss := range res {
			ts := &generic.TimeSeries{
				Name: proto.String(string(ss.Metric[model.MetricNameLabel])),
			}
			for k, v := range ss.Metric {
				if k != model.MetricNameLabel {
					ts.Labels = append(ts.Labels,
						&generic.LabelPair{
							Name:  proto.String(string(k)),
							Value: proto.String(string(v)),
						})
				}
			}
			ts.Samples = make([]*generic.Sample, 0, len(ss.Values))
			for _, s := range ss.Values {
				ts.Samples = append(ts.Samples, &generic.Sample{
					Value:       proto.Float64(float64(s.Value)),
					TimestampMs: proto.Int64(int64(s.Timestamp)),
				})
			}
			resp.Timeseries = append(resp.Timeseries, ts)
		}
		data, err := proto.Marshal(resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err = w.Write(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// TODO: set Content-type.
	})
}
