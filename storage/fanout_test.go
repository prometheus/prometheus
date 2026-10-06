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

package storage_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestFanout_SelectSorted(t *testing.T) {
	inputLabel := labels.FromStrings(model.MetricNameLabel, "a")
	outputLabel := labels.FromStrings(model.MetricNameLabel, "a")

	inputTotalSize := 0
	ctx := context.Background()

	priStorage := teststorage.New(t)
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

func TestFanout_SelectSorted_AppenderV2(t *testing.T) {
	inputLabel := labels.FromStrings(model.MetricNameLabel, "a")
	outputLabel := labels.FromStrings(model.MetricNameLabel, "a")

	inputTotalSize := 0

	priStorage := teststorage.New(t)
	app1 := priStorage.AppenderV2(t.Context())
	_, err := app1.Append(0, inputLabel, 0, 0, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++
	_, err = app1.Append(0, inputLabel, 0, 1000, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++
	_, err = app1.Append(0, inputLabel, 0, 2000, 2, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++
	require.NoError(t, app1.Commit())

	remoteStorage1 := teststorage.New(t)
	app2 := remoteStorage1.AppenderV2(t.Context())
	_, err = app2.Append(0, inputLabel, 0, 3000, 3, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++
	_, err = app2.Append(0, inputLabel, 0, 4000, 4, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++
	_, err = app2.Append(0, inputLabel, 0, 5000, 5, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++
	require.NoError(t, app2.Commit())

	remoteStorage2 := teststorage.New(t)
	app3 := remoteStorage2.AppenderV2(t.Context())
	_, err = app3.Append(0, inputLabel, 0, 6000, 6, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++
	_, err = app3.Append(0, inputLabel, 0, 7000, 7, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++
	_, err = app3.Append(0, inputLabel, 0, 8000, 8, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	inputTotalSize++

	require.NoError(t, app3.Commit())

	fanoutStorage := storage.NewFanout(nil, priStorage, remoteStorage1, remoteStorage2)

	t.Run("querier", func(t *testing.T) {
		querier, err := fanoutStorage.Querier(0, 8000)
		require.NoError(t, err)
		defer querier.Close()

		matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
		require.NoError(t, err)

		seriesSet := querier.Select(t.Context(), true, nil, matcher)

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

		seriesSet := storage.NewSeriesSetFromChunkSeriesSet(querier.Select(t.Context(), true, nil, matcher))

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
func (errStorage) Appender(context.Context) storage.Appender     { return nil }
func (errStorage) AppenderV2(context.Context) storage.AppenderV2 { return nil }
func (errStorage) StartTime() (int64, error)                     { return 0, nil }
func (errStorage) Close() error                                  { return nil }

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

type mockStorage struct {
	app   storage.Appendable
	appV2 storage.AppendableV2
	storage.Storage
}

func (m mockStorage) Appender(ctx context.Context) storage.Appender {
	return m.app.Appender(ctx)
}

func (m mockStorage) AppenderV2(ctx context.Context) storage.AppenderV2 {
	return m.appV2.AppenderV2(ctx)
}

type sample = teststorage.Sample

func withoutExemplars(s []sample) (ret []sample) {
	ret = make([]sample, len(s))
	copy(ret, s)
	for i := range ret {
		ret[i].ES = nil
	}
	return ret
}

type fanoutAppenderTestCase struct {
	name      string
	primary   *teststorage.Appendable
	secondary *teststorage.Appendable

	expectAppendErr     bool
	expectExemplarError bool
	expectCommitError   bool

	expectPrimarySamples   []sample
	expectSecondarySamples []sample
}

func fanoutAppenderTestCases(expected []sample) []fanoutAppenderTestCase {
	appErr := errors.New("append test error")
	exErr := errors.New("exemplar test error")
	commitErr := errors.New("commit test error")

	return []fanoutAppenderTestCase{
		{
			name:      "both works",
			primary:   teststorage.NewAppendable(),
			secondary: teststorage.NewAppendable(),

			expectPrimarySamples:   expected,
			expectSecondarySamples: expected,
		},
		{
			name:      "primary errors",
			primary:   teststorage.NewAppendable().WithErrs(func(labels.Labels) error { return appErr }, exErr, commitErr),
			secondary: teststorage.NewAppendable(),

			expectAppendErr:     true,
			expectExemplarError: true,
			expectCommitError:   true,
		},
		{
			name:      "exemplar errors",
			primary:   teststorage.NewAppendable().WithErrs(func(labels.Labels) error { return nil }, exErr, nil),
			secondary: teststorage.NewAppendable().WithErrs(func(labels.Labels) error { return nil }, exErr, nil),

			expectAppendErr:     false,
			expectExemplarError: true,
			expectCommitError:   false,

			expectPrimarySamples:   withoutExemplars(expected),
			expectSecondarySamples: withoutExemplars(expected),
		},
		{
			name:      "secondary errors",
			primary:   teststorage.NewAppendable(),
			secondary: teststorage.NewAppendable().WithErrs(func(labels.Labels) error { return appErr }, exErr, commitErr),

			expectAppendErr:     true,
			expectExemplarError: true,
			expectCommitError:   true,

			expectPrimarySamples: expected,
		},
	}
}

func TestFanoutAppender(t *testing.T) {
	h := tsdbutil.GenerateTestHistogram(0)
	fh := tsdbutil.GenerateTestFloatHistogram(0)
	ex := exemplar.Exemplar{Value: 1}

	expected := []sample{
		{L: labels.FromStrings(model.MetricNameLabel, "metric1"), V: 1, ES: []exemplar.Exemplar{ex}},
		{L: labels.FromStrings(model.MetricNameLabel, "metric2"), T: 1, H: h},
		{L: labels.FromStrings(model.MetricNameLabel, "metric3"), T: 2, FH: fh},
	}
	for _, tt := range fanoutAppenderTestCases(expected) {
		t.Run(tt.name, func(t *testing.T) {
			f := storage.NewFanout(nil, mockStorage{app: tt.primary}, mockStorage{app: tt.secondary})

			app := f.Appender(t.Context())
			ref, err := app.Append(0, labels.FromStrings(model.MetricNameLabel, "metric1"), 0, 1)
			if tt.expectAppendErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			_, err = app.AppendExemplar(ref, labels.FromStrings(model.MetricNameLabel, "metric1"), ex)
			if tt.expectExemplarError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			_, err = app.AppendHistogram(0, labels.FromStrings(model.MetricNameLabel, "metric2"), 1, h, nil)
			if tt.expectAppendErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			_, err = app.AppendHistogram(0, labels.FromStrings(model.MetricNameLabel, "metric3"), 2, nil, fh)
			if tt.expectAppendErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			err = app.Commit()
			if tt.expectCommitError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Nil(t, tt.primary.PendingSamples())
			testutil.RequireEqual(t, tt.expectPrimarySamples, tt.primary.ResultSamples())
			require.Nil(t, tt.primary.RolledbackSamples())

			require.Nil(t, tt.secondary.PendingSamples())
			testutil.RequireEqual(t, tt.expectSecondarySamples, tt.secondary.ResultSamples())
			require.Nil(t, tt.secondary.RolledbackSamples())
		})
	}
}

func TestFanoutAppenderV2(t *testing.T) {
	t.Run("indexed failures", func(t *testing.T) { testFanoutIndexedExemplarErrors(t, true) })
	h := tsdbutil.GenerateTestHistogram(0)
	fh := tsdbutil.GenerateTestFloatHistogram(0)
	ex := exemplar.Exemplar{Value: 1}

	expected := []sample{
		{L: labels.FromStrings(model.MetricNameLabel, "metric1"), ST: -1, V: 1, ES: []exemplar.Exemplar{ex}},
		{L: labels.FromStrings(model.MetricNameLabel, "metric2"), ST: -2, T: 1, H: h},
		{L: labels.FromStrings(model.MetricNameLabel, "metric3"), ST: -3, T: 2, FH: fh},
	}

	for _, tt := range fanoutAppenderTestCases(expected) {
		t.Run(tt.name, func(t *testing.T) {
			f := storage.NewFanout(nil, mockStorage{appV2: tt.primary}, mockStorage{appV2: tt.secondary})

			app := f.AppenderV2(t.Context())
			_, err := app.Append(0, labels.FromStrings(model.MetricNameLabel, "metric1"), -1, 0, 1, nil, nil, storage.AOptions{
				Exemplars: []exemplar.Exemplar{ex},
			})
			switch {
			case tt.expectAppendErr:
				require.Error(t, err)
			case tt.expectExemplarError:
				var pErr *storage.AppendPartialError
				require.ErrorAs(t, err, &pErr)
				// Both backends fail the same input exemplar.
				require.Len(t, pErr.ExemplarErrors, 1)
				require.Equal(t, 1, pErr.FailedExemplarCount())
			default:
				require.NoError(t, err)
			}

			_, err = app.Append(0, labels.FromStrings(model.MetricNameLabel, "metric2"), -2, 1, 0, h, nil, storage.AOptions{})
			if tt.expectAppendErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			_, err = app.Append(0, labels.FromStrings(model.MetricNameLabel, "metric3"), -3, 2, 0, nil, fh, storage.AOptions{})
			if tt.expectAppendErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			err = app.Commit()
			if tt.expectCommitError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Nil(t, tt.primary.PendingSamples())
			testutil.RequireEqual(t, tt.expectPrimarySamples, tt.primary.ResultSamples())
			require.Nil(t, tt.primary.RolledbackSamples())

			require.Nil(t, tt.secondary.PendingSamples())
			testutil.RequireEqual(t, tt.expectSecondarySamples, tt.secondary.ResultSamples())
			require.Nil(t, tt.secondary.RolledbackSamples())
		})
	}
}

func TestFanoutAppenderV2_AppendExemplars(t *testing.T) {
	t.Run("indexed failures", func(t *testing.T) { testFanoutIndexedExemplarErrors(t, false) })
	h := tsdbutil.GenerateTestHistogram(0)
	fh := tsdbutil.GenerateTestFloatHistogram(0)
	ex := exemplar.Exemplar{Value: 1}

	expected := []sample{
		{L: labels.FromStrings(model.MetricNameLabel, "metric1"), ST: -1, V: 1, ES: []exemplar.Exemplar{ex}},
		{L: labels.FromStrings(model.MetricNameLabel, "metric2"), ST: -2, T: 1, H: h},
		{L: labels.FromStrings(model.MetricNameLabel, "metric3"), ST: -3, T: 2, FH: fh},
	}

	for _, tt := range fanoutAppenderTestCases(expected) {
		t.Run(tt.name, func(t *testing.T) {
			f := storage.NewFanout(nil, mockStorage{appV2: tt.primary}, mockStorage{appV2: tt.secondary})

			app := f.AppenderV2(t.Context())
			exApp, ok := app.(storage.ExemplarAppenderV2)
			require.True(t, ok)

			ref, err := exApp.Append(0, labels.FromStrings(model.MetricNameLabel, "metric1"), -1, 0, 1, nil, nil, storage.AOptions{})
			if tt.expectAppendErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			_, err = exApp.AppendExemplars(ref, labels.FromStrings(model.MetricNameLabel, "metric1"), []exemplar.Exemplar{ex})
			switch {
			case tt.expectAppendErr:
				require.Error(t, err)
			case tt.expectExemplarError:
				var pErr *storage.AppendPartialError
				require.ErrorAs(t, err, &pErr)
				require.Len(t, pErr.ExemplarErrors, 1)
				require.Equal(t, 1, pErr.FailedExemplarCount())
			default:
				require.NoError(t, err)
			}

			_, err = exApp.Append(0, labels.FromStrings(model.MetricNameLabel, "metric2"), -2, 1, 0, h, nil, storage.AOptions{})
			if tt.expectAppendErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			_, err = exApp.Append(0, labels.FromStrings(model.MetricNameLabel, "metric3"), -3, 2, 0, nil, fh, storage.AOptions{})
			if tt.expectAppendErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			err = exApp.Commit()
			if tt.expectCommitError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Nil(t, tt.primary.PendingSamples())
			testutil.RequireEqual(t, tt.expectPrimarySamples, tt.primary.ResultSamples())
			require.Nil(t, tt.primary.RolledbackSamples())

			require.Nil(t, tt.secondary.PendingSamples())
			testutil.RequireEqual(t, tt.expectSecondarySamples, tt.secondary.ResultSamples())
			require.Nil(t, tt.secondary.RolledbackSamples())
		})
	}

	t.Run("secondary without ExemplarAppenderV2 is skipped", func(t *testing.T) {
		primary := teststorage.NewAppendable()
		secondary := teststorage.NewAppendable()
		f := storage.NewFanout(nil, mockStorage{appV2: primary}, mockStorage{appV2: &appendableV2WithoutExemplars{AppendableV2: secondary}})

		exApp := f.AppenderV2(t.Context()).(storage.ExemplarAppenderV2)
		lbls := labels.FromStrings(model.MetricNameLabel, "metric1")
		ref, err := exApp.Append(0, lbls, -1, 0, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)

		_, err = exApp.AppendExemplars(ref, lbls, []exemplar.Exemplar{ex})
		require.NoError(t, err)
		require.NoError(t, exApp.Commit())

		testutil.RequireEqual(t, []sample{{L: lbls, ST: -1, V: 1, ES: []exemplar.Exemplar{ex}}}, primary.ResultSamples())
		testutil.RequireEqual(t, []sample{{L: lbls, ST: -1, V: 1}}, secondary.ResultSamples())
	})

	t.Run("primary without ExemplarAppenderV2 returns error", func(t *testing.T) {
		primary := teststorage.NewAppendable()
		secondary := teststorage.NewAppendable()
		f := storage.NewFanout(nil, mockStorage{appV2: &appendableV2WithoutExemplars{AppendableV2: primary}}, mockStorage{appV2: secondary})

		exApp := f.AppenderV2(t.Context()).(storage.ExemplarAppenderV2)
		lbls := labels.FromStrings(model.MetricNameLabel, "metric1")
		ref, err := exApp.Append(0, lbls, -1, 0, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)

		_, err = exApp.AppendExemplars(ref, lbls, []exemplar.Exemplar{ex})
		require.ErrorContains(t, err, "does not implement ExemplarAppenderV2")
		require.NoError(t, exApp.Rollback())
	})
}

type appendableV2WithoutExemplars struct {
	storage.AppendableV2
}

type appenderV2WithoutExemplars struct {
	storage.AppenderV2
}

func (a *appendableV2WithoutExemplars) AppenderV2(ctx context.Context) storage.AppenderV2 {
	return &appenderV2WithoutExemplars{AppenderV2: a.AppendableV2.AppenderV2(ctx)}
}

// Recommended CLI invocation:
/*
	export bench=fanoutAppender && go test ./storage/... \
		-run '^$' -bench '^BenchmarkFanoutAppenderV2' \
		-benchtime 2s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkFanoutAppenderV2(b *testing.B) {
	ex := []exemplar.Exemplar{{Value: 1}}

	var series []labels.Labels
	for i := range 1000 {
		series = append(series, labels.FromStrings(model.MetricNameLabel, "metric1", "i", strconv.Itoa(i)))
	}
	for _, tt := range fanoutAppenderTestCases(nil) {
		// Turn our mock appender into ~noop for no allocs.
		tt.primary.SkipRecording(true)
		tt.secondary.SkipRecording(true)

		b.Run(tt.name, func(b *testing.B) {
			f := storage.NewFanout(nil, mockStorage{appV2: tt.primary}, mockStorage{appV2: tt.secondary})

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				app := f.AppenderV2(b.Context())
				for _, s := range series {
					// Purposefully skip errors as we want to benchmark error cases too (majority of the fanout logic).
					_, _ = app.Append(0, s, 0, 0, 1, nil, nil, storage.AOptions{
						Exemplars: ex,
					})
				}
				require.NoError(b, app.Rollback())
			}
		})
	}
}

type fanoutExemplarFailure struct{ backend string }

func (e *fanoutExemplarFailure) Error() string { return e.backend }

type partialFanoutAppendable struct {
	storage.Storage
	partial *storage.AppendPartialError
}

func (a partialFanoutAppendable) AppenderV2(context.Context) storage.AppenderV2 {
	return &partialFanoutAppender{partial: a.partial}
}

type partialFanoutAppender struct {
	storage.ExemplarAppenderV2
	partial *storage.AppendPartialError
}

func (a *partialFanoutAppender) Append(ref storage.SeriesRef, _ labels.Labels, _, _ int64, _ float64, _ *histogram.Histogram, _ *histogram.FloatHistogram, _ storage.AOptions) (storage.SeriesRef, error) {
	return ref, fmt.Errorf("backend: %w", a.partial)
}

func (a *partialFanoutAppender) AppendExemplars(ref storage.SeriesRef, _ labels.Labels, _ []exemplar.Exemplar) (storage.SeriesRef, error) {
	return ref, fmt.Errorf("backend: %w", a.partial)
}

func (*partialFanoutAppender) Rollback() error { return nil }

func testFanoutIndexedExemplarErrors(t *testing.T, inline bool) {
	t.Helper()
	first := &fanoutExemplarFailure{backend: "primary"}
	second := &fanoutExemplarFailure{backend: "secondary"}
	third := &fanoutExemplarFailure{backend: "third"}
	for _, tc := range []struct {
		name   string
		inputs [][]error
		want   [][]error
		hard   bool
	}{
		{name: "two overlapping", inputs: [][]error{{nil, first}, {nil, second}}, want: [][]error{nil, {first, second}}},
		{name: "two disjoint", inputs: [][]error{{first, nil}, {nil, second}}, want: [][]error{{first}, {second}}},
		{name: "three overlapping", inputs: [][]error{{first, nil}, {second, nil}, {third, nil}}, want: [][]error{{first, second, third}, nil}},
		{name: "three mixed", inputs: [][]error{{first, nil}, {nil, second}, {nil, third}}, want: [][]error{{first}, {second, third}}},
		{name: "all nil", inputs: [][]error{{nil, nil}, {nil, nil}}},
		{name: "malformed primary", inputs: [][]error{{first}, {nil, second}}, hard: true},
		{name: "malformed secondary", inputs: [][]error{{nil, nil}, {second}}, hard: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backends := make([]storage.Storage, len(tc.inputs))
			for i, input := range tc.inputs {
				backends[i] = partialFanoutAppendable{partial: &storage.AppendPartialError{ExemplarErrors: slices.Clone(input)}}
			}
			app := storage.NewFanout(nil, backends[0], backends[1:]...).AppenderV2(t.Context())
			defer func() { require.NoError(t, app.Rollback()) }()
			input := make([]exemplar.Exemplar, 2)
			var err error
			if inline {
				_, err = app.Append(1, labels.EmptyLabels(), 0, 1, 1, nil, nil, storage.AOptions{Exemplars: input})
			} else {
				_, err = app.(storage.ExemplarAppenderV2).AppendExemplars(1, labels.EmptyLabels(), input)
			}
			var partial *storage.AppendPartialError
			if tc.hard {
				require.Error(t, err)
				require.NotErrorAs(t, err, &partial)
				return
			}
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorAs(t, err, &partial)
			require.Len(t, partial.ExemplarErrors, len(input))
			failed := 0
			for i, causes := range tc.want {
				if len(causes) == 0 {
					require.NoError(t, partial.ExemplarErrors[i])
					continue
				}
				failed++
				for _, cause := range causes {
					require.ErrorIs(t, partial.ExemplarErrors[i], cause)
				}
				var backendCause *fanoutExemplarFailure
				require.ErrorAs(t, partial.ExemplarErrors[i], &backendCause)
			}
			require.Equal(t, failed, partial.FailedExemplarCount())
			partial.ExemplarErrors[0] = storage.ErrNotFound
			for i, backend := range backends {
				require.Equal(t, tc.inputs[i], backend.(partialFanoutAppendable).partial.ExemplarErrors)
			}
		})
	}
}
