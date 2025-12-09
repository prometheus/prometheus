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

package semconv

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func testSchemaURL(version string) string {
	// return "https://bwplotka.dev/semconv/" + version
	return "./testdata/" + version
}

var (
	testdataElementsSeriesOld = labels.FromStrings(
		"__name__", "my_app_custom_elements_total",
		"__schema_url__", testSchemaURL("1.0.0"),
		"__type__", "counter",
		"integer", "1",
		"category", "first",
		"fraction", "1.243",
		"test", "old",
	)
	testdataElementsSeriesNew = labels.FromStrings(
		"__name__", "my_app_custom_changed_elements_total",
		"__schema_url__", testSchemaURL("1.1.0"),
		"__type__", "counter",
		"number", "1",
		"class", "FIRST",
		"fraction", "1.243",
		"test", "new",
	)

	testdataLatencySeriesOld = labels.FromStrings(
		"__name__", "my_app_latency_milliseconds",
		"__schema_url__", testSchemaURL("1.0.0"),
		"__type__", "histogram",
		"__unit__", "milliseconds",
		"code", "200",
		"test", "old",
	)
	testdataLatencySeriesNew = labels.FromStrings(
		"__name__", "my_app_latency_seconds",
		"__schema_url__", testSchemaURL("1.1.0"),
		"__type__", "histogram",
		"__unit__", "seconds",
		"code", "200",
		"test", "new",
	)
)

type appendSeries struct {
	series  labels.Labels
	samples []chunks.Sample
}

func openTestDB(t testing.TB, opts *tsdb.Options, dataToAppend []appendSeries) (db *tsdb.DB) {
	t.Helper()

	tmpdir := t.TempDir()
	if opts == nil {
		opts = tsdb.DefaultOptions()
	}

	db, err := tsdb.Open(tmpdir, nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close
	})

	// Append test data.
	ctx := context.Background()
	app := db.Appender(ctx)
	for _, a := range dataToAppend {
		for _, s := range a.samples {
			if s.H() != nil || s.FH() != nil {
				_, err = app.AppendHistogram(0, a.series, s.T(), s.H(), nil)
				require.NoError(t, err)
			} else {
				_, err = app.Append(0, a.series, s.T(), s.F())
				require.NoError(t, err)
			}
		}
	}
	require.NoError(t, app.Commit())
	return db
}

func testNHCB(i int) *histogram.Histogram {
	return &histogram.Histogram{
		Schema: histogram.CustomBucketsSchema,
		CounterResetHint: func() histogram.CounterResetHint {
			if i == 0 {
				return 0
			}
			return 2
		}(),
		Count: 10 + uint64(i),
		Sum:   2.7 + float64(i),
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 4},
			{Offset: 0, Length: 0},
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0 + int64(i)},
		CustomValues:    []float64{5, 10, 20, 50, 100, 500},
	}
}

type sample struct {
	t  int64
	f  float64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func newSample(t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram) chunks.Sample {
	return sample{t, v, h, fh}
}

func (s sample) T() int64                      { return s.t }
func (s sample) F() float64                    { return s.f }
func (s sample) H() *histogram.Histogram       { return s.h }
func (s sample) FH() *histogram.FloatHistogram { return s.fh }

func (s sample) Type() chunkenc.ValueType {
	switch {
	case s.h != nil:
		return chunkenc.ValHistogram
	case s.fh != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

func (s sample) Copy() chunks.Sample {
	c := sample{t: s.t, f: s.f}
	if s.h != nil {
		c.h = s.h.Copy()
	}
	if s.fh != nil {
		c.fh = s.fh.Copy()
	}
	return c
}

func selectSeries(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]chunks.Sample {
	t.Helper()

	ss := q.Select(context.Background(), false, nil, matchers...)

	var it chunkenc.Iterator
	result := map[string][]chunks.Sample{}
	for ss.Next() {
		series := ss.At()

		it = series.Iterator(it)
		samples, err := storage.ExpandSamples(it, newSample)
		require.NoError(t, err)
		require.NoError(t, it.Err())

		if len(samples) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = samples
	}
	require.NoError(t, ss.Err())
	require.Empty(t, ss.Warnings())

	return result
}

func scaleSamples(samples []chunks.Sample, up bool, value float64) []chunks.Sample {
	ret := make([]chunks.Sample, len(samples))
	for i, s := range samples {
		if fh := s.FH(); fh != nil {
			if !fh.UsesCustomBuckets() {
				panic("can't scale native histograms ")
			}
			fh = fh.Copy()
			// Clone CustomValues since histogram.Copy() shares it by reference.
			fh.CustomValues = slices.Clone(fh.CustomValues)
			if up {
				fh.Sum *= value
				for cvi := range fh.CustomValues {
					fh.CustomValues[cvi] *= value
				}
			} else {
				fh.Sum /= value
				for cvi := range fh.CustomValues {
					fh.CustomValues[cvi] /= value
				}
			}
			ret[i] = sample{t: s.T(), fh: fh}
			continue
		}
		if h := s.H(); h != nil {
			if !h.UsesCustomBuckets() {
				panic("can't scale native histograms ")
			}
			h = h.Copy()
			// Clone CustomValues since histogram.Copy() shares it by reference.
			h.CustomValues = slices.Clone(h.CustomValues)
			if up {
				h.Sum *= value
				for cvi := range h.CustomValues {
					h.CustomValues[cvi] *= value
				}
			} else {
				h.Sum /= value
				for cvi := range h.CustomValues {
					h.CustomValues[cvi] /= value
				}
			}
			ret[i] = sample{t: s.T(), h: h}
			continue
		}
		if up {
			ret[i] = sample{t: s.T(), f: s.F() * value}
		} else {
			ret[i] = sample{t: s.T(), f: s.F() / value}
		}
	}
	return ret
}

func TestScaleSamples(t *testing.T) {
	t.Run("nhcb", func(t *testing.T) {
		testNHCBSamples := make([]chunks.Sample, 10)
		for i := range 10 {
			testNHCBSamples[i] = sample{
				t: int64(i),
				h: testNHCB(i),
			}
		}
		scaled := scaleSamples(testNHCBSamples, true, 1000)
		require.NotEqual(t, testNHCBSamples, scaled)
	})
}

func TestAwareStorage(t *testing.T) {
	const samples = 10

	testFSamples := make([]chunks.Sample, samples)
	for i := range samples {
		testFSamples[i] = sample{
			t: int64(i),
			f: float64(i),
		}
	}
	testNHCBSamples := make([]chunks.Sample, samples)
	for i := range samples {
		testNHCBSamples[i] = sample{
			t: int64(i),
			h: testNHCB(i),
		}
	}

	t.Run("counter", func(t *testing.T) {
		db := openTestDB(t, nil, []appendSeries{
			{series: testdataElementsSeriesOld, samples: testFSamples},
			{series: testdataElementsSeriesNew, samples: testFSamples},
		})

		notAware, err := db.Querier(0, samples)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, notAware.Close())
		})
		s, _ := AwareStorage(db)
		aware, err := s.Querier(0, samples)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, aware.Close())
		})

		t.Run("backward", func(t *testing.T) {
			onlyNewResult := selectSeries(t, notAware,
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataElementsSeriesNew).Name),
				labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", testdataElementsSeriesNew.Get("fraction")),
			)
			require.Equal(t, map[string][]chunks.Sample{
				`{__name__="my_app_custom_changed_elements_total", __schema_url__="` + testSchemaURL("1.1.0") + `", __type__="counter", class="FIRST", fraction="1.243", number="1", test="new"}`: testFSamples,
			}, onlyNewResult)
			got := selectSeries(t, aware,
				// Without schema selector, semconv aware storage should have no effect.
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataElementsSeriesNew).Name),
				labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", testdataElementsSeriesNew.Get("fraction")),
			)
			require.Equal(t, onlyNewResult, got)

			compatibleResult := selectSeries(t, aware,
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testdataElementsSeriesNew.Get(schemaURLLabel)),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataElementsSeriesNew).Name),
				labels.MustNewMatcher(labels.MatchNotEqual, "number", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "class", "FIRST|OTHER"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", testdataElementsSeriesNew.Get("fraction")),
			)
			require.Equal(t, map[string][]chunks.Sample{
				`{__name__="my_app_custom_changed_elements_total", __type__="counter", class="FIRST", fraction="1.243", number="1", test="new"}`: testFSamples,
				`{__name__="my_app_custom_changed_elements_total", __type__="counter", class="FIRST", fraction="1.243", number="1", test="old"}`: testFSamples,
			}, compatibleResult)
		})
		t.Run("forward", func(t *testing.T) {
			onlyOldResult := selectSeries(t, notAware,
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testdataElementsSeriesOld.Get(schemaURLLabel)),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataElementsSeriesOld).Name),
				labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", testdataElementsSeriesOld.Get("fraction")),
			)
			require.Equal(t, map[string][]chunks.Sample{
				`{__name__="my_app_custom_elements_total", __schema_url__="` + testSchemaURL("1.0.0") + `", __type__="counter", category="first", fraction="1.243", integer="1", test="old"}`: testFSamples,
			}, onlyOldResult)
			got := selectSeries(t, aware,
				// Without schema selector, semconv aware storage should have no effect.
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataElementsSeriesOld).Name),
				labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", testdataElementsSeriesOld.Get("fraction")),
			)
			require.Equal(t, onlyOldResult, got)

			compatibleResult := selectSeries(t, aware,
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testdataElementsSeriesOld.Get(schemaURLLabel)),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataElementsSeriesOld).Name),
				labels.MustNewMatcher(labels.MatchNotEqual, "integer", "2"),
				labels.MustNewMatcher(labels.MatchRegexp, "category", "first|other"),
				labels.MustNewMatcher(labels.MatchEqual, "fraction", testdataElementsSeriesOld.Get("fraction")),
			)
			require.Equal(t, map[string][]chunks.Sample{
				`{__name__="my_app_custom_elements_total", __type__="counter", category="first", fraction="1.243", integer="1", test="old"}`: testFSamples,
				`{__name__="my_app_custom_elements_total", __type__="counter", category="first", fraction="1.243", integer="1", test="new"}`: testFSamples,
			}, compatibleResult)
		})
	})
	t.Run("classic histogram", func(t *testing.T) {
		var a []appendSeries

		for _, m := range []labels.Labels{testdataLatencySeriesNew, testdataLatencySeriesOld} {
			b := labels.NewBuilder(m)
			if m.Get("test") == "new" {
				b.Set("le", "10")
			} else {
				b.Set("le", "10000")
			}

			b.Set("__name__", schema.NewMetadataFromLabels(m).Name+"_bucket")
			a = append(a, appendSeries{series: b.Labels(), samples: testFSamples})
			b = labels.NewBuilder(m)
			b.Set("__name__", schema.NewMetadataFromLabels(m).Name+"_count")
			a = append(a, appendSeries{series: b.Labels(), samples: testFSamples})
			b = labels.NewBuilder(m)
			b.Set("__name__", schema.NewMetadataFromLabels(m).Name+"_sum")
			a = append(a, appendSeries{series: b.Labels(), samples: testFSamples})
		}
		db := openTestDB(t, nil, a)

		notAware, err := db.Querier(0, samples)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, notAware.Close())
		})
		s, _ := AwareStorage(db)
		aware, err := s.Querier(0, samples)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, aware.Close())
		})

		t.Run("backward", func(t *testing.T) {
			t.Run("_bucket", func(t *testing.T) {
				onlyNewResult := selectSeries(t, notAware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_bucket"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_seconds_bucket", __schema_url__="` + testSchemaURL("1.1.0") + `", __type__="histogram", __unit__="seconds", code="200", le="10", test="new"}`: testFSamples,
				}, onlyNewResult)
				got := selectSeries(t, aware,
					// Without schema selector, semconv aware storage should have no effect.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_bucket"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, onlyNewResult, got)

				compatibleResult := selectSeries(t, aware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_bucket"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_seconds_bucket", __type__="histogram", __unit__="seconds", code="200", le="10", test="new"}`: testFSamples,
					`{__name__="my_app_latency_seconds_bucket", __type__="histogram", __unit__="seconds", code="200", le="10", test="old"}`: testFSamples,
				}, compatibleResult)
			})
			t.Run("_count", func(t *testing.T) {
				onlyNewResult := selectSeries(t, notAware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_count"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_seconds_count", __schema_url__="` + testSchemaURL("1.1.0") + `", __type__="histogram", __unit__="seconds", code="200", test="new"}`: testFSamples, // TODO(bwplotka): Type and unit proposal is not really consistent with count/sum
				}, onlyNewResult)
				got := selectSeries(t, aware,
					// Without schema selector, semconv aware storage should have no effect.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_count"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, onlyNewResult, got)

				compatibleResult := selectSeries(t, aware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_count"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_seconds_count", __type__="histogram", __unit__="seconds", code="200", test="new"}`: testFSamples,
					`{__name__="my_app_latency_seconds_count", __type__="histogram", __unit__="seconds", code="200", test="old"}`: testFSamples,
				}, compatibleResult)
			})
			t.Run("_sum", func(t *testing.T) {
				onlyNewResult := selectSeries(t, notAware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_sum"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_seconds_sum", __schema_url__="` + testSchemaURL("1.1.0") + `", __type__="histogram", __unit__="seconds", code="200", test="new"}`: testFSamples, // TODO(bwplotka): Type and unit proposal is not really consistent with count/sum
				}, onlyNewResult)
				got := selectSeries(t, aware,
					// Without schema selector, semconv aware storage should have no effect.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_sum"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, onlyNewResult, got)

				compatibleResult := selectSeries(t, aware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_seconds_sum"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_seconds_sum", __type__="histogram", __unit__="seconds", code="200", test="new"}`: testFSamples,
					`{__name__="my_app_latency_seconds_sum", __type__="histogram", __unit__="seconds", code="200", test="old"}`: scaleSamples(testFSamples, false, 1000),
				}, compatibleResult)
			})
		})
		t.Run("forward", func(t *testing.T) {
			t.Run("_bucket", func(t *testing.T) {
				onlyOldResult := selectSeries(t, notAware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.0.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_bucket"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_milliseconds_bucket", __schema_url__="` + testSchemaURL("1.0.0") + `", __type__="histogram", __unit__="milliseconds", code="200", le="10000", test="old"}`: testFSamples,
				}, onlyOldResult)
				got := selectSeries(t, aware,
					// Without schema selector, semconv aware storage should have no effect.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_bucket"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, onlyOldResult, got)

				compatibleResult := selectSeries(t, aware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.0.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_bucket"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_milliseconds_bucket", __type__="histogram", __unit__="milliseconds", code="200", le="10000", test="new"}`: testFSamples,
					`{__name__="my_app_latency_milliseconds_bucket", __type__="histogram", __unit__="milliseconds", code="200", le="10000", test="old"}`: testFSamples,
				}, compatibleResult)
			})
			t.Run("_count", func(t *testing.T) {
				onlyOldResult := selectSeries(t, notAware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.0.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_count"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_milliseconds_count", __schema_url__="` + testSchemaURL("1.0.0") + `", __type__="histogram", __unit__="milliseconds", code="200", test="old"}`: testFSamples, // TODO(bwplotka): Type and unit proposal is not really consistent with count/sum
				}, onlyOldResult)
				got := selectSeries(t, aware,
					// Without schema selector, semconv aware storage should have no effect.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_count"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, onlyOldResult, got)

				compatibleResult := selectSeries(t, aware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.0.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_count"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_milliseconds_count", __type__="histogram", __unit__="milliseconds", code="200", test="new"}`: testFSamples,
					`{__name__="my_app_latency_milliseconds_count", __type__="histogram", __unit__="milliseconds", code="200", test="old"}`: testFSamples,
				}, compatibleResult)
			})
			t.Run("_sum", func(t *testing.T) {
				onlyOldResult := selectSeries(t, notAware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.0.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_sum"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_milliseconds_sum", __schema_url__="` + testSchemaURL("1.0.0") + `", __type__="histogram", __unit__="milliseconds", code="200", test="old"}`: testFSamples, // TODO(bwplotka): Type and unit proposal is not really consistent with count/sum
				}, onlyOldResult)
				got := selectSeries(t, aware,
					// Without schema selector, semconv aware storage should have no effect.
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_sum"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, onlyOldResult, got)

				compatibleResult := selectSeries(t, aware,
					labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.0.0")),
					labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my_app_latency_milliseconds_sum"),
					labels.MustNewMatcher(labels.MatchEqual, "code", "200"),
				)
				require.Equal(t, map[string][]chunks.Sample{
					`{__name__="my_app_latency_milliseconds_sum", __type__="histogram", __unit__="milliseconds", code="200", test="old"}`: testFSamples,
					`{__name__="my_app_latency_milliseconds_sum", __type__="histogram", __unit__="milliseconds", code="200", test="new"}`: scaleSamples(testFSamples, true, 1000),
				}, compatibleResult)
			})
		})
	})
	t.Run("native histogram", func(t *testing.T) {
		db := openTestDB(t, nil, []appendSeries{
			{series: testdataLatencySeriesOld, samples: testNHCBSamples},
			{series: testdataLatencySeriesNew, samples: testNHCBSamples},
		})

		notAware, err := db.Querier(0, samples)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, notAware.Close())
		})
		s, _ := AwareStorage(db)
		aware, err := s.Querier(0, samples)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, aware.Close())
		})

		t.Run("backward", func(t *testing.T) {
			onlyNewResult := selectSeries(t, notAware,
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataLatencySeriesNew).Name),
				labels.MustNewMatcher(labels.MatchEqual, "code", testdataLatencySeriesNew.Get("code")),
			)
			require.Equal(t, map[string][]chunks.Sample{
				`{__name__="my_app_latency_seconds", __schema_url__="` + testSchemaURL("1.1.0") + `", __type__="histogram", __unit__="seconds", code="200", test="new"}`: testNHCBSamples,
			}, onlyNewResult)
			got := selectSeries(t, aware,
				// Without schema selector, semconv aware storage should have no effect.
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataLatencySeriesNew).Name),
				labels.MustNewMatcher(labels.MatchEqual, "code", testdataLatencySeriesNew.Get("code")),
			)
			require.Equal(t, onlyNewResult, got)

			compatibleResult := selectSeries(t, aware,
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.1.0")),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataLatencySeriesNew).Name),
				labels.MustNewMatcher(labels.MatchEqual, "code", testdataLatencySeriesNew.Get("code")),
			)
			require.Equal(t, map[string][]chunks.Sample{
				`{__name__="my_app_latency_seconds", __type__="histogram", __unit__="seconds", code="200", test="new"}`: testNHCBSamples,
				`{__name__="my_app_latency_seconds", __type__="histogram", __unit__="seconds", code="200", test="old"}`: scaleSamples(testNHCBSamples, false, 1000),
			}, compatibleResult)
		})
		t.Run("forward", func(t *testing.T) {
			onlyOldResult := selectSeries(t, notAware,
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.0.0")),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataLatencySeriesOld).Name),
				labels.MustNewMatcher(labels.MatchEqual, "code", testdataLatencySeriesOld.Get("code")),
			)
			require.Equal(t, map[string][]chunks.Sample{
				`{__name__="my_app_latency_milliseconds", __schema_url__="` + testSchemaURL("1.0.0") + `", __type__="histogram", __unit__="milliseconds", code="200", test="old"}`: testNHCBSamples,
			}, onlyOldResult)
			got := selectSeries(t, aware,
				// Without schema selector, semconv aware storage should have no effect.
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataLatencySeriesOld).Name),
				labels.MustNewMatcher(labels.MatchEqual, "code", testdataLatencySeriesOld.Get("code")),
			)
			require.Equal(t, onlyOldResult, got)

			compatibleResult := selectSeries(t, aware,
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, testSchemaURL("1.0.0")),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, schema.NewMetadataFromLabels(testdataLatencySeriesOld).Name),
				labels.MustNewMatcher(labels.MatchEqual, "code", testdataLatencySeriesOld.Get("code")),
			)
			require.Equal(t, map[string][]chunks.Sample{
				`{__name__="my_app_latency_milliseconds", __type__="histogram", __unit__="milliseconds", code="200", test="new"}`: scaleSamples(testNHCBSamples, true, 1000),
				`{__name__="my_app_latency_milliseconds", __type__="histogram", __unit__="milliseconds", code="200", test="old"}`: testNHCBSamples,
			}, compatibleResult)
		})
	})
}

func TestAwareStorage_PromQL_OverlappingSeries(t *testing.T) {
	const samples = 10

	testFSamples := make([]chunks.Sample, samples)
	for i := range samples {
		testFSamples[i] = sample{
			t: int64(i),
			f: float64(i),
		}
	}

	// Remove the test label so the series are identical after semconv conversion.
	seriesOld := labels.NewBuilder(testdataElementsSeriesOld).Del("test").Labels()
	seriesNew := labels.NewBuilder(testdataElementsSeriesNew).Del("test").Labels()

	ctx := context.TODO()
	e := promql.NewEngine(promql.EngineOpts{MaxSamples: 100, Timeout: 30 * time.Second})

	t.Run("rate with overlapping samples", func(t *testing.T) {
		db := openTestDB(t, nil, []appendSeries{
			{series: seriesOld, samples: testFSamples[:5]},
			{series: seriesNew, samples: testFSamples[5:]},
		})

		s, _ := AwareStorage(db)
		q, err := e.NewInstantQuery(
			ctx, s, nil,
			fmt.Sprintf("rate(%v{__schema_url__=%q}[10])", schema.NewMetadataFromLabels(seriesOld).Name, testSchemaURL("1.0.0")), timestamp.Time(10),
		)
		require.NoError(t, err)

		t.Cleanup(q.Close)

		res := q.Exec(ctx)
		require.NoError(t, res.Err)
		require.Empty(t, res.Warnings.AsErrors())
		require.Equal(t, "{category=\"first\", fraction=\"1.243\", integer=\"1\"} => 0.9999999999999998 @[10]", res.String())
	})

	t.Run("rate with duplicate samples", func(t *testing.T) {
		db := openTestDB(t, nil, []appendSeries{
			{series: seriesOld, samples: testFSamples},
			{series: seriesNew, samples: testFSamples},
		})

		s, _ := AwareStorage(db)
		q, err := e.NewInstantQuery(
			ctx, s, nil,
			fmt.Sprintf("rate(%v{__schema_url__=%q}[10])", schema.NewMetadataFromLabels(seriesOld).Name, testSchemaURL("1.0.0")), timestamp.Time(10),
		)
		require.NoError(t, err)

		t.Cleanup(q.Close)

		res := q.Exec(ctx)
		require.NoError(t, res.Err)
		require.Empty(t, res.Warnings.AsErrors())
		require.Equal(t, "{category=\"first\", fraction=\"1.243\", integer=\"1\"} => 0.9999999999999998 @[10]", res.String())
	})
}
