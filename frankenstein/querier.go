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
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/storage/remote/generic"
)

// A Querier allows querying all samples in a given time range that match a set
// of label matchers.
type Querier interface {
	Query(from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error)
}

// An IngesterQuerier is a Querier that fetches recent samples from a
// Frankenstein Ingester.
type IngesterQuerier struct {
	url    string
	client http.Client
}

func NewIngesterQuerier(url string, timeout time.Duration) *IngesterQuerier {
	client := http.Client{
		Timeout: timeout,
	}
	return &IngesterQuerier{
		url:    url,
		client: client,
	}
}

// Query implements Querier.
func (q *IngesterQuerier) Query(from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
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

	data, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(data)

	// TODO: This isn't actually the correct Content-type.
	resp, err := q.client.Post(q.url, string(expfmt.FmtProtoDelim), buf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	r := &generic.GenericReadResponse{}
	buf.Reset()
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %s", err)
	}
	err = proto.Unmarshal(buf.Bytes(), r)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response body: %s", err)
	}

	m := make(model.Matrix, 0, len(r.Timeseries))
	for _, ts := range r.Timeseries {
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

// A ChunkQuerier is a Querier that fetches samples from a ChunkStore.
type ChunkQuerier struct {
	Store ChunkStore
}

// Query implements Querier and transforms a list of chunks into sample
// matrices.
func (q *ChunkQuerier) Query(from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
	// Get chunks for all matching series from ChunkStore.
	chunks, err := q.Store.Get(from, to, matchers...)
	if err != nil {
		return nil, err
	}

	// Group chunks by series, sort and dedupe samples.
	sampleStreams := map[model.Fingerprint]*model.SampleStream{}

	for _, c := range chunks {
		fp := c.Metric.Fingerprint()
		ss, ok := sampleStreams[fp]
		if !ok {
			ss = &model.SampleStream{
				Metric: c.Metric,
			}
			sampleStreams[fp] = ss
		}
		ss.Values = append(ss.Values, local.DecodeDoubleDeltaChunk(c.Data)...)
	}

	for _, ss := range sampleStreams {
		sort.Sort(timeSortableSamplePairs(ss.Values))
		// TODO: should we also dedupe samples here or leave that to the upper layers?
	}

	matrix := make(model.Matrix, 0, len(sampleStreams))
	for _, ss := range sampleStreams {
		matrix = append(matrix, ss)
	}

	return matrix, nil
}

type timeSortableSamplePairs []model.SamplePair

func (ts timeSortableSamplePairs) Len() int {
	return len(ts)
}

func (ts timeSortableSamplePairs) Less(i, j int) bool {
	return ts[i].Timestamp < ts[j].Timestamp
}

func (ts timeSortableSamplePairs) Swap(i, j int) {
	ts[i], ts[j] = ts[j], ts[i]
}

// A MergeQuerier is a promql.Querier that merges the results of multiple
// frankenstein.Queriers for the same query.
type MergeQuerier struct {
	Queriers []Querier
}

// Query fetches series for a given time range and label matchers from multiple
// promql.Queriers and returns the merged results as a map of series iterators.
func (qm MergeQuerier) Query(from, to model.Time, matchers ...*metric.LabelMatcher) (map[model.Fingerprint]local.MetricSeriesIterator, error) {
	iterators := map[model.Fingerprint]local.MetricSeriesIterator{}

	// Fetch samples from all queriers and group them by fingerprint (unsorted
	// and with overlap).
	for _, q := range qm.Queriers {
		matrix, err := q.Query(from, to, matchers...)
		if err != nil {
			return nil, err
		}

		for _, ss := range matrix {
			fp := ss.Metric.Fingerprint()
			if it, ok := iterators[fp]; !ok {
				iterators[fp] = sampleStreamIterator{
					ss: ss,
				}
			} else {
				ssIt := it.(sampleStreamIterator)
				ssIt.ss.Values = append(ssIt.ss.Values, ss.Values...)
			}
		}
	}

	// Sort and dedupe samples.
	for _, it := range iterators {
		sortable := timeSortableSamplePairs(it.(sampleStreamIterator).ss.Values)
		sort.Sort(sortable)
		// TODO: Dedupe samples. Not strictly necessary.
	}

	return iterators, nil
}
