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
	"context"
	"sort"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
)

// Querier returns a new Querier on the storage.
func (r *Storage) Querier(mint, maxt int64) (storage.Querier, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	queriers := make([]storage.Querier, 0, len(r.clients))
	for _, c := range r.clients {
		queriers = append(queriers, &querier{
			mint:           mint,
			maxt:           maxt,
			client:         c,
			externalLabels: r.externalLabels,
		})
	}
	return storage.NewMergeQuerier(queriers), nil
}

// Querier is an adapter to make a Client usable as a storage.Querier.
type querier struct {
	mint, maxt     int64
	client         *Client
	externalLabels model.LabelSet
}

// Select returns a set of series that matches the given label matchers.
func (q *querier) Select(matchers ...*labels.Matcher) storage.SeriesSet {
	m, added := q.addExternalLabels(matchers)

	res, err := q.client.Read(context.TODO(), q.mint, q.maxt, labelMatchersToProto(m))
	if err != nil {
		return errSeriesSet{err: err}
	}

	series := make([]storage.Series, 0, len(res))
	for _, ts := range res {
		labels := labelPairsToLabels(ts.Labels)
		removeLabels(labels, added)
		series = append(series, &concreteSeries{
			labels:  labels,
			samples: ts.Samples,
		})
	}
	sort.Sort(byLabel(series))
	return &concreteSeriesSet{
		series: series,
	}
}

func labelMatchersToProto(matchers []*labels.Matcher) []*prompb.LabelMatcher {
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
			panic("invalid matcher type")
		}
		pbMatchers = append(pbMatchers, &prompb.LabelMatcher{
			Type:  mType,
			Name:  string(m.Name),
			Value: string(m.Value),
		})
	}
	return pbMatchers
}

func labelPairsToLabels(labelPairs []*prompb.Label) labels.Labels {
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

type byLabel []storage.Series

func (a byLabel) Len() int           { return len(a) }
func (a byLabel) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byLabel) Less(i, j int) bool { return labels.Compare(a[i].Labels(), a[j].Labels()) < 0 }

// LabelValues returns all potential values for a label name.
func (q *querier) LabelValues(name string) ([]string, error) {
	// TODO implement?
	return nil, nil
}

// Close releases the resources of the Querier.
func (q *querier) Close() error {
	return nil
}

// errSeriesSet implements storage.SeriesSet, just returning an error.
type errSeriesSet struct {
	err error
}

func (errSeriesSet) Next() bool {
	return false
}

func (errSeriesSet) At() storage.Series {
	return nil
}

func (e errSeriesSet) Err() error {
	return e.err
}

// concreteSeriesSet implements storage.SeriesSet.
type concreteSeriesSet struct {
	cur    int
	series []storage.Series
}

func (c *concreteSeriesSet) Next() bool {
	c.cur++
	return c.cur < len(c.series)
}

func (c *concreteSeriesSet) At() storage.Series {
	return c.series[c.cur]
}

func (c *concreteSeriesSet) Err() error {
	return nil
}

// concreteSeries implementes storage.Series.
type concreteSeries struct {
	labels  labels.Labels
	samples []*prompb.Sample
}

func (c *concreteSeries) Labels() labels.Labels {
	return c.labels
}

func (c *concreteSeries) Iterator() storage.SeriesIterator {
	return newConcreteSeriersIterator(c)
}

// concreteSeriesIterator implements storage.SeriesIterator.
type concreteSeriesIterator struct {
	cur    int
	series *concreteSeries
}

func newConcreteSeriersIterator(series *concreteSeries) storage.SeriesIterator {
	return &concreteSeriesIterator{
		cur:    -1,
		series: series,
	}
}

func (c *concreteSeriesIterator) Seek(t int64) bool {
	c.cur = sort.Search(len(c.series.samples), func(n int) bool {
		return c.series.samples[n].Timestamp >= t
	})
	return c.cur < len(c.series.samples)
}

func (c *concreteSeriesIterator) At() (t int64, v float64) {
	s := c.series.samples[c.cur]
	return s.Timestamp, s.Value
}

func (c *concreteSeriesIterator) Next() bool {
	c.cur++
	return c.cur < len(c.series.samples)
}

func (c *concreteSeriesIterator) Err() error {
	return nil
}

// addExternalLabels adds matchers for each external label. External labels
// that already have a corresponding user-supplied matcher are skipped, as we
// assume that the user explicitly wants to select a different value for them.
// We return the new set of matchers, along with a map of labels for which
// matchers were added, so that these can later be removed from the result
// time series again.
func (q *querier) addExternalLabels(matchers []*labels.Matcher) ([]*labels.Matcher, model.LabelSet) {
	el := make(model.LabelSet, len(q.externalLabels))
	for k, v := range q.externalLabels {
		el[k] = v
	}
	for _, m := range matchers {
		if _, ok := el[model.LabelName(m.Name)]; ok {
			delete(el, model.LabelName(m.Name))
		}
	}
	for k, v := range el {
		m, err := labels.NewMatcher(labels.MatchEqual, string(k), string(v))
		if err != nil {
			panic(err)
		}
		matchers = append(matchers, m)
	}
	return matchers, el
}

func removeLabels(l labels.Labels, toDelete model.LabelSet) {
	for i := 0; i < len(l); {
		if _, ok := toDelete[model.LabelName(l[i].Name)]; ok {
			l = l[:i+copy(l[i:], l[i+1:])]
		} else {
			i++
		}
	}
}
