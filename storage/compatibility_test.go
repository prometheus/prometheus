// Copyright 2025 The Prometheus Authors
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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
)

type mockAppender struct {
	t *testing.T

	// For asserting calls.
	gotLabels          []labels.Labels
	gotTs              []int64
	gotSamples         []float64
	gotHistograms      []*histogram.Histogram
	gotFloatHistograms []*histogram.FloatHistogram
	gotExemplars       []exemplar.Exemplar
	gotMetadata        []metadata.Metadata

	committed  bool
	rolledBack bool
	opts       *AppendOptions
}

func (m *mockAppender) Append(ref SeriesRef, l labels.Labels, t int64, v float64) (SeriesRef, error) {
	m.gotLabels = append(m.gotLabels, l)
	m.gotTs = append(m.gotTs, t)
	m.gotSamples = append(m.gotSamples, v)
	return ref, nil
}

func (m *mockAppender) AppendHistogram(ref SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (SeriesRef, error) {
	m.gotLabels = append(m.gotLabels, l)
	m.gotTs = append(m.gotTs, t)
	m.gotHistograms = append(m.gotHistograms, h)
	m.gotFloatHistograms = append(m.gotFloatHistograms, fh)
	return ref, nil
}

func (m *mockAppender) Commit() error {
	m.committed = true
	return nil
}

func (m *mockAppender) Rollback() error {
	m.rolledBack = true
	return nil
}

func (m *mockAppender) SetOptions(opts *AppendOptions) {
	m.opts = opts
}

func (m *mockAppender) AppendCTZeroSample(ref SeriesRef, l labels.Labels, _, ct int64) (SeriesRef, error) {
	return m.Append(ref, l, ct, 0) // Mimic the desired implementation.
}

var (
	zeroHistogram = &histogram.Histogram{
		// The CTZeroSample represents a counter reset by definition.
		CounterResetHint: histogram.CounterReset,
	}
	zeroFloatHistogram = &histogram.FloatHistogram{
		// The CTZeroSample represents a counter reset by definition.
		CounterResetHint: histogram.CounterReset,
	}
)

func (m *mockAppender) AppendHistogramCTZeroSample(ref SeriesRef, l labels.Labels, _, ct int64, _ *histogram.Histogram, _ *histogram.FloatHistogram) (SeriesRef, error) {
	return m.AppendHistogram(ref, l, ct, zeroHistogram, zeroFloatHistogram) // Mimic the desired implementation.
}

func (m *mockAppender) AppendExemplar(ref SeriesRef, l labels.Labels, e exemplar.Exemplar) (SeriesRef, error) {
	m.gotLabels = append(m.gotLabels, l)
	m.gotExemplars = append(m.gotExemplars, e)
	return ref, nil
}

func (m *mockAppender) UpdateMetadata(ref SeriesRef, l labels.Labels, md metadata.Metadata) (SeriesRef, error) {
	m.gotLabels = append(m.gotLabels, l)
	m.gotMetadata = append(m.gotMetadata, md)
	return ref, nil
}

func TestAsAppenderV2(t *testing.T) {
	testAsAppenderV2(t, func(appender Appender) AppenderV2 {
		return AsAppenderV2(nil, appender)
	})
}

func TestAsAppender(t *testing.T) {
	// To minimize the tests and mocks to maintain, we AsAppender between AsAppenderV2.
	testAsAppenderV2(t, func(appender Appender) AppenderV2 {
		return AsAppenderV2(nil,
			AsAppender(
				AsAppenderV2(nil, appender),
				appender.(ExemplarAppender),
				appender.(MetadataUpdater),
				appender.(combinedCTAppender),
			))
	})
}

func testAsAppenderV2(t *testing.T, asAppenderV2Fn func(Appender) AppenderV2) {
	t.Helper()

	ls := labels.FromStrings("a", "b")
	meta := Metadata{Metadata: metadata.Metadata{Type: "counter"}}
	es := []exemplar.Exemplar{{Value: 1}, {Value: 2}}
	h := &histogram.Histogram{Count: 1}
	fh := &histogram.FloatHistogram{Count: 2}
	t.Run("method=AppendSample", func(t *testing.T) {
		t.Run("basic", func(t *testing.T) {
			mockApp := &mockAppender{t: t}
			app := asAppenderV2Fn(mockApp)

			_, err := app.AppendSample(1, ls, Metadata{}, 0, 100, 1.23, nil)
			require.NoError(t, err)
			expectedMockState := mockAppender{
				t:          t,
				gotLabels:  []labels.Labels{ls},
				gotSamples: []float64{1.23},
				gotTs:      []int64{100},
			}
			require.Equal(t, expectedMockState, *mockApp)
		})
		t.Run("appendCTAsZero=true", func(t *testing.T) {
			mockApp := &mockAppender{t: t}
			app := asAppenderV2Fn(mockApp)
			app.SetOptions(&AppendV2Options{AppendCTAsZero: true})

			_, err := app.AppendSample(1, ls, Metadata{}, 0, 100, 1.23, nil)
			require.NoError(t, err)
			expectedMockState := mockAppender{
				t:          t,
				gotLabels:  []labels.Labels{ls},
				gotSamples: []float64{1.23},
				gotTs:      []int64{100},
				opts:       &AppendOptions{DiscardOutOfOrder: false},
			}
			require.Equal(t, expectedMockState, *mockApp)

			_, err = app.AppendSample(1, ls, Metadata{}, 101, 102, 3.45, nil)
			require.NoError(t, err)
			expectedMockState = mockAppender{
				t:          t,
				gotLabels:  []labels.Labels{ls, ls, ls},
				gotSamples: []float64{1.23, 0, 3.45},
				gotTs:      []int64{100, 101, 102},
				opts:       &AppendOptions{DiscardOutOfOrder: false},
			}
			require.Equal(t, expectedMockState, *mockApp)
		})
		t.Run("with extra", func(t *testing.T) {
			mockApp := &mockAppender{t: t}
			app := asAppenderV2Fn(mockApp)

			_, err := app.AppendSample(1, ls, meta, 0, 100, 1.23, es)
			require.NoError(t, err)
			expectedMockState := mockAppender{
				t:            t,
				gotLabels:    []labels.Labels{ls, ls, ls, ls}, // AppendSample, AppendExemplar, AppendExemplar, UpdateMetadata.
				gotSamples:   []float64{1.23},
				gotTs:        []int64{100},
				gotExemplars: es,
				gotMetadata:  []metadata.Metadata{meta.Metadata},
			}
			require.Equal(t, expectedMockState, *mockApp)
		})
	})
	t.Run("method=AppendHistogram", func(t *testing.T) {
		t.Run("basic", func(t *testing.T) {
			mockApp := &mockAppender{t: t}
			app := asAppenderV2Fn(mockApp)

			_, err := app.AppendHistogram(1, ls, Metadata{}, 0, 102, h, nil, nil)
			require.NoError(t, err)
			expectedMockState := mockAppender{
				t:                  t,
				gotLabels:          []labels.Labels{ls},
				gotHistograms:      []*histogram.Histogram{h},
				gotFloatHistograms: []*histogram.FloatHistogram{nil},
				gotTs:              []int64{102},
			}
			require.Equal(t, expectedMockState, *mockApp)
		})
		t.Run("appendCTAsZero=true", func(t *testing.T) {
			mockApp := &mockAppender{t: t}
			app := asAppenderV2Fn(mockApp)
			app.SetOptions(&AppendV2Options{AppendCTAsZero: true})

			_, err := app.AppendHistogram(1, ls, Metadata{}, 0, 102, h, nil, nil)
			require.NoError(t, err)
			expectedMockState := mockAppender{
				t:                  t,
				gotLabels:          []labels.Labels{ls},
				gotHistograms:      []*histogram.Histogram{h},
				gotFloatHistograms: []*histogram.FloatHistogram{nil},
				gotTs:              []int64{102},
				opts:               &AppendOptions{DiscardOutOfOrder: false},
			}
			require.Equal(t, expectedMockState, *mockApp)

			_, err = app.AppendHistogram(1, ls, Metadata{}, 103, 104, h, nil, nil)
			require.NoError(t, err)
			expectedMockState = mockAppender{
				t:                  t,
				gotLabels:          []labels.Labels{ls, ls, ls},
				gotHistograms:      []*histogram.Histogram{h, zeroHistogram, h},
				gotFloatHistograms: []*histogram.FloatHistogram{nil, zeroFloatHistogram, nil},
				gotTs:              []int64{102, 103, 104},
				opts:               &AppendOptions{DiscardOutOfOrder: false},
			}
			require.Equal(t, expectedMockState, *mockApp)
		})
		t.Run("with extra", func(t *testing.T) {
			mockApp := &mockAppender{t: t}
			app := asAppenderV2Fn(mockApp)

			_, err := app.AppendHistogram(1, ls, meta, 0, 102, nil, fh, es)
			require.NoError(t, err)
			expectedMockState := mockAppender{
				t:                  t,
				gotLabels:          []labels.Labels{ls, ls, ls, ls}, // AppendSample, AppendExemplar, AppendExemplar, UpdateMetadata.
				gotHistograms:      []*histogram.Histogram{nil},
				gotFloatHistograms: []*histogram.FloatHistogram{fh},
				gotTs:              []int64{102},
				gotExemplars:       es,
				gotMetadata:        []metadata.Metadata{meta.Metadata},
			}
			require.Equal(t, expectedMockState, *mockApp)
		})
	})

	t.Run("method=Commit/Rollback/SetOptions", func(t *testing.T) {
		mockApp := &mockAppender{t: t}
		app := asAppenderV2Fn(mockApp)

		require.NoError(t, app.Commit())
		require.True(t, mockApp.committed)

		require.NoError(t, app.Rollback())
		require.True(t, mockApp.rolledBack)

		opts := &AppendV2Options{DiscardOutOfOrder: true}
		app.SetOptions(opts)
		require.NotNil(t, mockApp.opts)
		require.True(t, mockApp.opts.DiscardOutOfOrder)
	})
}
