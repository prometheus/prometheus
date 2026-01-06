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

package record

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestRecord_EncodeDecode(t *testing.T) {
	var enc Encoder
	dec := NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())

	series := []RefSeries{
		{
			Ref:    100,
			Labels: labels.FromStrings("abc", "def", "123", "456"),
		}, {
			Ref:    1,
			Labels: labels.FromStrings("abc", "def2", "1234", "4567"),
		}, {
			Ref:    435245,
			Labels: labels.FromStrings("xyz", "def", "foo", "bar"),
		},
	}
	decSeries, err := dec.Series(enc.Series(series, nil), nil)
	require.NoError(t, err)
	testutil.RequireEqual(t, series, decSeries)

	metadata := []RefMetadata{
		{
			Ref:  100,
			Type: uint8(Counter),
			Unit: "",
			Help: "some magic counter",
		},
		{
			Ref:  1,
			Type: uint8(Counter),
			Unit: "seconds",
			Help: "CPU time counter",
		},
		{
			Ref:  147741,
			Type: uint8(Gauge),
			Unit: "percentage",
			Help: "current memory usage",
		},
	}
	decMetadata, err := dec.Metadata(enc.Metadata(metadata, nil), nil)
	require.NoError(t, err)
	require.Equal(t, metadata, decMetadata)

	samples := []RefSample{
		{Ref: 0, T: 12423423, V: 1.2345},
		{Ref: 123, T: -1231, V: -123},
		{Ref: 2, T: 0, V: 99999},
	}
	decSamples, err := dec.Samples(enc.Samples(samples, nil), nil)
	require.NoError(t, err)
	require.Equal(t, samples, decSamples)

	// Intervals get split up into single entries. So we don't get back exactly
	// what we put in.
	tstones := []tombstones.Stone{
		{Ref: 123, Intervals: tombstones.Intervals{
			{Mint: -1000, Maxt: 1231231},
			{Mint: 5000, Maxt: 0},
		}},
		{Ref: 13, Intervals: tombstones.Intervals{
			{Mint: -1000, Maxt: -11},
			{Mint: 5000, Maxt: 1000},
		}},
	}
	decTstones, err := dec.Tombstones(enc.Tombstones(tstones, nil), nil)
	require.NoError(t, err)
	require.Equal(t, []tombstones.Stone{
		{Ref: 123, Intervals: tombstones.Intervals{{Mint: -1000, Maxt: 1231231}}},
		{Ref: 123, Intervals: tombstones.Intervals{{Mint: 5000, Maxt: 0}}},
		{Ref: 13, Intervals: tombstones.Intervals{{Mint: -1000, Maxt: -11}}},
		{Ref: 13, Intervals: tombstones.Intervals{{Mint: 5000, Maxt: 1000}}},
	}, decTstones)

	exemplars := []RefExemplar{
		{Ref: 0, T: 12423423, V: 1.2345, Labels: labels.FromStrings("trace_id", "qwerty")},
		{Ref: 123, T: -1231, V: -123, Labels: labels.FromStrings("trace_id", "asdf")},
		{Ref: 2, T: 0, V: 99999, Labels: labels.FromStrings("trace_id", "zxcv")},
	}
	decExemplars, err := dec.Exemplars(enc.Exemplars(exemplars, nil), nil)
	require.NoError(t, err)
	testutil.RequireEqual(t, exemplars, decExemplars)

	histograms := []RefHistogramSample{
		{
			Ref: 56,
			T:   1234,
			H: &histogram.Histogram{
				Count:         5,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           18.4 * rand.Float64(),
				Schema:        1,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
			},
		},
		{
			Ref: 42,
			T:   5678,
			H: &histogram.Histogram{
				Count:         11,
				ZeroCount:     4,
				ZeroThreshold: 0.001,
				Sum:           35.5,
				Schema:        1,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 2, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				NegativeSpans: []histogram.Span{
					{Offset: 0, Length: 1},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{1, 2, -1},
			},
		},
		{
			Ref: 67,
			T:   5678,
			H: &histogram.Histogram{
				Count:         8,
				ZeroThreshold: 0.001,
				Sum:           35.5,
				Schema:        -53,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 2, Length: 2},
				},
				PositiveBuckets: []int64{2, -1, 2, 0},
				CustomValues:    []float64{0, 2, 4, 6, 8},
			},
		},
	}

	histSamples, customBucketsHistograms := enc.HistogramSamples(histograms, nil)
	customBucketsHistSamples := enc.CustomBucketsHistogramSamples(customBucketsHistograms, nil)
	decHistograms, err := dec.HistogramSamples(histSamples, nil)
	require.NoError(t, err)
	decCustomBucketsHistograms, err := dec.HistogramSamples(customBucketsHistSamples, nil)
	require.NoError(t, err)
	decHistograms = append(decHistograms, decCustomBucketsHistograms...)
	require.Equal(t, histograms, decHistograms)

	floatHistograms := make([]RefFloatHistogramSample, len(histograms))
	for i, h := range histograms {
		floatHistograms[i] = RefFloatHistogramSample{
			Ref: h.Ref,
			T:   h.T,
			FH:  h.H.ToFloat(nil),
		}
	}
	floatHistSamples, customBucketsFloatHistograms := enc.FloatHistogramSamples(floatHistograms, nil)
	customBucketsFloatHistSamples := enc.CustomBucketsFloatHistogramSamples(customBucketsFloatHistograms, nil)
	decFloatHistograms, err := dec.FloatHistogramSamples(floatHistSamples, nil)
	require.NoError(t, err)
	decCustomBucketsFloatHistograms, err := dec.FloatHistogramSamples(customBucketsFloatHistSamples, nil)
	require.NoError(t, err)
	decFloatHistograms = append(decFloatHistograms, decCustomBucketsFloatHistograms...)
	require.Equal(t, floatHistograms, decFloatHistograms)

	// Gauge integer histograms.
	for i := range histograms {
		histograms[i].H.CounterResetHint = histogram.GaugeType
	}

	gaugeHistSamples, customBucketsGaugeHistograms := enc.HistogramSamples(histograms, nil)
	customBucketsGaugeHistSamples := enc.CustomBucketsHistogramSamples(customBucketsGaugeHistograms, nil)
	decGaugeHistograms, err := dec.HistogramSamples(gaugeHistSamples, nil)
	require.NoError(t, err)
	decCustomBucketsGaugeHistograms, err := dec.HistogramSamples(customBucketsGaugeHistSamples, nil)
	require.NoError(t, err)
	decGaugeHistograms = append(decGaugeHistograms, decCustomBucketsGaugeHistograms...)
	require.Equal(t, histograms, decGaugeHistograms)

	// Gauge float histograms.
	for i := range floatHistograms {
		floatHistograms[i].FH.CounterResetHint = histogram.GaugeType
	}

	gaugeFloatHistSamples, customBucketsGaugeFloatHistograms := enc.FloatHistogramSamples(floatHistograms, nil)
	customBucketsGaugeFloatHistSamples := enc.CustomBucketsFloatHistogramSamples(customBucketsGaugeFloatHistograms, nil)
	decGaugeFloatHistograms, err := dec.FloatHistogramSamples(gaugeFloatHistSamples, nil)
	require.NoError(t, err)
	decCustomBucketsGaugeFloatHistograms, err := dec.FloatHistogramSamples(customBucketsGaugeFloatHistSamples, nil)
	require.NoError(t, err)
	decGaugeFloatHistograms = append(decGaugeFloatHistograms, decCustomBucketsGaugeFloatHistograms...)
	require.Equal(t, floatHistograms, decGaugeFloatHistograms)
}

func TestRecord_DecodeInvalidHistogramSchema(t *testing.T) {
	for _, schema := range []int32{-100, 100} {
		t.Run(fmt.Sprintf("schema=%d", schema), func(t *testing.T) {
			var enc Encoder

			var output bytes.Buffer
			logger := promslog.New(&promslog.Config{Writer: &output})
			dec := NewDecoder(labels.NewSymbolTable(), logger)
			histograms := []RefHistogramSample{
				{
					Ref: 56,
					T:   1234,
					H: &histogram.Histogram{
						Count:         5,
						ZeroCount:     2,
						ZeroThreshold: 0.001,
						Sum:           18.4 * rand.Float64(),
						Schema:        schema,
						PositiveSpans: []histogram.Span{
							{Offset: 0, Length: 2},
							{Offset: 1, Length: 2},
						},
						PositiveBuckets: []int64{1, 1, -1, 0},
					},
				},
			}
			histSamples, _ := enc.HistogramSamples(histograms, nil)
			decHistograms, err := dec.HistogramSamples(histSamples, nil)
			require.NoError(t, err)
			require.Empty(t, decHistograms)
			require.Contains(t, output.String(), "skipping histogram with unknown schema in WAL record")
		})
	}
}

func TestRecord_DecodeInvalidFloatHistogramSchema(t *testing.T) {
	for _, schema := range []int32{-100, 100} {
		t.Run(fmt.Sprintf("schema=%d", schema), func(t *testing.T) {
			var enc Encoder

			var output bytes.Buffer
			logger := promslog.New(&promslog.Config{Writer: &output})
			dec := NewDecoder(labels.NewSymbolTable(), logger)
			histograms := []RefFloatHistogramSample{
				{
					Ref: 56,
					T:   1234,
					FH: &histogram.FloatHistogram{
						Count:         5,
						ZeroCount:     2,
						ZeroThreshold: 0.001,
						Sum:           18.4 * rand.Float64(),
						Schema:        schema,
						PositiveSpans: []histogram.Span{
							{Offset: 0, Length: 2},
							{Offset: 1, Length: 2},
						},
						PositiveBuckets: []float64{1, 1, -1, 0},
					},
				},
			}
			histSamples, _ := enc.FloatHistogramSamples(histograms, nil)
			decHistograms, err := dec.FloatHistogramSamples(histSamples, nil)
			require.NoError(t, err)
			require.Empty(t, decHistograms)
			require.Contains(t, output.String(), "skipping histogram with unknown schema in WAL record")
		})
	}
}

func TestRecord_DecodeTooHighResolutionHistogramSchema(t *testing.T) {
	for _, schema := range []int32{9, 52} {
		t.Run(fmt.Sprintf("schema=%d", schema), func(t *testing.T) {
			var enc Encoder

			var output bytes.Buffer
			logger := promslog.New(&promslog.Config{Writer: &output})
			dec := NewDecoder(labels.NewSymbolTable(), logger)
			histograms := []RefHistogramSample{
				{
					Ref: 56,
					T:   1234,
					H: &histogram.Histogram{
						Count:         5,
						ZeroCount:     2,
						ZeroThreshold: 0.001,
						Sum:           18.4 * rand.Float64(),
						Schema:        schema,
						PositiveSpans: []histogram.Span{
							{Offset: 0, Length: 2},
							{Offset: 1, Length: 2},
						},
						PositiveBuckets: []int64{1, 1, -1, 0},
					},
				},
			}
			histSamples, _ := enc.HistogramSamples(histograms, nil)
			decHistograms, err := dec.HistogramSamples(histSamples, nil)
			require.NoError(t, err)
			require.Len(t, decHistograms, 1)
			require.Equal(t, histogram.ExponentialSchemaMax, decHistograms[0].H.Schema)
		})
	}
}

func TestRecord_DecodeTooHighResolutionFloatHistogramSchema(t *testing.T) {
	for _, schema := range []int32{9, 52} {
		t.Run(fmt.Sprintf("schema=%d", schema), func(t *testing.T) {
			var enc Encoder

			var output bytes.Buffer
			logger := promslog.New(&promslog.Config{Writer: &output})
			dec := NewDecoder(labels.NewSymbolTable(), logger)
			histograms := []RefFloatHistogramSample{
				{
					Ref: 56,
					T:   1234,
					FH: &histogram.FloatHistogram{
						Count:         5,
						ZeroCount:     2,
						ZeroThreshold: 0.001,
						Sum:           18.4 * rand.Float64(),
						Schema:        schema,
						PositiveSpans: []histogram.Span{
							{Offset: 0, Length: 2},
							{Offset: 1, Length: 2},
						},
						PositiveBuckets: []float64{1, 1, -1, 0},
					},
				},
			}
			histSamples, _ := enc.FloatHistogramSamples(histograms, nil)
			decHistograms, err := dec.FloatHistogramSamples(histSamples, nil)
			require.NoError(t, err)
			require.Len(t, decHistograms, 1)
			require.Equal(t, histogram.ExponentialSchemaMax, decHistograms[0].FH.Schema)
		})
	}
}

// TestRecord_Corrupted ensures that corrupted records return the correct error.
// Bugfix check for pull/521 and pull/523.
func TestRecord_Corrupted(t *testing.T) {
	var enc Encoder
	dec := NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())

	t.Run("Test corrupted series record", func(t *testing.T) {
		series := []RefSeries{
			{
				Ref:    100,
				Labels: labels.FromStrings("abc", "def", "123", "456"),
			},
		}

		corrupted := enc.Series(series, nil)[:8]
		_, err := dec.Series(corrupted, nil)
		require.Equal(t, err, encoding.ErrInvalidSize)
	})

	t.Run("Test corrupted sample record", func(t *testing.T) {
		samples := []RefSample{
			{Ref: 0, T: 12423423, V: 1.2345},
		}

		corrupted := enc.Samples(samples, nil)[:8]
		_, err := dec.Samples(corrupted, nil)
		require.ErrorIs(t, err, encoding.ErrInvalidSize)
	})

	t.Run("Test corrupted tombstone record", func(t *testing.T) {
		tstones := []tombstones.Stone{
			{Ref: 123, Intervals: tombstones.Intervals{
				{Mint: -1000, Maxt: 1231231},
				{Mint: 5000, Maxt: 0},
			}},
		}

		corrupted := enc.Tombstones(tstones, nil)[:8]
		_, err := dec.Tombstones(corrupted, nil)
		require.Equal(t, err, encoding.ErrInvalidSize)
	})

	t.Run("Test corrupted exemplar record", func(t *testing.T) {
		exemplars := []RefExemplar{
			{Ref: 0, T: 12423423, V: 1.2345, Labels: labels.FromStrings("trace_id", "asdf")},
		}

		corrupted := enc.Exemplars(exemplars, nil)[:8]
		_, err := dec.Exemplars(corrupted, nil)
		require.ErrorIs(t, err, encoding.ErrInvalidSize)
	})

	t.Run("Test corrupted metadata record", func(t *testing.T) {
		meta := []RefMetadata{
			{Ref: 147, Type: uint8(Counter), Unit: "unit", Help: "help"},
		}

		corrupted := enc.Metadata(meta, nil)[:8]
		_, err := dec.Metadata(corrupted, nil)
		require.ErrorIs(t, err, encoding.ErrInvalidSize)
	})

	t.Run("Test corrupted histogram record", func(t *testing.T) {
		histograms := []RefHistogramSample{
			{
				Ref: 56,
				T:   1234,
				H: &histogram.Histogram{
					Count:         5,
					ZeroCount:     2,
					ZeroThreshold: 0.001,
					Sum:           18.4 * rand.Float64(),
					Schema:        1,
					PositiveSpans: []histogram.Span{
						{Offset: 0, Length: 2},
						{Offset: 1, Length: 2},
					},
					PositiveBuckets: []int64{1, 1, -1, 0},
				},
			},
			{
				Ref: 67,
				T:   5678,
				H: &histogram.Histogram{
					Count:         8,
					ZeroThreshold: 0.001,
					Sum:           35.5,
					Schema:        -53,
					PositiveSpans: []histogram.Span{
						{Offset: 0, Length: 2},
						{Offset: 2, Length: 2},
					},
					PositiveBuckets: []int64{2, -1, 2, 0},
					CustomValues:    []float64{0, 2, 4, 6, 8},
				},
			},
		}

		corruptedHists, customBucketsHists := enc.HistogramSamples(histograms, nil)
		corruptedHists = corruptedHists[:8]
		corruptedCustomBucketsHists := enc.CustomBucketsHistogramSamples(customBucketsHists, nil)
		corruptedCustomBucketsHists = corruptedCustomBucketsHists[:8]
		_, err := dec.HistogramSamples(corruptedHists, nil)
		require.ErrorIs(t, err, encoding.ErrInvalidSize)
		_, err = dec.HistogramSamples(corruptedCustomBucketsHists, nil)
		require.ErrorIs(t, err, encoding.ErrInvalidSize)
	})
}

func TestRecord_Type(t *testing.T) {
	var enc Encoder
	var dec Decoder

	series := []RefSeries{{Ref: 100, Labels: labels.FromStrings("abc", "123")}}
	recordType := dec.Type(enc.Series(series, nil))
	require.Equal(t, Series, recordType)

	samples := []RefSample{{Ref: 123, T: 12345, V: 1.2345}}
	recordType = dec.Type(enc.Samples(samples, nil))
	require.Equal(t, Samples, recordType)

	tstones := []tombstones.Stone{{Ref: 1, Intervals: tombstones.Intervals{{Mint: 1, Maxt: 2}}}}
	recordType = dec.Type(enc.Tombstones(tstones, nil))
	require.Equal(t, Tombstones, recordType)

	metadata := []RefMetadata{{Ref: 147, Type: uint8(Counter), Unit: "unit", Help: "help"}}
	recordType = dec.Type(enc.Metadata(metadata, nil))
	require.Equal(t, Metadata, recordType)

	histograms := []RefHistogramSample{
		{
			Ref: 56,
			T:   1234,
			H: &histogram.Histogram{
				Count:         5,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           18.4 * rand.Float64(),
				Schema:        1,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
			},
		},
		{
			Ref: 67,
			T:   5678,
			H: &histogram.Histogram{
				Count:         8,
				ZeroThreshold: 0.001,
				Sum:           35.5,
				Schema:        -53,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 2, Length: 2},
				},
				PositiveBuckets: []int64{2, -1, 2, 0},
				CustomValues:    []float64{0, 2, 4, 6, 8},
			},
		},
	}
	hists, customBucketsHistograms := enc.HistogramSamples(histograms, nil)
	recordType = dec.Type(hists)
	require.Equal(t, HistogramSamples, recordType)
	customBucketsHists := enc.CustomBucketsHistogramSamples(customBucketsHistograms, nil)
	recordType = dec.Type(customBucketsHists)
	require.Equal(t, CustomBucketsHistogramSamples, recordType)

	recordType = dec.Type(nil)
	require.Equal(t, Unknown, recordType)

	recordType = dec.Type([]byte{0})
	require.Equal(t, Unknown, recordType)
}

func TestRecord_MetadataDecodeUnknownExtraFields(t *testing.T) {
	var enc encoding.Encbuf
	var dec Decoder

	// Write record type.
	enc.PutByte(byte(Metadata))

	// Write first metadata entry, all known fields.
	enc.PutUvarint64(101)
	enc.PutByte(byte(Counter))
	enc.PutUvarint(2)
	enc.PutUvarintStr(unitMetaName)
	enc.PutUvarintStr("")
	enc.PutUvarintStr(helpMetaName)
	enc.PutUvarintStr("some magic counter")

	// Write second metadata entry, known fields + unknown fields.
	enc.PutUvarint64(99)
	enc.PutByte(byte(Counter))
	enc.PutUvarint(3)
	// Known fields.
	enc.PutUvarintStr(unitMetaName)
	enc.PutUvarintStr("seconds")
	enc.PutUvarintStr(helpMetaName)
	enc.PutUvarintStr("CPU time counter")
	// Unknown fields.
	enc.PutUvarintStr("an extra field name to be skipped")
	enc.PutUvarintStr("with its value")

	// Write third metadata entry, with unknown fields and different order.
	enc.PutUvarint64(47250)
	enc.PutByte(byte(Gauge))
	enc.PutUvarint(4)
	enc.PutUvarintStr("extra name one")
	enc.PutUvarintStr("extra value one")
	enc.PutUvarintStr(helpMetaName)
	enc.PutUvarintStr("current memory usage")
	enc.PutUvarintStr("extra name two")
	enc.PutUvarintStr("extra value two")
	enc.PutUvarintStr(unitMetaName)
	enc.PutUvarintStr("percentage")

	// Should yield known fields for all entries and skip over unknown fields.
	expectedMetadata := []RefMetadata{
		{
			Ref:  101,
			Type: uint8(Counter),
			Unit: "",
			Help: "some magic counter",
		}, {
			Ref:  99,
			Type: uint8(Counter),
			Unit: "seconds",
			Help: "CPU time counter",
		}, {
			Ref:  47250,
			Type: uint8(Gauge),
			Unit: "percentage",
			Help: "current memory usage",
		},
	}

	decMetadata, err := dec.Metadata(enc.Get(), nil)
	require.NoError(t, err)
	require.Equal(t, expectedMetadata, decMetadata)
}

type refsCreateFn func(labelCount, histograms, buckets int) ([]RefSeries, []RefSample, []RefHistogramSample)

type recordsMaker struct {
	name string
	make refsCreateFn
}

// BenchmarkWAL_HistogramEncoding measures efficiency of encoding classic
// histograms and native histograms with custom buckets (NHCB).
func BenchmarkWAL_HistogramEncoding(b *testing.B) {
	initClassicRefs := func(labelCount, histograms, buckets int) (series []RefSeries, floatSamples []RefSample, histSamples []RefHistogramSample) {
		ref := chunks.HeadSeriesRef(0)
		lbls := map[string]string{}
		for i := range labelCount {
			lbls[fmt.Sprintf("l%d", i)] = fmt.Sprintf("v%d", i)
		}
		for i := range histograms {
			lbls[model.MetricNameLabel] = fmt.Sprintf("series_%d_count", i)
			series = append(series, RefSeries{
				Ref:    ref,
				Labels: labels.FromMap(lbls),
			})
			floatSamples = append(floatSamples, RefSample{
				Ref: ref,
				T:   100,
				V:   float64(i),
			})
			ref++

			lbls[model.MetricNameLabel] = fmt.Sprintf("series_%d_sum", i)
			series = append(series, RefSeries{
				Ref:    ref,
				Labels: labels.FromMap(lbls),
			})
			floatSamples = append(floatSamples, RefSample{
				Ref: ref,
				T:   100,
				V:   float64(i),
			})
			ref++

			if buckets == 0 {
				continue
			}
			lbls[model.MetricNameLabel] = fmt.Sprintf("series_%d_bucket", i)
			for j := range buckets {
				lbls[model.BucketLabel] = fmt.Sprintf("%d.0", j)
				series = append(series, RefSeries{
					Ref:    ref,
					Labels: labels.FromMap(lbls),
				})
				floatSamples = append(floatSamples, RefSample{
					Ref: ref,
					T:   100,
					V:   float64(i + j),
				})
				ref++
			}
			delete(lbls, model.BucketLabel)
		}
		return series, floatSamples, histSamples
	}

	initNHCBRefs := func(labelCount, histograms, buckets int) (series []RefSeries, floatSamples []RefSample, histSamples []RefHistogramSample) {
		ref := chunks.HeadSeriesRef(0)
		lbls := map[string]string{}
		for i := range labelCount {
			lbls[fmt.Sprintf("l%d", i)] = fmt.Sprintf("v%d", i)
		}
		for i := range histograms {
			lbls[model.MetricNameLabel] = fmt.Sprintf("series_%d", i)
			series = append(series, RefSeries{
				Ref:    ref,
				Labels: labels.FromMap(lbls),
			})
			h := &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           uint64(i),
				Sum:             float64(i),
				PositiveSpans:   []histogram.Span{{Length: uint32(buckets)}},
				PositiveBuckets: make([]int64, buckets+1),
				CustomValues:    make([]float64, buckets),
			}
			for j := range buckets {
				h.PositiveBuckets[j] = int64(i + j)
			}
			histSamples = append(histSamples, RefHistogramSample{
				Ref: ref,
				T:   100,
				H:   h,
			})
			ref++
		}
		return series, floatSamples, histSamples
	}

	for _, maker := range []recordsMaker{
		{
			name: "classic",
			make: initClassicRefs,
		},
		{
			name: "nhcb",
			make: initNHCBRefs,
		},
	} {
		for _, labelCount := range []int{0, 10, 50} {
			for _, histograms := range []int{10, 100, 1000} {
				for _, buckets := range []int{0, 1, 10, 100} {
					b.Run(fmt.Sprintf("type=%s/labels=%d/histograms=%d/buckets=%d", maker.name, labelCount, histograms, buckets), func(b *testing.B) {
						series, samples, nhcbs := maker.make(labelCount, histograms, buckets)
						enc := Encoder{}
						for b.Loop() {
							var buf []byte
							enc.Series(series, buf)
							enc.Samples(samples, buf)
							var leftOver []RefHistogramSample
							_, leftOver = enc.HistogramSamples(nhcbs, buf)
							if len(leftOver) > 0 {
								enc.CustomBucketsHistogramSamples(leftOver, buf)
							}
							b.ReportMetric(float64(len(buf)), "recordBytes/ops")
						}
					})
				}
			}
		}
	}
}
