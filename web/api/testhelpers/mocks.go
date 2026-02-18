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

// This file contains mock implementations of API dependencies for testing.
package testhelpers

import (
	"context"
	"net/url"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// LazyLoader allows lazy initialization of mocks per test.
type LazyLoader[T any] struct {
	loader func() T
	value  *T
}

// NewLazyLoader creates a new LazyLoader with the given loader function.
func NewLazyLoader[T any](loader func() T) *LazyLoader[T] {
	return &LazyLoader[T]{loader: loader}
}

// Get returns the loaded value, initializing it if necessary.
func (l *LazyLoader[T]) Get() T {
	if l.value == nil {
		v := l.loader()
		l.value = &v
	}
	return *l.value
}

// FakeQueryable implements storage.SampleAndChunkQueryable with configurable behavior.
type FakeQueryable struct {
	series []storage.Series
}

func (f *FakeQueryable) Querier(_, _ int64) (storage.Querier, error) {
	return &FakeQuerier{series: f.series}, nil
}

func (f *FakeQueryable) ChunkQuerier(_, _ int64) (storage.ChunkQuerier, error) {
	return &FakeChunkQuerier{series: f.series}, nil
}

// FakeQuerier implements storage.Querier.
type FakeQuerier struct {
	series []storage.Series
}

func (f *FakeQuerier) Select(_ context.Context, _ bool, _ *storage.SelectHints, _ ...*labels.Matcher) storage.SeriesSet {
	return &FakeSeriesSet{series: f.series, idx: -1}
}

func (f *FakeQuerier) LabelValues(_ context.Context, name string, _ *storage.LabelHints, _ ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	valuesMap := make(map[string]struct{})
	for _, s := range f.series {
		lbls := s.Labels()
		if val := lbls.Get(name); val != "" {
			valuesMap[val] = struct{}{}
		}
	}
	values := make([]string, 0, len(valuesMap))
	for v := range valuesMap {
		values = append(values, v)
	}
	return values, nil, nil
}

func (f *FakeQuerier) LabelNames(_ context.Context, _ *storage.LabelHints, _ ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	namesMap := make(map[string]struct{})
	for _, s := range f.series {
		lbls := s.Labels()
		lbls.Range(func(l labels.Label) {
			namesMap[l.Name] = struct{}{}
		})
	}
	names := make([]string, 0, len(namesMap))
	for n := range namesMap {
		names = append(names, n)
	}
	return names, nil, nil
}

func (*FakeQuerier) Close() error {
	return nil
}

// FakeChunkQuerier implements storage.ChunkQuerier.
type FakeChunkQuerier struct {
	series []storage.Series
}

func (f *FakeChunkQuerier) Select(_ context.Context, _ bool, _ *storage.SelectHints, _ ...*labels.Matcher) storage.ChunkSeriesSet {
	return &FakeChunkSeriesSet{series: f.series, idx: -1}
}

func (f *FakeChunkQuerier) LabelValues(_ context.Context, name string, _ *storage.LabelHints, _ ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	valuesMap := make(map[string]struct{})
	for _, s := range f.series {
		lbls := s.Labels()
		if val := lbls.Get(name); val != "" {
			valuesMap[val] = struct{}{}
		}
	}
	values := make([]string, 0, len(valuesMap))
	for v := range valuesMap {
		values = append(values, v)
	}
	return values, nil, nil
}

func (f *FakeChunkQuerier) LabelNames(_ context.Context, _ *storage.LabelHints, _ ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	namesMap := make(map[string]struct{})
	for _, s := range f.series {
		lbls := s.Labels()
		lbls.Range(func(l labels.Label) {
			namesMap[l.Name] = struct{}{}
		})
	}
	names := make([]string, 0, len(namesMap))
	for n := range namesMap {
		names = append(names, n)
	}
	return names, nil, nil
}

func (*FakeChunkQuerier) Close() error {
	return nil
}

// FakeSeriesSet implements storage.SeriesSet.
type FakeSeriesSet struct {
	series []storage.Series
	idx    int
}

func (f *FakeSeriesSet) Next() bool {
	f.idx++
	return f.idx < len(f.series)
}

func (f *FakeSeriesSet) At() storage.Series {
	return f.series[f.idx]
}

func (*FakeSeriesSet) Err() error {
	return nil
}

func (*FakeSeriesSet) Warnings() annotations.Annotations {
	return nil
}

// FakeChunkSeriesSet implements storage.ChunkSeriesSet.
type FakeChunkSeriesSet struct {
	series []storage.Series
	idx    int
}

func (f *FakeChunkSeriesSet) Next() bool {
	f.idx++
	return f.idx < len(f.series)
}

func (f *FakeChunkSeriesSet) At() storage.ChunkSeries {
	return &FakeChunkSeries{series: f.series[f.idx]}
}

func (*FakeChunkSeriesSet) Err() error {
	return nil
}

func (*FakeChunkSeriesSet) Warnings() annotations.Annotations {
	return nil
}

// FakeChunkSeries implements storage.ChunkSeries.
type FakeChunkSeries struct {
	series storage.Series
}

func (f *FakeChunkSeries) Labels() labels.Labels {
	return f.series.Labels()
}

func (*FakeChunkSeries) Iterator(_ chunks.Iterator) chunks.Iterator {
	return &FakeChunkSeriesIterator{}
}

// FakeChunkSeriesIterator implements chunks.Iterator.
type FakeChunkSeriesIterator struct{}

func (*FakeChunkSeriesIterator) Next() bool {
	return false
}

func (*FakeChunkSeriesIterator) At() chunks.Meta {
	return chunks.Meta{}
}

func (*FakeChunkSeriesIterator) Err() error {
	return nil
}

// FakeSeries implements storage.Series.
type FakeSeries struct {
	labels  labels.Labels
	samples []promql.FPoint
}

func (f *FakeSeries) Labels() labels.Labels {
	return f.labels
}

func (f *FakeSeries) Iterator(chunkenc.Iterator) chunkenc.Iterator {
	return &FakeSeriesIterator{samples: f.samples, idx: -1}
}

// FakeSeriesIterator implements chunkenc.Iterator.
type FakeSeriesIterator struct {
	samples []promql.FPoint
	idx     int
}

func (f *FakeSeriesIterator) Next() chunkenc.ValueType {
	f.idx++
	if f.idx < len(f.samples) {
		return chunkenc.ValFloat
	}
	return chunkenc.ValNone
}

func (f *FakeSeriesIterator) Seek(t int64) chunkenc.ValueType {
	for f.idx < len(f.samples)-1 {
		f.idx++
		if f.samples[f.idx].T >= t {
			return chunkenc.ValFloat
		}
	}
	return chunkenc.ValNone
}

func (f *FakeSeriesIterator) At() (int64, float64) {
	s := f.samples[f.idx]
	return s.T, s.F
}

func (*FakeSeriesIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("not implemented")
}

func (*FakeSeriesIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("not implemented")
}

func (f *FakeSeriesIterator) AtT() int64 {
	return f.samples[f.idx].T
}

func (*FakeSeriesIterator) AtST() int64 {
	return 0
}

func (*FakeSeriesIterator) Err() error {
	return nil
}

// FakeHistogramSeries implements storage.Series for histogram data.
type FakeHistogramSeries struct {
	labels     labels.Labels
	histograms []promql.HPoint
}

func (f *FakeHistogramSeries) Labels() labels.Labels {
	return f.labels
}

func (f *FakeHistogramSeries) Iterator(chunkenc.Iterator) chunkenc.Iterator {
	return &FakeHistogramSeriesIterator{histograms: f.histograms, idx: -1}
}

// FakeHistogramSeriesIterator implements chunkenc.Iterator for histogram data.
type FakeHistogramSeriesIterator struct {
	histograms []promql.HPoint
	idx        int
}

func (f *FakeHistogramSeriesIterator) Next() chunkenc.ValueType {
	f.idx++
	if f.idx < len(f.histograms) {
		return chunkenc.ValFloatHistogram
	}
	return chunkenc.ValNone
}

func (f *FakeHistogramSeriesIterator) Seek(t int64) chunkenc.ValueType {
	for f.idx < len(f.histograms)-1 {
		f.idx++
		if f.histograms[f.idx].T >= t {
			return chunkenc.ValFloatHistogram
		}
	}
	return chunkenc.ValNone
}

func (*FakeHistogramSeriesIterator) At() (int64, float64) {
	panic("not a float value")
}

func (*FakeHistogramSeriesIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("not implemented")
}

func (f *FakeHistogramSeriesIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	h := f.histograms[f.idx]
	return h.T, h.H
}

func (f *FakeHistogramSeriesIterator) AtT() int64 {
	return f.histograms[f.idx].T
}

func (*FakeHistogramSeriesIterator) AtST() int64 {
	return 0
}

func (*FakeHistogramSeriesIterator) Err() error {
	return nil
}

// FakeExemplarQueryable implements storage.ExemplarQueryable.
type FakeExemplarQueryable struct{}

func (*FakeExemplarQueryable) ExemplarQuerier(_ context.Context) (storage.ExemplarQuerier, error) {
	return &FakeExemplarQuerier{}, nil
}

// FakeExemplarQuerier implements storage.ExemplarQuerier.
type FakeExemplarQuerier struct{}

func (*FakeExemplarQuerier) Select(_, _ int64, _ ...[]*labels.Matcher) ([]exemplar.QueryResult, error) {
	return nil, nil
}

// FakeRulesRetriever implements v1.RulesRetriever.
type FakeRulesRetriever struct {
	groups []*rules.Group
}

func (f *FakeRulesRetriever) RuleGroups() []*rules.Group {
	return f.groups
}

func (f *FakeRulesRetriever) AlertingRules() []*rules.AlertingRule {
	var alertingRules []*rules.AlertingRule
	for _, g := range f.groups {
		for _, r := range g.Rules() {
			if ar, ok := r.(*rules.AlertingRule); ok {
				alertingRules = append(alertingRules, ar)
			}
		}
	}
	return alertingRules
}

// FakeTargetRetriever implements v1.TargetRetriever.
type FakeTargetRetriever struct {
	active        map[string][]*scrape.Target
	dropped       map[string][]*scrape.Target
	droppedCounts map[string]int
	scrapeConfig  map[string]*config.ScrapeConfig
}

func (f *FakeTargetRetriever) TargetsActive() map[string][]*scrape.Target {
	if f.active == nil {
		return make(map[string][]*scrape.Target)
	}
	return f.active
}

func (f *FakeTargetRetriever) TargetsDropped() map[string][]*scrape.Target {
	if f.dropped == nil {
		return make(map[string][]*scrape.Target)
	}
	return f.dropped
}

func (f *FakeTargetRetriever) TargetsDroppedCounts() map[string]int {
	if f.droppedCounts == nil {
		return make(map[string]int)
	}
	return f.droppedCounts
}

func (f *FakeTargetRetriever) ScrapePoolConfig(name string) (*config.ScrapeConfig, error) {
	if f.scrapeConfig == nil {
		return nil, nil
	}
	return f.scrapeConfig[name], nil
}

// FakeScrapePoolsRetriever implements v1.ScrapePoolsRetriever.
type FakeScrapePoolsRetriever struct {
	pools []string
}

func (f *FakeScrapePoolsRetriever) ScrapePools() []string {
	if f.pools == nil {
		return []string{}
	}
	return f.pools
}

// FakeAlertmanagerRetriever implements v1.AlertmanagerRetriever.
type FakeAlertmanagerRetriever struct{}

func (*FakeAlertmanagerRetriever) Alertmanagers() []*url.URL {
	return nil
}

func (*FakeAlertmanagerRetriever) DroppedAlertmanagers() []*url.URL {
	return nil
}

// FakeTSDBAdminStats implements v1.TSDBAdminStats.
type FakeTSDBAdminStats struct{}

func (*FakeTSDBAdminStats) CleanTombstones() error {
	return nil
}

func (*FakeTSDBAdminStats) Delete(_ context.Context, _, _ int64, _ ...*labels.Matcher) error {
	return nil
}

func (*FakeTSDBAdminStats) Snapshot(_ string, _ bool) error {
	return nil
}

func (*FakeTSDBAdminStats) Stats(_ string, _ int) (*tsdb.Stats, error) {
	return &tsdb.Stats{}, nil
}

func (*FakeTSDBAdminStats) WALReplayStatus() (tsdb.WALReplayStatus, error) {
	return tsdb.WALReplayStatus{}, nil
}

func (*FakeTSDBAdminStats) BlockMetas() ([]tsdb.BlockMeta, error) {
	return []tsdb.BlockMeta{}, nil
}

// NewEmptyQueryable returns a queryable with no series.
func NewEmptyQueryable() storage.SampleAndChunkQueryable {
	return &FakeQueryable{series: []storage.Series{}}
}

// NewQueryableWithSeries returns a queryable with the given series.
func NewQueryableWithSeries(series []storage.Series) storage.SampleAndChunkQueryable {
	return &FakeQueryable{series: series}
}

// TSDBNotReadyQueryable implements storage.SampleAndChunkQueryable that returns tsdb.ErrNotReady.
type TSDBNotReadyQueryable struct{}

func (*TSDBNotReadyQueryable) Querier(_, _ int64) (storage.Querier, error) {
	return nil, tsdb.ErrNotReady
}

func (*TSDBNotReadyQueryable) ChunkQuerier(_, _ int64) (storage.ChunkQuerier, error) {
	return nil, tsdb.ErrNotReady
}

// NewTSDBNotReadyQueryable returns a queryable that always returns tsdb.ErrNotReady.
func NewTSDBNotReadyQueryable() storage.SampleAndChunkQueryable {
	return &TSDBNotReadyQueryable{}
}

// NewEmptyExemplarQueryable returns an exemplar queryable with no exemplars.
func NewEmptyExemplarQueryable() storage.ExemplarQueryable {
	return &FakeExemplarQueryable{}
}

// NewEmptyRulesRetriever returns a rules retriever with no rules.
func NewEmptyRulesRetriever() *FakeRulesRetriever {
	return &FakeRulesRetriever{groups: []*rules.Group{}}
}

// NewRulesRetrieverWithGroups returns a rules retriever with the given groups.
func NewRulesRetrieverWithGroups(groups []*rules.Group) *FakeRulesRetriever {
	return &FakeRulesRetriever{groups: groups}
}

// NewEmptyTargetRetriever returns a target retriever with no targets.
func NewEmptyTargetRetriever() *FakeTargetRetriever {
	return &FakeTargetRetriever{}
}

// NewEmptyScrapePoolsRetriever returns a scrape pools retriever with no pools.
func NewEmptyScrapePoolsRetriever() *FakeScrapePoolsRetriever {
	return &FakeScrapePoolsRetriever{pools: []string{}}
}

// NewEmptyAlertmanagerRetriever returns an alertmanager retriever with no alertmanagers.
func NewEmptyAlertmanagerRetriever() *FakeAlertmanagerRetriever {
	return &FakeAlertmanagerRetriever{}
}

// NewEmptyTSDBAdminStats returns a TSDB admin stats with no-op implementations.
func NewEmptyTSDBAdminStats() *FakeTSDBAdminStats {
	return &FakeTSDBAdminStats{}
}
