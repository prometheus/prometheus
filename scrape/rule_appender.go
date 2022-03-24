// Copyright 2015 The Prometheus Authors
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

package scrape

import (
	"context"
	"time"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
)

type ruleAppender struct {
	targetLabels labels.Labels
	rules        []*config.AggregationRuleConfig
	appendable   storage.Appendable
	engine       *promql.Engine
}

type batch struct {
	targetLabels labels.Labels
	rules        []*config.AggregationRuleConfig
	samples      []*batchSample
	appender     storage.Appender
	engine       *promql.Engine
}

type batchSample struct {
	metric labels.Labels
	t      int64
	v      float64
}

func newRuleAppender(targetLabels labels.Labels, appendable storage.Appendable, rules []*config.AggregationRuleConfig) storage.Appendable {
	return &ruleAppender{
		targetLabels: targetLabels,
		rules:        rules,
		appendable:   appendable,
		engine: promql.NewEngine(promql.EngineOpts{
			MaxSamples:    50000000,
			Timeout:       10 * time.Second,
			LookbackDelta: 15 * time.Minute,
		}),
	}
}

func (a ruleAppender) Appender(ctx context.Context) storage.Appender {
	return &batch{
		targetLabels: a.targetLabels,
		appender:     a.appendable.Appender(ctx),
		rules:        a.rules,
		samples:      make([]*batchSample, 0),
		engine:       a.engine,
	}
}

func (b *batch) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	b.samples = append(b.samples, &batchSample{metric: l, t: t, v: v})
	return b.appender.Append(ref, l, t, v)
}

func (b *batch) Commit() error {
	err := b.evaluateRules()
	if err != nil {
		return err
	}

	return b.appender.Commit()
}

func (b *batch) evaluateRules() error {
	if len(b.samples) == 0 {
		return nil
	}

	for _, rule := range b.rules {
		queryable := storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
			return b, nil
		})
		ts := b.samples[0].t
		query, err := b.engine.NewInstantQuery(queryable, nil, rule.Expr, time.UnixMilli(ts))
		if err != nil {
			return err
		}

		result := query.Exec(context.Background())
		samples, err := result.Vector()
		if err != nil {
			return err
		}

		for _, s := range samples {
			lbls := s.Metric.WithoutLabels("__name__")
			lbls = append(lbls, labels.Label{
				Name:  "__name__",
				Value: rule.Record,
			})
			lbls = append(lbls, b.targetLabels...)
			_, err := b.appender.Append(0, lbls, s.T, s.V)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (b *batch) Rollback() error {
	return b.appender.Rollback()
}

func (b *batch) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	return b.appender.AppendExemplar(ref, l, e)
}

func (b *batch) Select(_ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	var samples []*batchSample
	for _, s := range b.samples {
		match := true
		for _, matcher := range matchers {
			if !matcher.Matches(s.metric.Get(matcher.Name)) {
				match = false
				break
			}
		}
		if match {
			samples = append(samples, s)
		}
	}

	return &seriesSet{
		i:       -1,
		samples: samples,
	}
}

func (b *batch) LabelNames(matchers ...*labels.Matcher) ([]string, storage.Warnings, error) {
	return nil, nil, nil
}

func (b *batch) LabelValues(name string, matchers ...*labels.Matcher) ([]string, storage.Warnings, error) {
	return nil, nil, nil
}

func (b *batch) Close() error {
	return nil
}

type seriesSet struct {
	i       int
	samples []*batchSample
}

func (s *seriesSet) Next() bool {
	s.i++
	if s.i == len(s.samples) {
		return false
	}
	return true
}

func (s *seriesSet) At() storage.Series {
	sample := s.samples[s.i]
	return promql.NewStorageSeries(promql.Series{
		Metric: sample.metric,
		Points: []promql.Point{
			{
				T: sample.t,
				V: sample.v,
			},
		},
	})
}

func (s *seriesSet) Err() error {
	return nil
}

func (s *seriesSet) Warnings() storage.Warnings {
	return nil
}
