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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
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
		return nil, nil, err
	}

	return FromQueryResult(res), nil, nil
}

// LabelValues implements storage.Querier. Read the series sets from the Client
// and return all the values of the given label name.
func (q *querier) LabelValues(name string) ([]string, storage.Warnings, error) {
	// By matcher __name__ != "" gets all the series.
	mat, _ := labels.NewMatcher(labels.MatchNotEqual, labels.MetricName, "")
	ss, warnings, err := q.Select(nil, mat)
	if err != nil {
		return nil, nil, err
	}

	var values []string
	if values, err = getLabelValues(name, ss); err != nil {
		return nil, warnings, err
	}

	return values, warnings, nil
}

// LabelNames implements storage.Querier. Read the series sets from the Client
// and return label names.
func (q *querier) LabelNames() ([]string, storage.Warnings, error) {
	// By matcher __name__ != "" gets all the series.
	mat, _ := labels.NewMatcher(labels.MatchNotEqual, labels.MetricName, "")
	ss, warnings, err := q.Select(nil, mat)
	if err != nil {
		return nil, nil, err
	}

	var names []string
	if names, err = getLabelNames(ss); err != nil {
		return nil, warnings, err
	}
	return names, warnings, nil
}

// Close implements storage.Querier and is a noop.
func (q *querier) Close() error {
	return nil
}

// ExternalLabelsHandler returns a storage.Queryable which creates a
// externalLabelsQuerier.
func ExternalLabelsHandler(next storage.Queryable, externalLabels model.LabelSet) storage.Queryable {
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

	externalLabels model.LabelSet
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

// LabelValues implements storage.Querier. Read the series sets from the Client
// and return all the values of the given label name.
func (q *requiredMatchersQuerier) LabelValues(name string) ([]string, storage.Warnings, error) {
	ss, warnings, err := q.Querier.LabelValues(name)
	if err != nil {
		return nil, nil, err
	}

	return ss, warnings, nil
}

// LabelNames implements storage.Querier.Read the series sets from the Client
// and return label names.
func (q *requiredMatchersQuerier) LabelNames() ([]string, storage.Warnings, error) {
	ss, warnings, err := q.Querier.LabelNames()
	if err != nil {
		return nil, nil, err
	}

	return ss, warnings, nil
}

// This querier return nil for LabelValues() and LabelNames().
type disabledQuerier struct {
	storage.Querier
}

// DisabledFilter returns a storage.Queryable which creates a
// disabledQuerier. disabledQuerier can select but return nil for labelValues and labelNames
func DisabledFilter(next storage.Queryable) storage.Queryable {
	return storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		q, err := next.Querier(ctx, mint, maxt)
		if err != nil {
			return nil, err
		}
		return &disabledQuerier{q}, nil
	})
}

// LabelValues implements storage.Querier. disabledQuerier disables this functionality so just return nil.
func (q disabledQuerier) LabelValues(name string) ([]string, storage.Warnings, error) {
	return []string{}, nil, nil
}

// LabelNames implements storage.Querier. disabledQuerier disables this functionality so just return nil.
func (q disabledQuerier) LabelNames() ([]string, storage.Warnings, error) {
	return []string{}, nil, nil
}

// addExternalLabels adds matchers for each external label. External labels
// that already have a corresponding user-supplied matcher are skipped, as we
// assume that the user explicitly wants to select a different value for them.
// We return the new set of matchers, along with a map of labels for which
// matchers were added, so that these can later be removed from the result
// time series again.
func (q externalLabelsQuerier) addExternalLabels(ms []*labels.Matcher) ([]*labels.Matcher, model.LabelSet) {
	el := make(model.LabelSet, len(q.externalLabels))
	for k, v := range q.externalLabels {
		el[k] = v
	}
	for _, m := range ms {
		if _, ok := el[model.LabelName(m.Name)]; ok {
			delete(el, model.LabelName(m.Name))
		}
	}
	for k, v := range el {
		m, err := labels.NewMatcher(labels.MatchEqual, string(k), string(v))
		if err != nil {
			panic(err)
		}
		ms = append(ms, m)
	}
	return ms, el
}

func newSeriesSetFilter(ss storage.SeriesSet, toFilter model.LabelSet) storage.SeriesSet {
	return &seriesSetFilter{
		SeriesSet: ss,
		toFilter:  toFilter,
	}
}

type seriesSetFilter struct {
	storage.SeriesSet
	toFilter model.LabelSet
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
	toFilter model.LabelSet
}

func (sf seriesFilter) Labels() labels.Labels {
	lbs := sf.Series.Labels()
	for i := 0; i < len(lbs); {
		if _, ok := sf.toFilter[model.LabelName(lbs[i].Name)]; ok {
			lbs = lbs[:i+copy(lbs[i:], lbs[i+1:])]
			continue
		}
		i++
	}
	return lbs
}

//Iterate the series sets and get the values of the given label name.
func getLabelValues(name string, ss storage.SeriesSet) ([]string, error) {
	var labelVal string
	labelValuesMap := make(map[string]struct{})
	for ss.Next() {
		labelVal = ss.At().Labels().Get(name)
		labelValuesMap[labelVal] = struct{}{}
	}
	if err := ss.Err(); err != nil {
		return nil, err
	}
	if _, ok := labelValuesMap[""]; ok {
		delete(labelValuesMap, "")
	}

	if len(labelValuesMap) == 0 {
		return []string{}, nil
	}
	labelValues := make([]string, 0, len(labelValuesMap))
	for name := range labelValuesMap {
		labelValues = append(labelValues, name)
	}
	sort.Strings(labelValues)

	return labelValues, nil
}

//Iterate the series sets and get all the label names.
func getLabelNames(ss storage.SeriesSet) ([]string, error) {
	labelNamesMap := make(map[string]struct{})
	for ss.Next() {
		for _, label := range ss.At().Labels() {
			labelNamesMap[label.Name] = struct{}{}
		}
	}
	if err := ss.Err(); err != nil {
		return nil, err
	}
	if len(labelNamesMap) == 0 {
		return []string{}, nil
	}
	labelNames := make([]string, 0, len(labelNamesMap))
	for name := range labelNamesMap {
		labelNames = append(labelNames, name)
	}
	sort.Strings(labelNames)

	return labelNames, nil
}
