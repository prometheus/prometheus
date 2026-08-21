// Copyright The Prometheus Authors
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

package rules

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// MaxBacktestSteps caps the number of simulated evaluations of a single backtest.
// It matches the range query point limit of the HTTP API.
const MaxBacktestSteps = 11000

// BacktestOptions configures a backtest run.
type BacktestOptions struct {
	// Start and End bound the simulated evaluations, both inclusive.
	Start, End time.Time
	// Interval is the simulated group evaluation interval. It determines the
	// resolution at which the alert expression is sampled, so a coarse interval
	// makes the "for" duration coarse too.
	Interval time.Duration
	// Limit is the per-evaluation alert limit, as configured on the rule group.
	Limit int
	// ExternalURL is used to expand the $externalURL template variable.
	ExternalURL *url.URL
}

func (o BacktestOptions) validate() error {
	switch {
	case o.Interval <= 0:
		return errors.New("interval must be positive")
	case o.End.Before(o.Start):
		return errors.New("end timestamp must not be before start timestamp")
	case o.steps() > MaxBacktestSteps:
		return fmt.Errorf("exceeded maximum of %d evaluations, try a larger interval or a shorter range", MaxBacktestSteps)
	}
	return nil
}

func (o BacktestOptions) steps() int {
	return int(o.End.Sub(o.Start)/o.Interval) + 1
}

// Backtest replays rule over the range described by opts using data already in
// storage, and returns the ALERTS and ALERTS_FOR_STATE series the rule would
// have produced. The rule expression is evaluated as a single range query, which
// is equivalent to, but much cheaper than, one instant query per step.
//
// rule is left untouched; the replay runs against a copy of it.
func Backtest(ctx context.Context, rule *AlertingRule, engine promql.QueryEngine, queryable storage.Queryable, opts BacktestOptions) (promql.Matrix, annotations.Annotations, error) {
	if err := opts.validate(); err != nil {
		return nil, nil, err
	}

	qry, err := engine.NewRangeQuery(ctx, queryable, nil, rule.vector.String(), opts.Start, opts.End, opts.Interval)
	if err != nil {
		return nil, nil, err
	}
	defer qry.Close()

	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, res.Warnings, res.Err
	}
	m, err := res.Matrix()
	if err != nil {
		return nil, res.Warnings, err
	}

	out, err := BacktestMatrix(ctx, rule, m, EngineQueryFunc(engine, queryable), opts)
	return out, res.Warnings, err
}

// BacktestMatrix replays rule over m, which must be the result of range querying
// the rule expression over the same range and at the same interval as opts
// describes. query is only used to expand templates and may be nil if the rule
// has no templated labels or annotations.
//
// It is the transport-independent half of Backtest, for callers that obtain the
// matrix from somewhere other than a local query engine.
func BacktestMatrix(ctx context.Context, rule *AlertingRule, m promql.Matrix, query QueryFunc, opts BacktestOptions) (promql.Matrix, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	if query == nil {
		query = noopQueryFunc
	}

	// A copy keeps the live state of rule intact, and restored=true makes eval
	// emit the alert series we are after.
	replay := rule.clone(true)
	out := seriesBuilder{}
	it := newMatrixIterator(m)

	for i := range opts.steps() {
		ts := opts.Start.Add(time.Duration(i) * opts.Interval)
		vec, err := replay.eval(ctx, 0, ts, it.at(ts), query, opts.ExternalURL, opts.Limit)
		if err != nil {
			return nil, fmt.Errorf("evaluating at %s: %w", ts, err)
		}
		out.add(vec)
	}
	return out.matrix(), nil
}

func noopQueryFunc(context.Context, string, time.Time) (promql.Vector, error) {
	return nil, nil
}

// seriesBuilder accumulates the vectors of successive evaluations into a matrix.
type seriesBuilder map[uint64]*promql.Series

func (b seriesBuilder) add(vec promql.Vector) {
	for _, s := range vec {
		h := s.Metric.Hash()
		if _, ok := b[h]; !ok {
			b[h] = &promql.Series{Metric: s.Metric}
		}
		b[h].Floats = append(b[h].Floats, promql.FPoint{T: s.T, F: s.F})
	}
}

func (b seriesBuilder) matrix() promql.Matrix {
	m := make(promql.Matrix, 0, len(b))
	for _, s := range b {
		m = append(m, *s)
	}
	sort.Sort(m)
	return m
}

// clone returns a copy of r with no active alerts, so that it can be evaluated
// independently of the original.
func (r *AlertingRule) clone(restored bool) *AlertingRule {
	return NewAlertingRule(
		r.name, r.vector, r.holdDuration, r.keepFiringFor,
		r.labels, r.annotations, labels.FromMap(r.externalLabels), r.externalURL,
		restored, r.logger,
	)
}

// matrixIterator walks a matrix timestamp by timestamp, turning it back into the
// vectors an instant query would have returned at each step.
type matrixIterator struct {
	m    promql.Matrix
	fPos []int
	hPos []int
	vec  promql.Vector
}

func newMatrixIterator(m promql.Matrix) *matrixIterator {
	return &matrixIterator{
		m:    m,
		fPos: make([]int, len(m)),
		hPos: make([]int, len(m)),
		vec:  make(promql.Vector, 0, len(m)),
	}
}

// at returns the samples at ts. The returned vector is only valid until the next
// call, and callers must not retain it.
func (it *matrixIterator) at(ts time.Time) promql.Vector {
	t := timestamp.FromTime(ts)
	it.vec = it.vec[:0]

	for i, s := range it.m {
		for it.fPos[i] < len(s.Floats) && s.Floats[it.fPos[i]].T < t {
			it.fPos[i]++
		}
		if it.fPos[i] < len(s.Floats) && s.Floats[it.fPos[i]].T == t {
			p := s.Floats[it.fPos[i]]
			it.vec = append(it.vec, promql.Sample{Metric: s.Metric, T: p.T, F: p.F})
			continue
		}

		for it.hPos[i] < len(s.Histograms) && s.Histograms[it.hPos[i]].T < t {
			it.hPos[i]++
		}
		if it.hPos[i] < len(s.Histograms) && s.Histograms[it.hPos[i]].T == t {
			p := s.Histograms[it.hPos[i]]
			it.vec = append(it.vec, promql.Sample{Metric: s.Metric, T: p.T, H: p.H})
		}
	}
	return it.vec
}
