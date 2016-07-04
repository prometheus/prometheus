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
	"fmt"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/api/prometheus"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/metric"
)

// A Querier allows querying all samples in a given time range that match a set
// of label matchers.
type Querier interface {
	Query(from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error)
}

// An IngesterQuerier is a Querier that fetches recent samples from a
// Frankenstein Ingester.
type IngesterQuerier struct {
	api prometheus.QueryAPI
}

// NewIngesterQuerier creates a new IngesterQuerier given an ingester URL.
// TODO: Make query timeout configurable.
func NewIngesterQuerier(url string) (*IngesterQuerier, error) {
	client, err := prometheus.New(prometheus.Config{
		Address: url,
	})
	if err != nil {
		return nil, err
	}
	return &IngesterQuerier{
		api: prometheus.NewQueryAPI(client),
	}, nil
}

// Query implements Querier.
func (q *IngesterQuerier) Query(from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
	// Create a PromQL query from the label matchers.
	strs := make([]string, 0, len(matchers))
	for _, m := range matchers {
		strs = append(strs, m.String())
	}
	// TODO: Is LabelMatcher.String() sufficiently escaped?
	expr := fmt.Sprintf("{%s}[%ds]", strings.Join(strs, ","), int64(to.Sub(from).Seconds()))

	// Query the remote ingester.
	res, err := q.api.Query(context.Background(), expr, to.Time())
	if err != nil {
		return nil, err
	}

	// Munge response data into a model.Matrix if necessary.
	switch res.Type() {
	case model.ValMatrix:
		return res.(model.Matrix), nil
	case model.ValVector:
		v := res.(model.Vector)
		m := make(model.Matrix, 0, len(v))
		for _, s := range v {
			m = append(m, &model.SampleStream{
				Metric: s.Metric,
				Values: []model.SamplePair{
					{
						Value:     s.Value,
						Timestamp: s.Timestamp,
					},
				},
			})
		}
		return m, nil
	default:
		panic("unexpected response value type")
	}
}

// A ChunkQuerier is a Querier that fetches samples from a ChunkStore.
type ChunkQuerier struct {
	store ChunkStore
}

// NewChunkQuerier creates a new ChunkQuerier given a ChunkStore.
func NewChunkQuerier(store ChunkStore) *ChunkQuerier {
	return &ChunkQuerier{
		store: store,
	}
}

// Query implements Querier and transforms a list of chunks into sample
// matrices.
func (q *ChunkQuerier) Query(from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
	// Get chunks for all matching series from ChunkStore.
	chunks, err := q.store.Get(from, to, matchers...)
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
		ss.Values = append(ss.Values, decodeChunk(c)...)
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

func decodeChunk(c Chunk) []model.SamplePair {
	// TODO: Implement chunk decoding (this function is just a placeholder, this
	// code will live somewhere else anyways). The chunking format is not defined
	// yet, so it depends on how the write path will encode them.
	return nil
}

// A MergeQuerier is a promql.Querier that merges the results of multiple
// frankenstein.Queriers for the same query.
type MergeQuerier struct {
	queriers []Querier
}

// Query fetches series for a given time range and label matchers from multiple
// promql.Queriers and returns the merged results as a map of series iterators.
func (qm MergeQuerier) Query(from, to model.Time, matchers ...*metric.LabelMatcher) (map[model.Fingerprint]promql.SeriesIterator, error) {
	iterators := map[model.Fingerprint]promql.SeriesIterator{}

	// Fetch samples from all queriers and group them by fingerprint (unsorted
	// and with overlap).
	for _, q := range qm.queriers {
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
