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
	modelLabels "github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite/labels"
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

func (m *mockCombinedAppender) AppendSample(metricFamilyName string, ls labels.Labels, meta metadata.Metadata, t, ct int64, v float64, es []exemplar.Exemplar) error {
	m.pendingSamples = append(m.pendingSamples, combinedSample{
		metricFamilyName: metricFamilyName,
		ls:               ls,
		meta:             meta,
		t:                t,
		ct:               ct,
		v:                v,
		es:               es,
	})
	return nil
}

func (m *mockCombinedAppender) AppendHistogram(metricFamilyName string, ls labels.Labels, meta metadata.Metadata, t, ct int64, h *histogram.Histogram, es []exemplar.Exemplar) error {
	m.pendingHistograms = append(m.pendingHistograms, combinedHistogram{
		metricFamilyName: metricFamilyName,
		ls:               ls,
		meta:             meta,
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
			Labels: modelLabels.FromStrings("tracid", "122"),
			Value:  1337,
		},
		{
			Labels: modelLabels.FromStrings("tracid", "132"),
			Value:  7777,
		},
	}
	expectedExemplars := []exemplar.QueryResult{
		{
			SeriesLabels: modelLabels.FromStrings(
				model.MetricNameLabel, "test_bytes_total",
				"foo", "bar",
			),
			Exemplars: testExemplars,
		},
	}

	testCases := map[string]struct {
		appendFunc        func(CombinedAppender) error
		expectedSamples   []sample
		expectedExemplars []exemplar.QueryResult
	}{
		"single float sample, zero CT": {
			appendFunc: func(app CombinedAppender) error {
				return app.AppendSample("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), 0, 42.0, testExemplars)
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
			appendFunc: func(app CombinedAppender) error {
				return app.AppendSample("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), 1, 42.0, nil)
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
		},
		"single float sample, normal CT": {
			appendFunc: func(app CombinedAppender) error {
				return app.AppendSample("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), now.Add(-2*time.Minute).UnixMilli(), 42.0, nil)
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
		"single float sample, CT name time as sample": {
			appendFunc: func(app CombinedAppender) error {
				return app.AppendSample("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), now.UnixMilli(), 42.0, nil)
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
		},
		"single float sample, CT in the future of the sample": {
			appendFunc: func(app CombinedAppender) error {
				return app.AppendSample("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), now.Add(time.Minute).UnixMilli(), 42.0, nil)
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					f: 42.0,
				},
			},
		},
		"single histogram sample, zero CT": {
			appendFunc: func(app CombinedAppender) error {
				return app.AppendHistogram("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), 0, tsdbutil.GenerateTestHistogram(42), testExemplars)
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
			appendFunc: func(app CombinedAppender) error {
				return app.AppendHistogram("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), 1, tsdbutil.GenerateTestHistogram(42), nil)
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
		},
		"single histogram sample, normal CT": {
			appendFunc: func(app CombinedAppender) error {
				return app.AppendHistogram("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), now.Add(-2*time.Minute).UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil)
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
		"single histogram sample, CT name time as sample": {
			appendFunc: func(app CombinedAppender) error {
				return app.AppendHistogram("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), now.UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil)
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
		},
		"single histogram sample, CT in the future of the sample": {
			appendFunc: func(app CombinedAppender) error {
				return app.AppendHistogram("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), now.Add(time.Minute).UnixMilli(), tsdbutil.GenerateTestHistogram(42), nil)
			},
			expectedSamples: []sample{
				{
					t: now.UnixMilli(),
					h: tsdbutil.GenerateTestHistogram(42),
				},
			},
		},
		"multiple float samples": {
			appendFunc: func(app CombinedAppender) error {
				err := app.AppendSample("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), 0, 42.0, nil)
				if err != nil {
					return err
				}
				return app.AppendSample("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.Add(15*time.Second).UnixMilli(), 0, 62.0, nil)
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
			appendFunc: func(app CombinedAppender) error {
				err := app.AppendHistogram("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.UnixMilli(), 0, tsdbutil.GenerateTestHistogram(42), nil)
				if err != nil {
					return err
				}
				return app.AppendHistogram("test_bytes_total", labels.FromStrings(
					model.MetricNameLabel, "test_bytes_total",
					"foo", "bar",
				), metadata.Metadata{
					Type: model.MetricTypeCounter,
					Unit: "bytes",
					Help: "some help",
				}, now.Add(15*time.Second).UnixMilli(), 0, tsdbutil.GenerateTestHistogram(62), nil)
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
			capp := NewCombinedAppender(db.Appender(ctx), promslog.NewNopLogger(), reg, ingestCTZeroSample, NewCombinedAppenderMetrics(reg))

			require.NoError(t, tc.appendFunc(capp))

			require.NoError(t, capp.Commit())

			q, err := db.Querier(int64(math.MinInt64), int64(math.MaxInt64))
			require.NoError(t, err)

			ss := q.Select(ctx, false, &storage.SelectHints{
				Start: int64(math.MinInt64),
				End:   int64(math.MaxInt64),
			}, modelLabels.MustNewMatcher(modelLabels.MatchEqual, model.MetricNameLabel, "test_bytes_total"))

			require.NoError(t, ss.Err())

			require.True(t, ss.Next())
			series := ss.At()
			it := series.Iterator(nil)
			for _, sample := range tc.expectedSamples {
				if !ingestCTZeroSample && sample.ctZero {
					continue
				}
				if sample.h == nil {
					require.Equal(t, chunkenc.ValFloat, it.Next())
					ts, v := it.At()
					require.Equal(t, sample.t, ts)
					require.Equal(t, sample.f, v)
				} else {
					require.Equal(t, chunkenc.ValHistogram, it.Next())
					ts, h := it.AtHistogram(nil)
					require.Equal(t, sample.t, ts)
					require.Equal(t, sample.h.Count, h.Count)
				}
			}
			require.False(t, ss.Next())

			eq, err := db.ExemplarQuerier(ctx)
			require.NoError(t, err)
			exResult, err := eq.Select(int64(math.MinInt64), int64(math.MaxInt64), []*modelLabels.Matcher{modelLabels.MustNewMatcher(modelLabels.MatchEqual, model.MetricNameLabel, "test_bytes_total")})
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
