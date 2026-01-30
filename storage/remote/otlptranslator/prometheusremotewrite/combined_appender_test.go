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

package prometheusremotewrite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	st               int64
	v                float64
	es               []exemplar.Exemplar
}

type combinedHistogram struct {
	metricFamilyName string
	ls               labels.Labels
	meta             metadata.Metadata
	t                int64
	st               int64
	h                *histogram.Histogram
	es               []exemplar.Exemplar
}

func (m *mockCombinedAppender) AppendSample(ls labels.Labels, meta Metadata, st, t int64, v float64, es []exemplar.Exemplar) error {
	m.pendingSamples = append(m.pendingSamples, combinedSample{
		metricFamilyName: meta.MetricFamilyName,
		ls:               ls,
		meta:             meta.Metadata,
		t:                t,
		st:               st,
		v:                v,
		es:               es,
	})
	return nil
}

func (m *mockCombinedAppender) AppendHistogram(ls labels.Labels, meta Metadata, st, t int64, h *histogram.Histogram, es []exemplar.Exemplar) error {
	m.pendingHistograms = append(m.pendingHistograms, combinedHistogram{
		metricFamilyName: meta.MetricFamilyName,
		ls:               ls,
		meta:             meta.Metadata,
		t:                t,
		st:               st,
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

func requireEqual(t testing.TB, expected, actual any, msgAndArgs ...any) {
	testutil.RequireEqualWithOptions(t, expected, actual, []cmp.Option{cmp.AllowUnexported(combinedSample{}, combinedHistogram{})}, msgAndArgs...)
}

// TestCombinedAppenderOnTSDB runs some basic tests on a real TSDB to check
// that the combinedAppender works on a real TSDB.
func TestCombinedAppenderOnTSDB(t *testing.T) {
	t.Run("ingestSTZeroSample=false", func(t *testing.T) { testCombinedAppenderOnTSDB(t, false) })

	t.Run("ingestSTZeroSample=true", func(t *testing.T) { testCombinedAppenderOnTSDB(t, true) })
}

func testCombinedAppenderOnTSDB(t *testing.T, ingestSTZeroSample bool) {
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
		extraAppendFunc   func(*testing.T, CombinedAppender)
		expectedSamples   []sample
		expectedExemplars []exemplar.QueryResult
		expectedLogsForST []string
	}{
		"single float sample, zero ST": {
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
		"single float sample, very old ST": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, 1, now.UnixMilli(), 42.0, nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
			expectedLogsForST: []string{
				"Error when appending ST from OTLP",
				"out of bound",
			},
		},
		"single float sample, normal ST": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.Add(-2*time.Minute).UnixMilli(), now.UnixMilli(), 42.0, nil))
			},
			expectedSamples: []sample{
				{
					stZero: true,
					t:      now.Add(-2 * time.Minute).UnixMilli(),
				},
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
		},
		"single float sample, ST same time as sample": {
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
		"two float samples in different messages, ST same time as first sample": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.UnixMilli(), now.UnixMilli(), 42.0, nil))
			},
			extraAppendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.UnixMilli(), now.Add(time.Second).UnixMilli(), 43.0, nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
				{
					t: now.Add(time.Second).UnixMilli(),
					f: 43.0,
				},
			},
		},
		"single float sample, ST in the future of the sample": {
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
		"single histogram sample, zero ST": {
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
		"single histogram sample, very old ST": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, 1, now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
			expectedLogsForST: []string{
				"Error when appending ST from OTLP",
				"out of bound",
			},
		},
		"single histogram sample, normal ST": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), histogramMetadata, now.Add(-2*time.Minute).UnixMilli(), now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil))
			},
			expectedSamples: []sample{
				{
					stZero: true,
					t:      now.Add(-2 * time.Minute).UnixMilli(),
					h:      &histogram.Histogram{},
				},
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
		},
		"single histogram sample, ST same time as sample": {
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
		"two histogram samples in different messages, ST same time as first sample": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), floatMetadata, now.UnixMilli(), now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil))
			},
			extraAppendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendHistogram(seriesLabels.Copy(), floatMetadata, now.UnixMilli(), now.Add(time.Second).UnixMilli(), tsdbutil.GenerateTestHistogram(43), nil))
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
				{
					t: now.Add(time.Second).UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(43),
				},
			},
		},
		"single histogram sample, ST in the future of the sample": {
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
		"float samples with ST changing": {
			appendFunc: func(t *testing.T, app CombinedAppender) {
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.Add(-4*time.Second).UnixMilli(), now.Add(-3*time.Second).UnixMilli(), 42.0, nil))
				require.NoError(t, app.AppendSample(seriesLabels.Copy(), floatMetadata, now.Add(-1*time.Second).UnixMilli(), now.UnixMilli(), 62.0, nil))
			},
			expectedSamples: []sample{
				{
					stZero: true,
					t:      now.Add(-4 * time.Second).UnixMilli(),
				},
				{
					t: now.Add(-3 * time.Second).UnixMilli(),
					f: 42.0,
				},
				{
					stZero: true,
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
			var expectedLogs []string
			if ingestSTZeroSample {
				expectedLogs = append(expectedLogs, tc.expectedLogsForST...)
			}

			dir := t.TempDir()
			opts := tsdb.DefaultOptions()
			opts.EnableExemplarStorage = true
			opts.MaxExemplars = 100
			db, err := tsdb.Open(dir, promslog.NewNopLogger(), prometheus.NewRegistry(), opts, nil)
			require.NoError(t, err)

			t.Cleanup(func() { db.Close() })

			var output bytes.Buffer
			logger := promslog.New(&promslog.Config{Writer: &output})

			ctx := context.Background()
			reg := prometheus.NewRegistry()
			cappMetrics := NewCombinedAppenderMetrics(reg)
			app := db.Appender(ctx)
			capp := NewCombinedAppender(app, logger, ingestSTZeroSample, false, cappMetrics)
			tc.appendFunc(t, capp)
			require.NoError(t, app.Commit())

			if tc.extraAppendFunc != nil {
				app = db.Appender(ctx)
				capp = NewCombinedAppender(app, logger, ingestSTZeroSample, false, cappMetrics)
				tc.extraAppendFunc(t, capp)
				require.NoError(t, app.Commit())
			}

			if len(expectedLogs) > 0 {
				for _, expectedLog := range expectedLogs {
					require.Contains(t, output.String(), expectedLog)
				}
			} else {
				require.Empty(t, output.String(), "unexpected log output")
			}

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
				if !ingestSTZeroSample && sample.stZero {
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
	stZero bool

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

	t.Run("happy case with ST zero, reference is passed and reused", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, false, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 3, 4, 62.0, []exemplar.Exemplar{
			{
				Labels: labels.FromStrings("tracid", "122"),
				Value:  1337,
			},
		}))

		require.Len(t, app.records, 5)
		requireEqualOpAndRef(t, "AppendSTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "AppendSTZeroSample", ref, app.records[2])
		requireEqualOpAndRef(t, "Append", ref, app.records[3])
		requireEqualOpAndRef(t, "AppendExemplar", ref, app.records[4])
	})

	t.Run("error on second ST ingest doesn't update the reference", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, false, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))

		app.appendSTZeroSampleError = errors.New("test error")
		require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 3, 4, 62.0, nil))

		require.Len(t, app.records, 4)
		requireEqualOpAndRef(t, "AppendSTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "AppendSTZeroSample", ref, app.records[2])
		require.Zero(t, app.records[2].outRef, "the second AppendSTZeroSample returned 0")
		requireEqualOpAndRef(t, "Append", ref, app.records[3])
	})

	t.Run("metadata, exemplars are not updated if append failed", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, false, NewCombinedAppenderMetrics(prometheus.NewRegistry()))
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
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

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
		requireEqualOpAndRef(t, "AppendSTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
		requireEqualOpAndRef(t, "AppendSTZeroSample", ref, app.records[3])
		requireEqualOpAndRef(t, "Append", ref, app.records[4])
		require.Zero(t, app.records[4].outRef, "the second Append returned 0")
		requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[5])
		requireEqualOpAndRef(t, "AppendExemplar", ref, app.records[6])
	})

	t.Run("simulate conflict with existing series", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, false, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

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

		require.Len(t, app.records, 5)
		requireEqualOpAndRef(t, "AppendSTZeroSample", 0, app.records[0])
		ref := app.records[0].outRef
		require.NotZero(t, ref)
		requireEqualOpAndRef(t, "Append", ref, app.records[1])
		requireEqualOpAndRef(t, "AppendSTZeroSample", 0, app.records[2])
		newRef := app.records[2].outRef
		require.NotEqual(t, ref, newRef, "the second AppendSTZeroSample returned a different reference")
		requireEqualOpAndRef(t, "Append", newRef, app.records[3])
		requireEqualOpAndRef(t, "AppendExemplar", newRef, app.records[4])
	})

	t.Run("check that invoking AppendHistogram returns an error for nil histogram", func(t *testing.T) {
		app := &appenderRecorder{}
		capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, false, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

		ls := labels.FromStrings(
			model.MetricNameLabel, "test_bytes_total",
			"foo", "bar",
		)
		err := capp.AppendHistogram(ls, Metadata{}, 4, 2, nil, nil)
		require.Error(t, err)
	})

	for _, appendMetadata := range []bool{false, true} {
		t.Run(fmt.Sprintf("appendMetadata=%t", appendMetadata), func(t *testing.T) {
			app := &appenderRecorder{}
			capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, appendMetadata, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

			require.NoError(t, capp.AppendSample(seriesLabels.Copy(), floatMetadata, 1, 2, 42.0, nil))

			if appendMetadata {
				require.Len(t, app.records, 3)
				requireEqualOp(t, "AppendSTZeroSample", app.records[0])
				requireEqualOp(t, "Append", app.records[1])
				requireEqualOp(t, "UpdateMetadata", app.records[2])
			} else {
				require.Len(t, app.records, 2)
				requireEqualOp(t, "AppendSTZeroSample", app.records[0])
				requireEqualOp(t, "Append", app.records[1])
			}
		})
	}
}

// TestCombinedAppenderMetadataChanges verifies that UpdateMetadata is called
// when metadata fields change (help, unit, or type).
func TestCombinedAppenderMetadataChanges(t *testing.T) {
	seriesLabels := labels.FromStrings(
		model.MetricNameLabel, "test_metric",
		"foo", "bar",
	)

	baseMetadata := Metadata{
		Metadata: metadata.Metadata{
			Type: model.MetricTypeCounter,
			Unit: "bytes",
			Help: "original help",
		},
		MetricFamilyName: "test_metric",
	}

	tests := []struct {
		name           string
		modifyMetadata func(Metadata) Metadata
	}{
		{
			name: "help changes",
			modifyMetadata: func(m Metadata) Metadata {
				m.Help = "new help text"
				return m
			},
		},
		{
			name: "unit changes",
			modifyMetadata: func(m Metadata) Metadata {
				m.Unit = "seconds"
				return m
			},
		},
		{
			name: "type changes",
			modifyMetadata: func(m Metadata) Metadata {
				m.Type = model.MetricTypeGauge
				return m
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &appenderRecorder{}
			capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, true, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

			newMetadata := tt.modifyMetadata(baseMetadata)

			require.NoError(t, capp.AppendSample(seriesLabels.Copy(), baseMetadata, 1, 2, 42.0, nil))
			require.NoError(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 4, 62.0, nil))
			require.NoError(t, capp.AppendSample(seriesLabels.Copy(), newMetadata, 3, 5, 162.0, nil))

			// Verify expected operations.
			require.Len(t, app.records, 7)
			requireEqualOpAndRef(t, "AppendSTZeroSample", 0, app.records[0])
			ref := app.records[0].outRef
			require.NotZero(t, ref)
			requireEqualOpAndRef(t, "Append", ref, app.records[1])
			requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[2])
			requireEqualOpAndRef(t, "AppendSTZeroSample", ref, app.records[3])
			requireEqualOpAndRef(t, "Append", ref, app.records[4])
			requireEqualOpAndRef(t, "UpdateMetadata", ref, app.records[5])
			requireEqualOpAndRef(t, "Append", ref, app.records[6])
		})
	}
}

func requireEqualOp(t *testing.T, expectedOp string, actual appenderRecord) {
	t.Helper()
	require.Equal(t, expectedOp, actual.op)
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
	appendSTZeroSampleError          error
	appendHistogramError             error
	appendHistogramSTZeroSampleError error
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

func (a *appenderRecorder) AppendSTZeroSample(ref storage.SeriesRef, ls labels.Labels, _, _ int64) (storage.SeriesRef, error) {
	a.records = append(a.records, appenderRecord{op: "AppendSTZeroSample", ref: ref, ls: ls})
	if a.appendSTZeroSampleError != nil {
		return 0, a.appendSTZeroSampleError
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

func (a *appenderRecorder) AppendHistogramSTZeroSample(ref storage.SeriesRef, ls labels.Labels, _, _ int64, _ *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	a.records = append(a.records, appenderRecord{op: "AppendHistogramSTZeroSample", ref: ref, ls: ls})
	if a.appendHistogramSTZeroSampleError != nil {
		return 0, a.appendHistogramSTZeroSampleError
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

func TestMetadataChangedLogic(t *testing.T) {
	seriesLabels := labels.FromStrings(model.MetricNameLabel, "test_metric", "foo", "bar")
	baseMetadata := Metadata{
		Metadata:         metadata.Metadata{Type: model.MetricTypeCounter, Unit: "bytes", Help: "original"},
		MetricFamilyName: "test_metric",
	}

	tests := []struct {
		name           string
		appendMetadata bool
		modifyMetadata func(Metadata) Metadata
		expectWALCall  bool
		verifyCached   func(*testing.T, metadata.Metadata)
	}{
		{
			name:           "appendMetadata=false, no change",
			appendMetadata: false,
			modifyMetadata: func(m Metadata) Metadata { return m },
			expectWALCall:  false,
			verifyCached:   func(t *testing.T, m metadata.Metadata) { require.Equal(t, "original", m.Help) },
		},
		{
			name:           "appendMetadata=false, help changes - cache updated, no WAL",
			appendMetadata: false,
			modifyMetadata: func(m Metadata) Metadata { m.Help = "changed"; return m },
			expectWALCall:  false,
			verifyCached:   func(t *testing.T, m metadata.Metadata) { require.Equal(t, "changed", m.Help) },
		},
		{
			name:           "appendMetadata=true, help changes - cache and WAL updated",
			appendMetadata: true,
			modifyMetadata: func(m Metadata) Metadata { m.Help = "changed"; return m },
			expectWALCall:  true,
			verifyCached:   func(t *testing.T, m metadata.Metadata) { require.Equal(t, "changed", m.Help) },
		},
		{
			name:           "appendMetadata=true, unit changes",
			appendMetadata: true,
			modifyMetadata: func(m Metadata) Metadata { m.Unit = "seconds"; return m },
			expectWALCall:  true,
			verifyCached:   func(t *testing.T, m metadata.Metadata) { require.Equal(t, "seconds", m.Unit) },
		},
		{
			name:           "appendMetadata=true, type changes",
			appendMetadata: true,
			modifyMetadata: func(m Metadata) Metadata { m.Type = model.MetricTypeGauge; return m },
			expectWALCall:  true,
			verifyCached:   func(t *testing.T, m metadata.Metadata) { require.Equal(t, model.MetricTypeGauge, m.Type) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &appenderRecorder{}
			capp := NewCombinedAppender(app, promslog.NewNopLogger(), true, tt.appendMetadata, NewCombinedAppenderMetrics(prometheus.NewRegistry()))

			require.NoError(t, capp.AppendSample(seriesLabels.Copy(), baseMetadata, 1, 2, 42.0, nil))

			modifiedMetadata := tt.modifyMetadata(baseMetadata)
			app.records = nil
			require.NoError(t, capp.AppendSample(seriesLabels.Copy(), modifiedMetadata, 1, 3, 43.0, nil))

			hash := seriesLabels.Hash()
			cached, exists := capp.(*combinedAppender).refs[hash]
			require.True(t, exists)
			tt.verifyCached(t, cached.meta)

			updateMetadataCalled := false
			for _, record := range app.records {
				if record.op == "UpdateMetadata" {
					updateMetadataCalled = true
					break
				}
			}
			require.Equal(t, tt.expectWALCall, updateMetadataCalled)
		})
	}
}
