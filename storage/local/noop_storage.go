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

package local

import (
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage/metric"
)

// NoopStorage is a dummy storage for use when Prometheus's local storage is
// disabled. It throws away any appended samples and returns empty results.
type NoopStorage struct{}

// Start implements Storage.
func (s *NoopStorage) Start() (err error) {
	return nil
}

// Stop implements Storage.
func (s *NoopStorage) Stop() error {
	return nil
}

// WaitForIndexing implements Storage.
func (s *NoopStorage) WaitForIndexing() {
}

// Querier implements Storage.
func (s *NoopStorage) Querier() (Querier, error) {
	return &NoopQuerier{}, nil
}

// NoopQuerier is a dummy Querier for use when Prometheus's local storage is
// disabled. It is returned by the NoopStorage Querier method and always returns
// empty results.
type NoopQuerier struct{}

// Close implements Querier.
func (s *NoopQuerier) Close() error {
	return nil
}

// LastSampleForLabelMatchers implements Querier.
func (s *NoopQuerier) LastSampleForLabelMatchers(ctx context.Context, cutoff model.Time, matcherSets ...metric.LabelMatchers) (model.Vector, error) {
	return nil, nil
}

// QueryRange implements Querier
func (s *NoopQuerier) QueryRange(ctx context.Context, from, through model.Time, matchers ...*metric.LabelMatcher) ([]SeriesIterator, error) {
	return nil, nil
}

// QueryInstant implements Querier.
func (s *NoopQuerier) QueryInstant(ctx context.Context, ts model.Time, stalenessDelta time.Duration, matchers ...*metric.LabelMatcher) ([]SeriesIterator, error) {
	return nil, nil
}

// MetricsForLabelMatchers implements Querier.
func (s *NoopQuerier) MetricsForLabelMatchers(
	ctx context.Context,
	from, through model.Time,
	matcherSets ...metric.LabelMatchers,
) ([]metric.Metric, error) {
	return nil, nil
}

// LabelValuesForLabelName implements Querier.
func (s *NoopQuerier) LabelValuesForLabelName(ctx context.Context, labelName model.LabelName) (model.LabelValues, error) {
	return nil, nil
}

// DropMetricsForLabelMatchers implements Storage.
func (s *NoopStorage) DropMetricsForLabelMatchers(ctx context.Context, matchers ...*metric.LabelMatcher) (int, error) {
	return 0, nil
}

// Append implements Storage.
func (s *NoopStorage) Append(sample *model.Sample) error {
	return nil
}

// NeedsThrottling implements Storage.
func (s *NoopStorage) NeedsThrottling() bool {
	return false
}
