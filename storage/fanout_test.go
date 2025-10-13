// Copyright 2020 The Prometheus Authors
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

package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestFanout_SelectSorted(t *testing.T) {
	inputLabel := labels.FromStrings(model.MetricNameLabel, "a")
	outputLabel := labels.FromStrings(model.MetricNameLabel, "a")

	inputTotalSize := 0
	ctx := context.Background()

	priStorage := teststorage.New(t)
	defer priStorage.Close()
	app1 := priStorage.Appender(ctx)
	app1.Append(0, inputLabel, 0, 0)
	inputTotalSize++
	app1.Append(0, inputLabel, 1000, 1)
	inputTotalSize++
	app1.Append(0, inputLabel, 2000, 2)
	inputTotalSize++
	err := app1.Commit()
	require.NoError(t, err)

	remoteStorage1 := teststorage.New(t)
	defer remoteStorage1.Close()
	app2 := remoteStorage1.Appender(ctx)
	app2.Append(0, inputLabel, 3000, 3)
	inputTotalSize++
	app2.Append(0, inputLabel, 4000, 4)
	inputTotalSize++
	app2.Append(0, inputLabel, 5000, 5)
	inputTotalSize++
	err = app2.Commit()
	require.NoError(t, err)

	remoteStorage2 := teststorage.New(t)
	defer remoteStorage2.Close()

	app3 := remoteStorage2.Appender(ctx)
	app3.Append(0, inputLabel, 6000, 6)
	inputTotalSize++
	app3.Append(0, inputLabel, 7000, 7)
	inputTotalSize++
	app3.Append(0, inputLabel, 8000, 8)
	inputTotalSize++

	err = app3.Commit()
	require.NoError(t, err)

	fanoutStorage := storage.NewFanout(nil, priStorage, remoteStorage1, remoteStorage2)

	t.Run("querier", func(t *testing.T) {
		querier, err := fanoutStorage.Querier(0, 8000)
		require.NoError(t, err)
		defer querier.Close()

		matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
		require.NoError(t, err)

		seriesSet := querier.Select(ctx, true, nil, matcher)

		result := make(map[int64]float64)
		var labelsResult labels.Labels
		var iterator chunkenc.Iterator
		for seriesSet.Next() {
			series := seriesSet.At()
			seriesLabels := series.Labels()
			labelsResult = seriesLabels
			iterator := series.Iterator(iterator)
			for iterator.Next() == chunkenc.ValFloat {
				timestamp, value := iterator.At()
				result[timestamp] = value
			}
		}

		require.Equal(t, labelsResult, outputLabel)
		require.Len(t, result, inputTotalSize)
	})
	t.Run("chunk querier", func(t *testing.T) {
		querier, err := fanoutStorage.ChunkQuerier(0, 8000)
		require.NoError(t, err)
		defer querier.Close()

		matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
		require.NoError(t, err)

		seriesSet := storage.NewSeriesSetFromChunkSeriesSet(querier.Select(ctx, true, nil, matcher))

		result := make(map[int64]float64)
		var labelsResult labels.Labels
		var iterator chunkenc.Iterator
		for seriesSet.Next() {
			series := seriesSet.At()
			seriesLabels := series.Labels()
			labelsResult = seriesLabels
			iterator := series.Iterator(iterator)
			for iterator.Next() == chunkenc.ValFloat {
				timestamp, value := iterator.At()
				result[timestamp] = value
			}
		}

		require.NoError(t, seriesSet.Err())
		require.Equal(t, labelsResult, outputLabel)
		require.Len(t, result, inputTotalSize)
	})
}

func TestFanoutErrors(t *testing.T) {
	workingStorage := teststorage.New(t)
	defer workingStorage.Close()

	cases := []struct {
		primary   storage.Storage
		secondary storage.Storage
		warning   error
		err       error
	}{
		{
			primary:   workingStorage,
			secondary: errStorage{},
			warning:   errSelect,
			err:       nil,
		},
		{
			primary:   errStorage{},
			secondary: workingStorage,
			warning:   nil,
			err:       errSelect,
		},
	}

	for _, tc := range cases {
		fanoutStorage := storage.NewFanout(nil, tc.primary, tc.secondary)

		t.Run("samples", func(t *testing.T) {
			querier, err := fanoutStorage.Querier(0, 8000)
			require.NoError(t, err)
			defer querier.Close()

			matcher := labels.MustNewMatcher(labels.MatchEqual, "a", "b")
			ss := querier.Select(context.Background(), true, nil, matcher)

			// Exhaust.
			for ss.Next() {
				ss.At()
			}

			if tc.err != nil {
				require.EqualError(t, ss.Err(), tc.err.Error())
			}

			if tc.warning != nil {
				w := ss.Warnings()
				require.NotEmpty(t, w, "warnings expected")
				require.EqualError(t, w.AsErrors()[0], tc.warning.Error())
			}
		})
		t.Run("chunks", func(t *testing.T) {
			t.Skip("enable once TestStorage and TSDB implements ChunkQuerier")
			querier, err := fanoutStorage.ChunkQuerier(0, 8000)
			require.NoError(t, err)
			defer querier.Close()

			matcher := labels.MustNewMatcher(labels.MatchEqual, "a", "b")
			ss := querier.Select(context.Background(), true, nil, matcher)

			// Exhaust.
			for ss.Next() {
				ss.At()
			}

			if tc.err != nil {
				require.EqualError(t, ss.Err(), tc.err.Error())
			}

			if tc.warning != nil {
				w := ss.Warnings()
				require.NotEmpty(t, w, "warnings expected")
				require.EqualError(t, w.AsErrors()[0], tc.warning.Error())
			}
		})
	}
}

var errSelect = errors.New("select error")

type errStorage struct{}

type errQuerier struct{}

func (errStorage) Querier(_, _ int64) (storage.Querier, error) {
	return errQuerier{}, nil
}

type errChunkQuerier struct{ errQuerier }

func (errStorage) ChunkQuerier(_, _ int64) (storage.ChunkQuerier, error) {
	return errChunkQuerier{}, nil
}
func (errStorage) Appender(context.Context) storage.Appender { return nil }
func (errStorage) StartTime() (int64, error)                 { return 0, nil }
func (errStorage) Close() error                              { return nil }

func (errQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	return storage.ErrSeriesSet(errSelect)
}

func (errQuerier) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, errors.New("label values error")
}

func (errQuerier) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, errors.New("label names error")
}

func (errQuerier) Close() error { return nil }

func (errChunkQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.ChunkSeriesSet {
	return storage.ErrChunkSeriesSet(errSelect)
}
