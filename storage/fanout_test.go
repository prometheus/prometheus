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
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
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
		querier, err := fanoutStorage.Querier(context.Background(), 0, 8000)
		require.NoError(t, err)
		defer querier.Close()

		matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
		require.NoError(t, err)

		seriesSet := querier.Select(true, nil, matcher)

		result := make(map[int64]float64)
		var labelsResult labels.Labels
		for seriesSet.Next() {
			series := seriesSet.At()
			seriesLabels := series.Labels()
			labelsResult = seriesLabels
			iterator := series.Iterator()
			for iterator.Next() {
				timestamp, value := iterator.At()
				result[timestamp] = value
			}
		}

		require.Equal(t, labelsResult, outputLabel)
		require.Equal(t, inputTotalSize, len(result))
	})
	t.Run("chunk querier", func(t *testing.T) {
		querier, err := fanoutStorage.ChunkQuerier(ctx, 0, 8000)
		require.NoError(t, err)
		defer querier.Close()

		matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
		require.NoError(t, err)

		seriesSet := storage.NewSeriesSetFromChunkSeriesSet(querier.Select(true, nil, matcher))

		result := make(map[int64]float64)
		var labelsResult labels.Labels
		for seriesSet.Next() {
			series := seriesSet.At()
			seriesLabels := series.Labels()
			labelsResult = seriesLabels
			iterator := series.Iterator()
			for iterator.Next() {
				timestamp, value := iterator.At()
				result[timestamp] = value
			}
		}

		require.NoError(t, seriesSet.Err())
		require.Equal(t, labelsResult, outputLabel)
		require.Equal(t, inputTotalSize, len(result))
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
			querier, err := fanoutStorage.Querier(context.Background(), 0, 8000)
			require.NoError(t, err)
			defer querier.Close()

			matcher := labels.MustNewMatcher(labels.MatchEqual, "a", "b")
			ss := querier.Select(true, nil, matcher)

			// Exhaust.
			for ss.Next() {
				ss.At()
			}

			if tc.err != nil {
				require.Error(t, ss.Err())
				require.Equal(t, tc.err.Error(), ss.Err().Error())
			}

			if tc.warning != nil {
				require.Greater(t, len(ss.Warnings()), 0, "warnings expected")
				require.Error(t, ss.Warnings()[0])
				require.Equal(t, tc.warning.Error(), ss.Warnings()[0].Error())
			}
		})
		t.Run("chunks", func(t *testing.T) {
			t.Skip("enable once TestStorage and TSDB implements ChunkQuerier")
			querier, err := fanoutStorage.ChunkQuerier(context.Background(), 0, 8000)
			require.NoError(t, err)
			defer querier.Close()

			matcher := labels.MustNewMatcher(labels.MatchEqual, "a", "b")
			ss := querier.Select(true, nil, matcher)

			// Exhaust.
			for ss.Next() {
				ss.At()
			}

			if tc.err != nil {
				require.Error(t, ss.Err())
				require.Equal(t, tc.err.Error(), ss.Err().Error())
			}

			if tc.warning != nil {
				require.Greater(t, len(ss.Warnings()), 0, "warnings expected")
				require.Error(t, ss.Warnings()[0])
				require.Equal(t, tc.warning.Error(), ss.Warnings()[0].Error())
			}
		})
	}
}

var errSelect = errors.New("select error")

type errStorage struct{}

type errQuerier struct{}

func (errStorage) Querier(_ context.Context, _, _ int64) (storage.Querier, error) {
	return errQuerier{}, nil
}

type errChunkQuerier struct{ errQuerier }

func (errStorage) ChunkQuerier(_ context.Context, _, _ int64) (storage.ChunkQuerier, error) {
	return errChunkQuerier{}, nil
}
func (errStorage) Appender(_ context.Context) storage.Appender { return nil }
func (errStorage) StartTime() (int64, error)                   { return 0, nil }
func (errStorage) Close() error                                { return nil }

func (errQuerier) Select(bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	return storage.ErrSeriesSet(errSelect)
}

func (errQuerier) LabelValues(name string, matchers ...*labels.Matcher) ([]string, storage.Warnings, error) {
	return nil, nil, errors.New("label values error")
}

func (errQuerier) LabelNames() ([]string, storage.Warnings, error) {
	return nil, nil, errors.New("label names error")
}

func (errQuerier) Close() error { return nil }

func (errChunkQuerier) Select(bool, *storage.SelectHints, ...*labels.Matcher) storage.ChunkSeriesSet {
	return storage.ErrChunkSeriesSet(errSelect)
}
