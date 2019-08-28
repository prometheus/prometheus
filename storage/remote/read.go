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
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

var remoteReadQueries = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "remote_read_queries",
		Help:      "The number of in-flight remote read queries.",
	},
	[]string{"client"},
)

func init() {
	prometheus.MustRegister(remoteReadQueries)
}

// QueryableClient returns a storage.Queryable which queries the given
// Client to select series sets.
func QueryableClient(c *Client) storage.Queryable {
	remoteReadQueries.WithLabelValues(c.Name())
	return storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		return &querier{
			ctx:    ctx,
			mint:   mint,
			maxt:   maxt,
			client: c,
		}, nil
	})
}

// querier is an adapter to make a Client usable as a storage.Querier.
type querier struct {
	ctx        context.Context
	mint, maxt int64
	client     *Client
}

// Select implements storage.Querier and uses the given matchers to read series
// sets from the Client.
func (q *querier) Select(p *storage.SelectParams, matchers ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	query, err := ToQuery(q.mint, q.maxt, matchers, p)
	if err != nil {
		return nil, nil, err
	}

	remoteReadGauge := remoteReadQueries.WithLabelValues(q.client.Name())
	remoteReadGauge.Inc()
	defer remoteReadGauge.Dec()

	res, err := q.client.Read(q.ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("remote_read: %v", err)
	}

	return FromQueryResult(res), nil, nil
}

// LabelValues implements storage.Querier and is a noop.
func (q *querier) LabelValues(name string) ([]string, storage.Warnings, error) {
	// TODO implement?
	return nil, nil, nil
}

// LabelNames implements storage.Querier and is a noop.
func (q *querier) LabelNames() ([]string, storage.Warnings, error) {
	// TODO implement?
	return nil, nil, nil
}

// Close implements storage.Querier and is a noop.
func (q *querier) Close() error {
	return nil
}

// ExternalLabelsHandler returns a storage.Queryable which creates a
// externalLabelsQuerier.
func ExternalLabelsHandler(next storage.Queryable, externalLabels labels.Labels) storage.Queryable {
	return storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		q, err := next.Querier(ctx, mint, maxt)
		if err != nil {
			return nil, err
		}
		return &externalLabelsQuerier{Querier: q, externalLabels: externalLabels}, nil
	})
}

// externalLabelsQuerier is a querier which ensures that Select() results match
// the configured external labels.
type externalLabelsQuerier struct {
	storage.Querier

	externalLabels labels.Labels
}

// Select adds equality matchers for all external labels to the list of matchers
// before calling the wrapped storage.Queryable. The added external labels are
// removed from the returned series sets.
func (q externalLabelsQuerier) Select(p *storage.SelectParams, matchers ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	m, added := q.addExternalLabels(matchers)
	s, warnings, err := q.Querier.Select(p, m...)
	if err != nil {
		return nil, warnings, err
	}
	return newSeriesSetFilter(s, added), warnings, nil
}

// PreferLocalStorageFilter returns a QueryableFunc which creates a NoopQuerier
// if requested timeframe can be answered completely by the local TSDB, and
// reduces maxt if the timeframe can be partially answered by TSDB.
func PreferLocalStorageFilter(next storage.Queryable, cb startTimeCallback) storage.Queryable {
	return storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		localStartTime, err := cb()
		if err != nil {
			return nil, err
		}
		cmaxt := maxt
		// Avoid queries whose timerange is later than the first timestamp in local DB.
		if mint > localStartTime {
			return storage.NoopQuerier(), nil
		}
		// Query only samples older than the first timestamp in local DB.
		if maxt > localStartTime {
			cmaxt = localStartTime
		}
		return next.Querier(ctx, mint, cmaxt)
	})
}

// RequiredMatchersFilter returns a storage.Queryable which creates a
// requiredMatchersQuerier.
func RequiredMatchersFilter(next storage.Queryable, required []*labels.Matcher) storage.Queryable {
	return storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		q, err := next.Querier(ctx, mint, maxt)
		if err != nil {
			return nil, err
		}
		return &requiredMatchersQuerier{Querier: q, requiredMatchers: required}, nil
	})
}

// requiredMatchersQuerier wraps a storage.Querier and requires Select() calls
// to match the given labelSet.
type requiredMatchersQuerier struct {
	storage.Querier

	requiredMatchers []*labels.Matcher
}

// Select returns a NoopSeriesSet if the given matchers don't match the label
// set of the requiredMatchersQuerier. Otherwise it'll call the wrapped querier.
func (q requiredMatchersQuerier) Select(p *storage.SelectParams, matchers ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	ms := q.requiredMatchers
	for _, m := range matchers {
		for i, r := range ms {
			if m.Type == labels.MatchEqual && m.Name == r.Name && m.Value == r.Value {
				ms = append(ms[:i], ms[i+1:]...)
				break
			}
		}
		if len(ms) == 0 {
			break
		}
	}
	if len(ms) > 0 {
		return storage.NoopSeriesSet(), nil, nil
	}
	return q.Querier.Select(p, matchers...)
}

// addExternalLabels adds matchers for each external label. External labels
// that already have a corresponding user-supplied matcher are skipped, as we
// assume that the user explicitly wants to select a different value for them.
// We return the new set of matchers, along with a map of labels for which
// matchers were added, so that these can later be removed from the result
// time series again.
func (q externalLabelsQuerier) addExternalLabels(ms []*labels.Matcher) ([]*labels.Matcher, labels.Labels) {
	el := make(labels.Labels, len(q.externalLabels))
	copy(el, q.externalLabels)

	// ms won't be sorted, so have to O(n^2) the search.
	for _, m := range ms {
		for i := 0; i < len(el); {
			if el[i].Name == m.Name {
				el = el[:i+copy(el[i:], el[i+1:])]
				continue
			}
			i++
		}
	}

	for _, l := range el {
		m, err := labels.NewMatcher(labels.MatchEqual, l.Name, l.Value)
		if err != nil {
			panic(err)
		}
		ms = append(ms, m)
	}
	return ms, el
}

func newSeriesSetFilter(ss storage.SeriesSet, toFilter labels.Labels) storage.SeriesSet {
	return &seriesSetFilter{
		SeriesSet: ss,
		toFilter:  toFilter,
	}
}

type seriesSetFilter struct {
	storage.SeriesSet
	toFilter labels.Labels
	querier  storage.Querier
}

func (ssf *seriesSetFilter) GetQuerier() storage.Querier {
	return ssf.querier
}

func (ssf *seriesSetFilter) SetQuerier(querier storage.Querier) {
	ssf.querier = querier
}

func (ssf seriesSetFilter) At() storage.Series {
	return seriesFilter{
		Series:   ssf.SeriesSet.At(),
		toFilter: ssf.toFilter,
	}
}

type seriesFilter struct {
	storage.Series
	toFilter labels.Labels
}

func (sf seriesFilter) Labels() labels.Labels {
	labels := sf.Series.Labels()
	for i, j := 0, 0; i < len(labels) && j < len(sf.toFilter); {
		if labels[i].Name < sf.toFilter[j].Name {
			i++
		} else if labels[i].Name > sf.toFilter[j].Name {
			j++
		} else {
			labels = labels[:i+copy(labels[i:], labels[i+1:])]
			j++
		}
	}
	return labels
}
