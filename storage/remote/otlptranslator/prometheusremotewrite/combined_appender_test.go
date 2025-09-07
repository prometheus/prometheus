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

package prometheusremotewrite

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/testutil"
)

type mockCombinedAppender struct {
	pendingSamples    []combinedSample
	pendingHistograms []combinedHistogram

	samples    []combinedSample
	histograms []combinedHistogram
}

type combinedSample struct {
	metricFamilyName string
	ls               labels.Labels
	meta             metadata.Metadata
	t                int64
	ct               int64
	v                float64
	es               []exemplar.Exemplar
}

type combinedHistogram struct {
	metricFamilyName string
	ls               labels.Labels
	meta             metadata.Metadata
	t                int64
	ct               int64
	h                *histogram.Histogram
	es               []exemplar.Exemplar
}

func (m *mockCombinedAppender) AppendSample(ls labels.Labels, meta Metadata, ct, t int64, v float64, es []exemplar.Exemplar) error {
	m.pendingSamples = append(m.pendingSamples, combinedSample{
		metricFamilyName: meta.MetricFamilyName,
		ls:               ls,
		meta:             meta.Metadata,
		t:                t,
		ct:               ct,
		v:                v,
		es:               es,
	})
	return nil
}

func (m *mockCombinedAppender) AppendHistogram(ls labels.Labels, meta Metadata, ct, t int64, h *histogram.Histogram, es []exemplar.Exemplar) error {
	m.pendingHistograms = append(m.pendingHistograms, combinedHistogram{
		metricFamilyName: meta.MetricFamilyName,
		ls:               ls,
		meta:             meta.Metadata,
		t:                t,
		ct:               ct,
		h:                h,
		es:               es,
	})
	return nil
}

func (m *mockCombinedAppender) Commit() error {
	m.samples = append(m.samples, m.pendingSamples...)
	m.pendingSamples = m.pendingSamples[:0]
	m.histograms = append(m.histograms, m.pendingHistograms...)
	m.pendingHistograms = m.pendingHistograms[:0]
	return nil
}

func requireEqual(t testing.TB, expected, actual interface{}, msgAndArgs ...interface{}) {
	testutil.RequireEqualWithOptions(t, expected, actual, []cmp.Option{cmp.AllowUnexported(combinedSample{}, combinedHistogram{})}, msgAndArgs...)
}

// TestCombinedAppenderOnTSDB runs some basic tests on a real TSDB to check
// that the combinedAppender works on a real TSDB.
func TestCombinedAppenderOnTSDB(t *testing.T) {
	t.Run("ingestCTZeroSample=false", func(t *testing.T) { testCombinedAppenderOnTSDB(t, false) })

	t.Run("ingestCTZeroSample=true", func(t *testing.T) { testCombinedAppenderOnTSDB(t, true) })
}

func testCombinedAppenderOnTSDB(t *testing.T, ingestCTZeroSample bool) {
	t.Helper()

	now := time.Now()

	testExemplars := []exemplar.Exemplar{
		{
			Labels: labels.FromStrings("tracid", "122"),
			Value:  1337,
		},
		{
			Labels: labels.FromStrings("tracid", "132"),
			Value:  7777,
		},
	}
	expectedExemplars := []exemplar.QueryResult{
		{
			SeriesLabels: labels.FromStrings(
				model.MetricNameLabel, "test_bytes_total",
				"foo", "bar",
			),
			Exemplars: testExemplars,
		},
	}

	seriesLabels := labels.FromStrings(
		model.MetricNameLabel, "test_bytes_total",
		"foo", "bar",
	)
	floatMetadata := Metadata{
		Metadata: metadata.Metadata{
			Type: model.MetricTypeCounter,
			Unit: "bytes",
			Help: "some help",
		},
		MetricFamilyName: "test_bytes_total",
	}

	histogramMetadata := Metadata{
		Metadata: metadata.Metadata{
			Type: model.MetricTypeHistogram,
			Unit: "bytes",
			Help: "some help",
		},
		MetricFamilyName: "test_bytes",
	}

	testCases := map[string]struct {
		appendFunc        func(*testing.T, CombinedAppender)
		expectedSamples   []sample
		expectedExemplars []exemplar.QueryResult
	}{
		"single float sample, zero CT": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, 0, now.UnixMilli(), 42.0, testExemplars))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
			expectedExemplars: expectedExemplars,
		},
		"single float sample, very old CT": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, 1, now.UnixMilli(), 42.0, nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
		},
		"single float sample, normal CT": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.Add(-2*time.Minute).UnixMilli(), now.UnixMilli(), 42.0, nil))
			},
			expectedSamples: []sample{
				{
					ctZero: true,
					t:      now.Add(-2 * time.Minute).UnixMilli(),
				},
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
		},
		"single float sample, CT same time as sample": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.UnixMilli(), now.UnixMilli(), 42.0, nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
		},
		"single float sample, CT in the future of the sample": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.Add(time.Minute).UnixMilli(), now.UnixMilli(), 42.0, nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
		},
		"single histogram sample, zero CT": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, 0, now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), testExemplars))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
			expectedExemplars: expectedExemplars,
		},
		"single histogram sample, very old CT": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, 1, now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
		},
		"single histogram sample, normal CT": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, now.Add(-2*time.Minute).UnixMilli(), now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil))
			},
			expectedSamples: []sample{
				{
					ctZero: true,
					t:      now.Add(-2 * time.Minute).UnixMilli(),
					h:      &histogram.Histogram{},
				},
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
		},
		"single histogram sample, CT same time as sample": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, now.UnixMilli(), now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
		},
		"single histogram sample, CT in the future of the sample": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, now.Add(time.Minute).UnixMilli(), now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
		},
		"multiple float samples": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, 0, now.UnixMilli(), 42.0, nil))
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, 0, now.Add(15*time.Second).UnixMilli(), 62.0, nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
				{
					t: now.Add(15 * time.Second).UnixMilli(),
					f: 62.0,
				},
			},
		},
		"multiple histogram samples": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, 0, now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil))
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, 0, now.Add(15*time.Second).UnixMilli(), tsdbutil.GenerateTestHistogram(62), nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
				{
					t: now.Add(15 * time.Second).UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(62),
				},
			},
		},
		"float samples with CT changing": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.Add(-4*time.Second).UnixMilli(), now.Add(-3*time.Second).UnixMilli(), 42.0, nil))
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.Add(-1*time.Second).UnixMilli(), now.UnixMilli(), 62.0, nil))
			},
			expectedSamples: []sample{
				{
					ctZero: true,
					t:      now.Add(-4 * time.Second).UnixMilli(),
				},
				{
					t: now.Add(-3 * time.Second).UnixMilli(),
					f: 42.0,
				},
				{
					ctZero: true,
					t:      now.Add(-1 * time.Second).UnixMilli(),
				},
				{
					t: now.UnixMilli(),
					f: 62.0,
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			opts := tsdb.DefaultOptions()
			opts.EnableExemplarStorage = true
			opts.MaxExemplars = 100
			opts.EnableNativeHistograms = true
			db, err := tsdb.Open(dir, promslog.NewNopLogger(), prometheus.NewRegistry(), opts, nil)
			require.NoError(t, err)

			t.Cleanup(func() { db.Close() })

			ctx := context.Background()
			reg := prometheus.NewRegistry()
			app := db.Appender(ctx)
			capp := NewCombinedAppender(app, promslog.NewNopLogger(), ingestCTZeroSample, NewCombinedAppenderMetrics(reg))

			tc.appendFunc(t, capp)

			require.NoError(t, app.Commit())

			q, err := db.Querier(int64(math.MinInt64), int64(math.MaxInt64))
			require.NoError(t, err)

			ss := q.Select(ctx, false, &storage.SelectHints{
				Start: int64(math.MinInt64),
				End:   int64(math.MaxInt64),
			}, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_bytes_total"))

			require.NoError(t, ss.Err())

			require.True(t, ss.Next())
			series := ss.At()
			it := series.Iterator(nil)
			for i, sample := range tc.expectedSamples {
				if !ingestCTZeroSample && sample.ctZero {
					continue
				}
				if sample.h == nil {
					require.Equal(t, chunkenc.ValFloat, it.Next())
					ts, v := it.At()
					require.Equal(t, sample.t, ts, "sample ts %d", i)
					require.Equal(t, sample.f, v, "sample v %d", i)
				} else {
					require.Equal(t, chunkenc.ValHistogram, it.Next())
					ts, h := it.AtHistogram(nil)
					require.Equal(t, sample.t, ts, "sample ts %d", i)
					require.Equal(t, sample.h.Count, h.Count, "sample v %d", i)
				}
			}
			require.False(t, ss.Next())

			eq, err := db.ExemplarQuerier(ctx)
			require.NoError(t, err)
			exResult, err := eq.Select(int64(math.MinInt64), int64(math.MaxInt64), []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_bytes_total")})
			require.NoError(t, err)
			if tc.expectedExemplars == nil {
				tc.expectedExemplars = []exemplar.QueryResult{}
			}
			require.Equal(t, tc.expectedExemplars, exResult)
		})
	}
}

type sample struct {
	ctZero bool

	t int64
	f float64
	h *histogram.Histogram
}

// TestCombinedAppenderSeriesRefs checks that the combined appender
// correctly uses and updates the series references in the internal map.
func TestCombinedAppenderSeriesRefs(t *testing.T) {
	seriesLabels := labels.FromStrings(
		model.MetricNameLabel, "test_bytes_total",
		"foo", "bar",
	)

	floatMetadata := Metadata{
		Metadata: metadata.Metadata{
			Type: model.MetricTypeCounter,
			Unit: "bytes",
			Help: "some help",
		},
		MetricFamilyName: "test_bytes_total",
	}

	t.Run("happy case with CT zero, reference is passed and reused", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 3, 4, 62.0, []exemplar.Exemplar{
			{
				Labels: labels.FromStrings("tracid", "122"),
				Value:  1337,
			},
		}))

		require.Len(t, app.records, 6)
		requireEqualOpAndRef(t, "AppendCTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
		requireEqualOpAndRef(t, "AppendCTZeroSample", ref, app.records[3])
		requireEqualOpAndRef(t, "Append", ref, app.records[4])
		requireEqualOpAndRef(t, "AppendExemplar", ref, app.records[5])
	})

	t.Run("error on second CT ingest doesn't update the reference", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))

		app.appendCTZeroSampleError = errors.New("test error")
		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 3, 4, 62.0, nil))

		require.Len(t, app.records, 5)
		requireEqualOpAndRef(t, "AppendCTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
		requireEqualOpAndRef(t, "AppendCTZeroSample", ref, app.records[3])
		require.Zero(t, app.records[3].outRef, "the second AppendCTZeroSample returned 0")
		requireEqualOpAndRef(t, "Append", ref, app.records[4])
	})

	t.Run("updateMetadata called when meta help changes", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		newMetadata := floatMetadata
		newMetadata.Help = "some other help"

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))
		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 4, 62.0, nil))
		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 5, 162.0, nil))

		require.Len(t, app.records, 7)
		requireEqualOpAndRef(t, "AppendCTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
		requireEqualOpAndRef(t, "AppendCTZeroSample", ref, app.records[3])
		requireEqualOpAndRef(t, "Append", ref, app.records[4])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[5])
		requireEqualOpAndRef(t, "Append", ref, app.records[6])
	})

	t.Run("updateMetadata called when meta unit changes", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		newMetadata := floatMetadata
		newMetadata.Unit = "seconds"

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))
		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 4, 62.0, nil))
		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 5, 162.0, nil))

		require.Len(t, app.records, 7)
		requireEqualOpAndRef(t, "AppendCTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
		requireEqualOpAndRef(t, "AppendCTZeroSample", ref, app.records[3])
		requireEqualOpAndRef(t, "Append", ref, app.records[4])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[5])
		requireEqualOpAndRef(t, "Append", ref, app.records[6])
	})

	t.Run("updateMetadata called when meta type changes", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		newMetadata := floatMetadata
		newMetadata.Type = model.MetricTypeGauge

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))
		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 4, 62.0, nil))
		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 5, 162.0, nil))

		require.Len(t, app.records, 7)
		requireEqualOpAndRef(t, "AppendCTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
		requireEqualOpAndRef(t, "AppendCTZeroSample", ref, app.records[3])
		requireEqualOpAndRef(t, "Append", ref, app.records[4])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[5])
		requireEqualOpAndRef(t, "Append", ref, app.records[6])
	})

	t.Run("metadata, exemplars are not updated if append failed", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))
		app.appendError = errors.New("test error")
		require.Error(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 0, 1, 42.0, []exemplar.Exemplar{
			{
				Labels: labels.FromStrings("tracid", "122"),
				Value:  1337,
			},
		}))

		require.Len(t, app.records, 1)
		require.Equal(t, appenderRecord{
			op: "Append",
			ls: labels.FromStrings(model.MetricNameLabel, "test_bytes_total", "foo", "bar"),
		}, app.records[0])
	})

	t.Run("metadata, exemplars are updated if append failed but reference is valid", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		newMetadata := floatMetadata
		newMetadata.Help = "some other help"

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))
		app.appendError = errors.New("test error")
		require.Error(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 4, 62.0, []exemplar.Exemplar{
			{
				Labels: labels.FromStrings("tracid", "122"),
				Value:  1337,
			},
		}))

		require.Len(t, app.records, 7)
		requireEqualOpAndRef(t, "AppendCTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
		requireEqualOpAndRef(t, "AppendCTZeroSample", ref, app.records[3])
		requireEqualOpAndRef(t, "Append", ref, app.records[4])
		require.Zero(t, app.records[4].outRef, "the second Append returned 0")
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[5])
		requireEqualOpAndRef(t, "AppendExemplar", ref, app.records[6])
	})

	t.Run("simulate conflict with existing series", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		ls := labels.FromStrings(
			model.MetricNameLabel, "test_bytes_total",
			"foo", "bar",
		)

		require.NoError(t, capp.AppendSample(ls, floatMetadata, 1, 2, 42.0, nil))

		hash := ls.Hash()
		cappImpl := capp.(*combinedAppender)
		series := cappImpl.refs[hash]
		series.ls = labels.FromStrings(
			model.MetricNameLabel, "test_bytes_total",
			"foo", "club",
		)
		// The hash and ref remain the same, but we altered the labels.
		// This simulates a conflict with an existing series.
		cappImpl.refs[hash] = series

		require.NoError(t, capp.AppendSample(ls, floatMetadata, 3, 4, 62.0, []exemplar.Exemplar{
			{
				Labels: labels.FromStrings("tracid", "122"),
				Value:  1337,
			},
		}))

		require.Len(t, app.records, 7)
		requireEqualOpAndRef(t, "AppendCTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
		requireEqualOpAndRef(t, "AppendCTZeroSample", 0, app.records[3])
		newRef := app.records[3].outRef
		require.NotEqual(t, ref, newRef, "the second AppendCTZeroSample returned a different reference")
		requireEqualOpAndRef(t, "Append", newRef, app.records[4])
		requireEqualOpAndRef(t, "UpdateMetadata", newRef, app.records[5])
		requireEqualOpAndRef(t, "AppendExemplar", newRef, app.records[6])
	})

	t.Run("check that invoking AppendHistogram returns an error for nil histogram", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		ls := labels.FromStrings(
			model.MetricNameLabel, "test_bytes_total",
			"foo", "bar",
		)
		err := capp.AppendHistogram(ls, Metadata{}, 4, 2, nil, nil)
		require.Error(t, err)
	})
}

func requireEqualOpAndRef(t *testing.T, expectedOp string, expectedRef storage.SeriesRef, actual appenderRecord) {
	t.Helper()
	require.Equal(t, expectedOp, actual.op)
	require.Equal(t, expectedRef, actual.ref)
}

type appenderRecord struct {
	op     string
	ref    storage.SeriesRef
	outRef storage.SeriesRef
	ls     labels.Labels
}

type appenderRecorder struct {
	refcount uint64
	records  []appenderRecord

	appendError                      error
	appendCTZeroSampleError          error
	appendHistogramError             error
	appendHistogramCTZeroSampleError error
	updateMetadataError              error
	appendExemplarError              error
}

var _ storage.Appender = &appenderRecorder{}

func (a *appenderRecorder) setOutRef(ref storage.SeriesRef) {
	if len(a.records) == 0 {
		return
	}
	a.records[len(a.records)-1].outRef = ref
}

func (a *appenderRecorder) newRef() storage.SeriesRef {
	a.refcount++
	return storage.SeriesRef(a.refcount)
}

func (a *appenderRecorder) Append(ref storage.SeriesRef, ls labels.Labels, _ int64, _ float64) (storage.SeriesRef, error) {
	a.records = append(a.records, appenderRecord{op: "Append", ref: ref, ls: ls})
	if a.appendError != nil {
		return 0, a.appendError
	}
	if ref == 0 {
		ref = a.newRef()
	}
	a.setOutRef(ref)
	return ref, nil
}

func (a *appenderRecorder) AppendCTZeroSample(ref storage.SeriesRef, ls labels.Labels, _, _ int64) (storage.SeriesRef, error) {
	a.records = append(a.records, appenderRecord{op: "AppendCTZeroSample", ref: ref, ls: ls})
	if a.appendCTZeroSampleError != nil {
		return 0, a.appendCTZeroSampleError
	}
	if ref == 0 {
		ref = a.newRef()
	}
	a.setOutRef(ref)
	return ref, nil
}

func (a *appenderRecorder) AppendHistogram(ref storage.SeriesRef, ls labels.Labels, _ int64, _ *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	a.records = append(a.records, appenderRecord{op: "AppendHistogram", ref: ref, ls: ls})
	if a.appendHistogramError != nil {
		return 0, a.appendHistogramError
	}
	if ref == 0 {
		ref = a.newRef()
	}
	a.setOutRef(ref)
	return ref, nil
}

func (a *appenderRecorder) AppendHistogramCTZeroSample(ref storage.SeriesRef, ls labels.Labels, _, _ int64, _ *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	a.records = append(a.records, appenderRecord{op: "AppendHistogramCTZeroSample", ref: ref, ls: ls})
	if a.appendHistogramCTZeroSampleError != nil {
		return 0, a.appendHistogramCTZeroSampleError
	}
	if ref == 0 {
		ref = a.newRef()
	}
	a.setOutRef(ref)
	return ref, nil
}

func (a *appenderRecorder) UpdateMetadata(ref storage.SeriesRef, ls labels.Labels, _ metadata.Metadata) (storage.SeriesRef, error) {
	a.records = append(a.records, appenderRecord{op: "UpdateMetadata", ref: ref, ls: ls})
	if a.updateMetadataError != nil {
		return 0, a.updateMetadataError
	}
	a.setOutRef(ref)
	return ref, nil
}

func (a *appenderRecorder) AppendExemplar(ref storage.SeriesRef, ls labels.Labels, _ exemplar.Exemplar) (storage.SeriesRef, error) {
	a.records = append(a.records, appenderRecord{op: "AppendExemplar", ref: ref, ls: ls})
	if a.appendExemplarError != nil {
		return 0, a.appendExemplarError
	}
	a.setOutRef(ref)
	return ref, nil
}

func (a *appenderRecorder) Commit() error {
	a.records = append(a.records, appenderRecord{op: "Commit"})
	return nil
}

func (a *appenderRecorder) Rollback() error {
	a.records = append(a.records, appenderRecord{op: "Rollback"})
	return nil
}

func (*appenderRecorder) SetOptions(_ *storage.AppendOptions) {
	panic("not implemented")
}
