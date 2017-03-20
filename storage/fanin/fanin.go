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

package fanin

import (
	"sort"
	"time"

	"golang.org/x/net/context"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/storage/remote"
)

type contextKey string

const ctxLocalOnly contextKey = "local-only"

// WithLocalOnly decorates a context to indicate that a query should
// only be executed against local data.
func WithLocalOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxLocalOnly, struct{}{})
}

func localOnly(ctx context.Context) bool {
	return ctx.Value(ctxLocalOnly) == struct{}{}
}

// Queryable is a local.Queryable that reads from local and remote storage.
type Queryable struct {
	Local  promql.Queryable
	Remote *remote.Reader
}

// Querier implements local.Queryable.
func (q Queryable) Querier() (local.Querier, error) {
	localQuerier, err := q.Local.Querier()
	if err != nil {
		return nil, err
	}

	fq := querier{
		local:   localQuerier,
		remotes: q.Remote.Queriers(),
	}
	return fq, nil
}

type querier struct {
	local   local.Querier
	remotes []local.Querier
}

func (q querier) QueryRange(ctx context.Context, from, through model.Time, matchers ...*metric.LabelMatcher) ([]local.SeriesIterator, error) {
	return q.query(ctx, func(q local.Querier) ([]local.SeriesIterator, error) {
		return q.QueryRange(ctx, from, through, matchers...)
	})
}

func (q querier) QueryInstant(ctx context.Context, ts model.Time, stalenessDelta time.Duration, matchers ...*metric.LabelMatcher) ([]local.SeriesIterator, error) {
	return q.query(ctx, func(q local.Querier) ([]local.SeriesIterator, error) {
		return q.QueryInstant(ctx, ts, stalenessDelta, matchers...)
	})
}

func (q querier) query(ctx context.Context, qFn func(q local.Querier) ([]local.SeriesIterator, error)) ([]local.SeriesIterator, error) {
	localIts, err := qFn(q.local)
	if err != nil {
		return nil, err
	}

	if len(q.remotes) == 0 || localOnly(ctx) {
		return localIts, nil
	}

	fpToIt := map[model.Fingerprint]*mergeIterator{}

	for _, it := range localIts {
		fp := it.Metric().Metric.Fingerprint()
		fpToIt[fp] = &mergeIterator{local: it}
	}

	for _, q := range q.remotes {
		its, err := qFn(q)
		if err != nil {
			return nil, err
		}
		mergeIterators(fpToIt, its)
	}

	its := make([]local.SeriesIterator, 0, len(fpToIt))
	for _, it := range fpToIt {
		its = append(its, it)
	}
	return its, nil
}

func (q querier) MetricsForLabelMatchers(ctx context.Context, from, through model.Time, matcherSets ...metric.LabelMatchers) ([]metric.Metric, error) {
	return q.local.MetricsForLabelMatchers(ctx, from, through, matcherSets...)
}

func (q querier) LastSampleForLabelMatchers(ctx context.Context, cutoff model.Time, matcherSets ...metric.LabelMatchers) (model.Vector, error) {
	return q.local.LastSampleForLabelMatchers(ctx, cutoff, matcherSets...)
}

func (q querier) LabelValuesForLabelName(ctx context.Context, ln model.LabelName) (model.LabelValues, error) {
	return q.local.LabelValuesForLabelName(ctx, ln)
}

func (q querier) Close() error {
	if q.local != nil {
		if err := q.local.Close(); err != nil {
			return err
		}
	}

	for _, q := range q.remotes {
		if err := q.Close(); err != nil {
			return err
		}
	}
	return nil
}

// mergeIterator is a SeriesIterator which merges query results for local and remote
// SeriesIterators. If a series has samples in a local iterator, remote samples are
// only considered before the first local sample of a series. This is to avoid things
// like downsampling on the side of the remote storage to interfere with rate(),
// irate(), etc.
type mergeIterator struct {
	local  local.SeriesIterator
	remote []local.SeriesIterator
}

func (mit mergeIterator) ValueAtOrBeforeTime(t model.Time) model.SamplePair {
	latest := model.ZeroSamplePair
	if mit.local != nil {
		latest = mit.local.ValueAtOrBeforeTime(t)
	}

	// We only need to look for a remote last sample if we don't have a local one
	// at all. If we have a local one, by definition we would not care about earlier
	// "last" samples, and we would not consider later ones as well, because we
	// generally only consider remote samples that are older than the oldest
	// local sample.
	if latest == model.ZeroSamplePair {
		for _, it := range mit.remote {
			v := it.ValueAtOrBeforeTime(t)
			if v.Timestamp.After(latest.Timestamp) {
				latest = v
			}
		}
	}

	return latest
}

func (mit mergeIterator) RangeValues(interval metric.Interval) []model.SamplePair {
	remoteCutoff := model.Latest
	var values []model.SamplePair
	if mit.local != nil {
		values = mit.local.RangeValues(interval)
		if len(values) > 0 {
			remoteCutoff = values[0].Timestamp
		}
	}

	for _, it := range mit.remote {
		vs := it.RangeValues(interval)
		n := sort.Search(len(vs), func(i int) bool {
			return !vs[i].Timestamp.Before(remoteCutoff)
		})
		values = mergeSamples(values, vs[:n])
	}
	return values
}

func (mit mergeIterator) Metric() metric.Metric {
	if mit.local != nil {
		return mit.local.Metric()
	}

	// If there is no local iterator, there has to be at least one remote one in
	// order for this iterator to have been created.
	return mit.remote[0].Metric()
}

func (mit mergeIterator) Close() {
	if mit.local != nil {
		mit.local.Close()
	}
	for _, it := range mit.remote {
		it.Close()
	}
}

func mergeIterators(fpToIt map[model.Fingerprint]*mergeIterator, its []local.SeriesIterator) {
	for _, it := range its {
		fp := it.Metric().Metric.Fingerprint()
		if fpIts, ok := fpToIt[fp]; !ok {
			fpToIt[fp] = &mergeIterator{remote: []local.SeriesIterator{it}}
		} else {
			fpToIt[fp].remote = append(fpIts.remote, it)
		}
	}
}

// mergeSamples merges two lists of sample pairs and removes duplicate
// timestamps. It assumes that both lists are sorted by timestamp.
func mergeSamples(a, b []model.SamplePair) []model.SamplePair {
	result := make([]model.SamplePair, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].Timestamp < b[j].Timestamp {
			result = append(result, a[i])
			i++
		} else if a[i].Timestamp > b[j].Timestamp {
			result = append(result, b[j])
			j++
		} else {
			result = append(result, a[i])
			i++
			j++
		}
	}
	result = append(result, a[i:]...)
	result = append(result, b[j:]...)
	return result
}
