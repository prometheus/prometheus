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

package storage

import (
	"context"
	"math"

	"github.com/prometheus/prometheus/pkg/labels"
)

type noopStorage struct{}

func NoopStorage() Storage {
	return &noopStorage{}
}

// Querier returns a new Querier on the storage.
func (n *noopStorage) Querier(ctx context.Context, mint, maxt int64) (Querier, error) {
	return NoopQuerier(), nil
}

// StartTime returns the oldest timestamp stored in the storage.
func (n *noopStorage) StartTime() (int64, error) {
	return 0, nil
}

// Appender returns a new appender against the storage.
func (n *noopStorage) Appender() (Appender, error) {
	return NoopAppender(), nil
}

// Close closes the storage and all its underlying resources.
func (n *noopStorage) Close() error {
	return nil
}

type noopAppender struct{}

func NoopAppender() Appender {
	return &noopAppender{}
}

func (a *noopAppender) Add(l labels.Labels, t int64, v float64) (uint64, error) {
	return 0, nil
}

func (a *noopAppender) AddFast(l labels.Labels, ref uint64, t int64, v float64) error {
	return nil
}

// Commit submits the collected samples and purges the batch.
func (a *noopAppender) Commit() error   { return nil }
func (a *noopAppender) Rollback() error { return nil }

type noopQuerier struct{}

// NoopQuerier is a Querier that does nothing.
func NoopQuerier() Querier {
	return noopQuerier{}
}

func (noopQuerier) Select(*SelectParams, ...*labels.Matcher) (SeriesSet, Warnings, error) {
	return NoopSeriesSet(), nil, nil
}

func (noopQuerier) LabelValues(name string) ([]string, error) {
	return nil, nil
}

func (noopQuerier) LabelNames() ([]string, error) {
	return nil, nil
}

func (noopQuerier) Close() error {
	return nil
}

type noopSeriesSet struct{}

// NoopSeriesSet is a SeriesSet that does nothing.
func NoopSeriesSet() SeriesSet {
	return noopSeriesSet{}
}

func (noopSeriesSet) Next() bool {
	return false
}

func (noopSeriesSet) At() Series {
	return nil
}

func (noopSeriesSet) Err() error {
	return nil
}

type noopSeriesIterator struct{}

// NoopSeriesIt is a SeriesIterator that does nothing.
var NoopSeriesIt = noopSeriesIterator{}

func (noopSeriesIterator) At() (int64, float64) {
	return math.MinInt64, 0
}

func (noopSeriesIterator) Seek(t int64) bool {
	return false
}

func (noopSeriesIterator) Next() bool {
	return false
}

func (noopSeriesIterator) Err() error {
	return nil
}
