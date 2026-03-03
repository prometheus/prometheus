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

	// Without ST.
	samples := []RefSample{
		{Ref: 0, T: 12423423, V: 1.2345},
		{Ref: 123, T: -1231, V: -123},
		{Ref: 2, T: 0, V: 99999},
	}
	encoded := enc.Samples(samples, nil)
	require.Equal(t, Samples, dec.Type(encoded))
	decSamples, err := dec.Samples(encoded, nil)
	require.NoError(t, err)
	require.Equal(t, samples, decSamples)

	enc = Encoder{EnableSTStorage: true}
	// Without ST again, but with V1 encoder that enables SamplesV2.
	samples = []RefSample{
		{Ref: 0, T: 12423423, V: 1.2345},
		{Ref: 123, T: -1231, V: -123},
		{Ref: 2, T: 0, V: 99999},
	}
	encoded = enc.Samples(samples, nil)
	require.Equal(t, SamplesV2, dec.Type(encoded))
	decSamples, err = dec.Samples(encoded, nil)
	require.NoError(t, err)
	require.Equal(t, samples, decSamples)

	// With ST.
	samplesWithST := []RefSample{
		{Ref: 0, T: 12423423, ST: 14, V: 1.2345},
		{Ref: 123, T: -1231, ST: 14, V: -123},
		{Ref: 2, T: 0, ST: 14, V: 99999},
	}
	encoded = enc.Samples(samplesWithST, nil)
	require.Equal(t, SamplesV2, dec.Type(encoded))
	decSamples, err = dec.Samples(encoded, nil)
	require.NoError(t, err)
	require.Equal(t, samplesWithST, decSamples)

	// With ST (ST[i] == T[i-1]).
	samplesWithSTDelta := []RefSample{
		{Ref: 0, T: 12423400, ST: 12423300, V: 1.2345},
		{Ref: 123, T: 12423500, ST: 12423400, V: -123},
		{Ref: 2, T: 12423600, ST: 12423500, V: 99999},
	}
	decSamples, err = dec.Samples(enc.Samples(samplesWithSTDelta, nil), nil)
	require.NoError(t, err)
	require.Equal(t, samplesWithSTDelta, decSamples)

	// With ST (ST[i] == ST[i-1]).
	samplesWithConstST := []RefSample{
		{Ref: 0, T: 12423400, ST: 12423300, V: 1.2345},
		{Ref: 123, T: 12423500, ST: 12423300, V: -123},
		{Ref: 2, T: 12423600, ST: 12423300, V: 99999},
	}
	decSamples, err = dec.Samples(enc.Samples(samplesWithConstST, nil), nil)
	require.NoError(t, err)
	require.Equal(t, samplesWithConstST, decSamples)

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

	// V2 int-histogram round-trip tests covering all ST scenarios.
	enc = Encoder{EnableSTStorage: true}

	t.Run("V2 int-histogram no ST", func(t *testing.T) {
		histsV2NoST := []RefHistogramSample{
			{Ref: 56, T: 1234, H: histograms[0].H},
			{Ref: 42, T: 5678, H: histograms[1].H},
			{Ref: 67, T: 5678, H: histograms[2].H},
		}
		histSamplesV2, customBucketsV2 := enc.HistogramSamples(histsV2NoST, nil)
		customBucketsHistSamplesV2 := enc.CustomBucketsHistogramSamples(customBucketsV2, nil)
		decHistsV2, err := dec.HistogramSamples(histSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsV2, err := dec.HistogramSamples(customBucketsHistSamplesV2, nil)
		require.NoError(t, err)
		decHistsV2 = append(decHistsV2, decCustomBucketsV2...)
		require.Equal(t, histsV2NoST, decHistsV2)
	})

	t.Run("V2 int-histogram constant ST", func(t *testing.T) {
		histsV2ConstST := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 1000, H: histograms[0].H},
			{Ref: 42, T: 5678, ST: 1000, H: histograms[1].H},
			{Ref: 67, T: 5678, ST: 1000, H: histograms[2].H},
		}
		histSamplesV2, customBucketsV2 := enc.HistogramSamples(histsV2ConstST, nil)
		customBucketsHistSamplesV2 := enc.CustomBucketsHistogramSamples(customBucketsV2, nil)
		decHistsV2, err := dec.HistogramSamples(histSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsV2, err := dec.HistogramSamples(customBucketsHistSamplesV2, nil)
		require.NoError(t, err)
		decHistsV2 = append(decHistsV2, decCustomBucketsV2...)
		require.Equal(t, histsV2ConstST, decHistsV2)
	})

	t.Run("V2 int-histogram varying ST", func(t *testing.T) {
		histsV2VarST := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 1000, H: histograms[0].H},
			{Ref: 42, T: 5678, ST: 1234, H: histograms[1].H},
			{Ref: 67, T: 9012, ST: 5678, H: histograms[2].H},
		}
		histSamplesV2, customBucketsV2 := enc.HistogramSamples(histsV2VarST, nil)
		customBucketsHistSamplesV2 := enc.CustomBucketsHistogramSamples(customBucketsV2, nil)
		decHistsV2, err := dec.HistogramSamples(histSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsV2, err := dec.HistogramSamples(customBucketsHistSamplesV2, nil)
		require.NoError(t, err)
		decHistsV2 = append(decHistsV2, decCustomBucketsV2...)
		require.Equal(t, histsV2VarST, decHistsV2)
	})

	t.Run("V2 int-histogram same ST across samples", func(t *testing.T) {
		histsV2SameST := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 900, H: histograms[0].H},
			{Ref: 42, T: 5678, ST: 900, H: histograms[1].H},
			{Ref: 67, T: 9012, ST: 900, H: histograms[2].H},
		}
		histSamplesV2, customBucketsV2 := enc.HistogramSamples(histsV2SameST, nil)
		customBucketsHistSamplesV2 := enc.CustomBucketsHistogramSamples(customBucketsV2, nil)
		decHistsV2, err := dec.HistogramSamples(histSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsV2, err := dec.HistogramSamples(customBucketsHistSamplesV2, nil)
		require.NoError(t, err)
		decHistsV2 = append(decHistsV2, decCustomBucketsV2...)
		require.Equal(t, histsV2SameST, decHistsV2)
	})

	// V2 float-histogram round-trip tests covering all ST scenarios.
	t.Run("V2 float-histogram no ST", func(t *testing.T) {
		histsV2NoST := []RefHistogramSample{
			{Ref: 56, T: 1234, H: histograms[0].H},
			{Ref: 42, T: 5678, H: histograms[1].H},
			{Ref: 67, T: 5678, H: histograms[2].H},
		}
		floatHistsV2 := make([]RefFloatHistogramSample, len(histsV2NoST))
		for i, h := range histsV2NoST {
			floatHistsV2[i] = RefFloatHistogramSample{
				Ref: h.Ref,
				T:   h.T,
				ST:  h.ST,
				FH:  h.H.ToFloat(nil),
			}
		}
		floatHistSamplesV2, customBucketsFloatV2 := enc.FloatHistogramSamples(floatHistsV2, nil)
		customBucketsFloatHistSamplesV2 := enc.CustomBucketsFloatHistogramSamples(customBucketsFloatV2, nil)
		decFloatHistsV2, err := dec.FloatHistogramSamples(floatHistSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsFloatV2, err := dec.FloatHistogramSamples(customBucketsFloatHistSamplesV2, nil)
		require.NoError(t, err)
		decFloatHistsV2 = append(decFloatHistsV2, decCustomBucketsFloatV2...)
		require.Equal(t, floatHistsV2, decFloatHistsV2)
	})

	t.Run("V2 float-histogram constant ST", func(t *testing.T) {
		histsV2ConstST := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 1000, H: histograms[0].H},
			{Ref: 42, T: 5678, ST: 1000, H: histograms[1].H},
			{Ref: 67, T: 5678, ST: 1000, H: histograms[2].H},
		}
		floatHistsV2 := make([]RefFloatHistogramSample, len(histsV2ConstST))
		for i, h := range histsV2ConstST {
			floatHistsV2[i] = RefFloatHistogramSample{
				Ref: h.Ref,
				T:   h.T,
				ST:  h.ST,
				FH:  h.H.ToFloat(nil),
			}
		}
		floatHistSamplesV2, customBucketsFloatV2 := enc.FloatHistogramSamples(floatHistsV2, nil)
		customBucketsFloatHistSamplesV2 := enc.CustomBucketsFloatHistogramSamples(customBucketsFloatV2, nil)
		decFloatHistsV2, err := dec.FloatHistogramSamples(floatHistSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsFloatV2, err := dec.FloatHistogramSamples(customBucketsFloatHistSamplesV2, nil)
		require.NoError(t, err)
		decFloatHistsV2 = append(decFloatHistsV2, decCustomBucketsFloatV2...)
		require.Equal(t, floatHistsV2, decFloatHistsV2)
	})

	t.Run("V2 float-histogram varying ST", func(t *testing.T) {
		histsV2VarST := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 1000, H: histograms[0].H},
			{Ref: 42, T: 5678, ST: 1234, H: histograms[1].H},
			{Ref: 67, T: 9012, ST: 5678, H: histograms[2].H},
		}
		floatHistsV2 := make([]RefFloatHistogramSample, len(histsV2VarST))
		for i, h := range histsV2VarST {
			floatHistsV2[i] = RefFloatHistogramSample{
				Ref: h.Ref,
				T:   h.T,
				ST:  h.ST,
				FH:  h.H.ToFloat(nil),
			}
		}
		floatHistSamplesV2, customBucketsFloatV2 := enc.FloatHistogramSamples(floatHistsV2, nil)
		customBucketsFloatHistSamplesV2 := enc.CustomBucketsFloatHistogramSamples(customBucketsFloatV2, nil)
		decFloatHistsV2, err := dec.FloatHistogramSamples(floatHistSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsFloatV2, err := dec.FloatHistogramSamples(customBucketsFloatHistSamplesV2, nil)
		require.NoError(t, err)
		decFloatHistsV2 = append(decFloatHistsV2, decCustomBucketsFloatV2...)
		require.Equal(t, floatHistsV2, decFloatHistsV2)
	})

	t.Run("V2 float-histogram same ST across samples", func(t *testing.T) {
		histsV2SameST := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 900, H: histograms[0].H},
			{Ref: 42, T: 5678, ST: 900, H: histograms[1].H},
			{Ref: 67, T: 9012, ST: 900, H: histograms[2].H},
		}
		floatHistsV2 := make([]RefFloatHistogramSample, len(histsV2SameST))
		for i, h := range histsV2SameST {
			floatHistsV2[i] = RefFloatHistogramSample{
				Ref: h.Ref,
				T:   h.T,
				ST:  h.ST,
				FH:  h.H.ToFloat(nil),
			}
		}
		floatHistSamplesV2, customBucketsFloatV2 := enc.FloatHistogramSamples(floatHistsV2, nil)
		customBucketsFloatHistSamplesV2 := enc.CustomBucketsFloatHistogramSamples(customBucketsFloatV2, nil)
		decFloatHistsV2, err := dec.FloatHistogramSamples(floatHistSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsFloatV2, err := dec.FloatHistogramSamples(customBucketsFloatHistSamplesV2, nil)
		require.NoError(t, err)
		decFloatHistsV2 = append(decFloatHistsV2, decCustomBucketsFloatV2...)
		require.Equal(t, floatHistsV2, decFloatHistsV2)
	})

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

	// V2 gauge int-histogram round-trip.
	t.Run("V2 gauge int-histogram", func(t *testing.T) {
		enc = Encoder{EnableSTStorage: true}
		gaugeHistsV2 := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 1000, H: histograms[0].H},
			{Ref: 42, T: 5678, ST: 1000, H: histograms[1].H},
			{Ref: 67, T: 5678, ST: 1000, H: histograms[2].H},
		}
		histSamplesV2, customBucketsV2 := enc.HistogramSamples(gaugeHistsV2, nil)
		customBucketsHistSamplesV2 := enc.CustomBucketsHistogramSamples(customBucketsV2, nil)
		decHistsV2, err := dec.HistogramSamples(histSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsV2, err := dec.HistogramSamples(customBucketsHistSamplesV2, nil)
		require.NoError(t, err)
		decHistsV2 = append(decHistsV2, decCustomBucketsV2...)
		require.Equal(t, gaugeHistsV2, decHistsV2)
	})

	// V2 gauge float-histogram round-trip.
	t.Run("V2 gauge float-histogram", func(t *testing.T) {
		gaugeHistsV2 := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 1000, H: histograms[0].H},
			{Ref: 42, T: 5678, ST: 1000, H: histograms[1].H},
			{Ref: 67, T: 5678, ST: 1000, H: histograms[2].H},
		}
		gaugeFloatHistsV2 := make([]RefFloatHistogramSample, len(gaugeHistsV2))
		for i, h := range gaugeHistsV2 {
			gaugeFloatHistsV2[i] = RefFloatHistogramSample{
				Ref: h.Ref,
				T:   h.T,
				ST:  h.ST,
				FH:  h.H.ToFloat(nil),
			}
		}
		floatHistSamplesV2, customBucketsFloatV2 := enc.FloatHistogramSamples(gaugeFloatHistsV2, nil)
		customBucketsFloatHistSamplesV2 := enc.CustomBucketsFloatHistogramSamples(customBucketsFloatV2, nil)
		decFloatHistsV2, err := dec.FloatHistogramSamples(floatHistSamplesV2, nil)
		require.NoError(t, err)
		decCustomBucketsFloatV2, err := dec.FloatHistogramSamples(customBucketsFloatHistSamplesV2, nil)
		require.NoError(t, err)
		decFloatHistsV2 = append(decFloatHistsV2, decCustomBucketsFloatV2...)
		require.Equal(t, gaugeFloatHistsV2, decFloatHistsV2)
	})

	for _, enableSTStorage := range []bool{false, true} {
		t.Run(fmt.Sprintf("int-histogram empty slice stStorage=%v", enableSTStorage), func(t *testing.T) {
			enc := Encoder{EnableSTStorage: enableSTStorage}
			histBuf, customBuckets := enc.HistogramSamples(nil, nil)
			require.Nil(t, customBuckets)

			decoded, err := dec.HistogramSamples(histBuf, nil)
			require.NoError(t, err)
			require.Empty(t, decoded)
		})

		t.Run(fmt.Sprintf("float-histogram empty slice stStorage=%v", enableSTStorage), func(t *testing.T) {
			enc := Encoder{EnableSTStorage: enableSTStorage}
			floatBuf, customBucketsFloat := enc.FloatHistogramSamples(nil, nil)
			require.Nil(t, customBucketsFloat)

			decoded, err := dec.FloatHistogramSamples(floatBuf, nil)
			require.NoError(t, err)
			require.Empty(t, decoded)
		})
	}

	// When all histograms are custom-bucket, HistogramSamples must return an
	// empty buffer (buf.Reset path) and pass every sample through as custom.
	t.Run("V2 int-histogram all custom bucket", func(t *testing.T) {
		allCustom := []RefHistogramSample{
			{Ref: 56, T: 1234, ST: 1000, H: histograms[2].H},
			{Ref: 67, T: 5678, ST: 1000, H: histograms[2].H},
		}
		histBuf, customBuckets := enc.HistogramSamples(allCustom, nil)
		require.Empty(t, histBuf, "regular histogram buffer must be empty when all samples are custom bucket")
		require.Equal(t, allCustom, customBuckets)

		customBuf := enc.CustomBucketsHistogramSamples(customBuckets, nil)
		decoded, err := dec.HistogramSamples(customBuf, nil)
		require.NoError(t, err)
		require.Equal(t, allCustom, decoded)
	})

	t.Run("V2 float-histogram all custom bucket", func(t *testing.T) {
		allCustomFloat := []RefFloatHistogramSample{
			{Ref: 56, T: 1234, ST: 1000, FH: histograms[2].H.ToFloat(nil)},
			{Ref: 67, T: 5678, ST: 1000, FH: histograms[2].H.ToFloat(nil)},
		}
		floatBuf, customBucketsFloat := enc.FloatHistogramSamples(allCustomFloat, nil)
		require.Empty(t, floatBuf, "regular float histogram buffer must be empty when all samples are custom bucket")
		require.Equal(t, allCustomFloat, customBucketsFloat)

		customFloatBuf := enc.CustomBucketsFloatHistogramSamples(customBucketsFloat, nil)
		decoded, err := dec.FloatHistogramSamples(customFloatBuf, nil)
		require.NoError(t, err)
		require.Equal(t, allCustomFloat, decoded)
	})

	// When all histograms are custom-bucket, V1 HistogramSamples must return an
	// empty buffer (buf.Reset path) and pass every sample through as custom.
	t.Run("V1 int-histogram all custom bucket", func(t *testing.T) {
		encV1 := Encoder{}
		allCustom := []RefHistogramSample{
			{Ref: 56, T: 1234, H: histograms[2].H},
			{Ref: 67, T: 5678, H: histograms[2].H},
		}
		histBuf, customBuckets := encV1.HistogramSamples(allCustom, nil)
		require.Empty(t, histBuf, "regular histogram buffer must be empty when all samples are custom bucket")
		require.Equal(t, allCustom, customBuckets)

		customBuf := encV1.CustomBucketsHistogramSamples(customBuckets, nil)
		decoded, err := dec.HistogramSamples(customBuf, nil)
		require.NoError(t, err)
		require.Equal(t, allCustom, decoded)
	})

	t.Run("V1 float-histogram all custom bucket", func(t *testing.T) {
		encV1 := Encoder{}
		allCustomFloat := []RefFloatHistogramSample{
			{Ref: 56, T: 1234, FH: histograms[2].H.ToFloat(nil)},
			{Ref: 67, T: 5678, FH: histograms[2].H.ToFloat(nil)},
		}
		floatBuf, customBucketsFloat := encV1.FloatHistogramSamples(allCustomFloat, nil)
		require.Empty(t, floatBuf, "regular float histogram buffer must be empty when all samples are custom bucket")
		require.Equal(t, allCustomFloat, customBucketsFloat)

		customFloatBuf := encV1.CustomBucketsFloatHistogramSamples(customBucketsFloat, nil)
		decoded, err := dec.FloatHistogramSamples(customFloatBuf, nil)
		require.NoError(t, err)
		require.Equal(t, allCustomFloat, decoded)
	})

	// Backward compat: V1-encoded histograms decode with ST=0.
	t.Run("V1 backward compat int-histogram ST=0", func(t *testing.T) {
		encV1 := Encoder{}
		v1HistSamples, v1CustomBucketsHists := encV1.HistogramSamples(histograms, nil)
		v1CustomBucketsHistSamples := encV1.CustomBucketsHistogramSamples(v1CustomBucketsHists, nil)
		decV1Hists, err := dec.HistogramSamples(v1HistSamples, nil)
		require.NoError(t, err)
		decV1CustomBuckets, err := dec.HistogramSamples(v1CustomBucketsHistSamples, nil)
		require.NoError(t, err)
		for _, h := range append(decV1Hists, decV1CustomBuckets...) {
			require.Equal(t, int64(0), h.ST, "V1 histogram records must decode with ST=0")
		}
	})

	// Backward compat: V1-encoded float histograms decode with ST=0.
	t.Run("V1 backward compat float-histogram ST=0", func(t *testing.T) {
		encV1 := Encoder{}
		v1FloatHistSamples, v1CustomBucketsFloatHists := encV1.FloatHistogramSamples(floatHistograms, nil)
		v1CustomBucketsFloatHistSamples := encV1.CustomBucketsFloatHistogramSamples(v1CustomBucketsFloatHists, nil)
		decV1FloatHists, err := dec.FloatHistogramSamples(v1FloatHistSamples, nil)
		require.NoError(t, err)
		decV1CustomBucketsFloatHists, err := dec.FloatHistogramSamples(v1CustomBucketsFloatHistSamples, nil)
		require.NoError(t, err)
		for _, h := range append(decV1FloatHists, decV1CustomBucketsFloatHists...) {
			require.Equal(t, int64(0), h.ST, "V1 float histogram records must decode with ST=0")
		}
	})

	enc = Encoder{EnableSTStorage: true}

	// V2 int-histogram with custom bucket as first element.
	t.Run("V2 int-histogram leading custom bucket", func(t *testing.T) {
		// histograms[2] uses custom buckets; placing it first exercises the
		// code path where the anchor must be the first non-custom-bucket sample.
		input := []RefHistogramSample{
			{Ref: 67, T: 1000, ST: 500, H: histograms[2].H},
			{Ref: 56, T: 2000, ST: 500, H: histograms[0].H},
			{Ref: 42, T: 3000, ST: 600, H: histograms[1].H},
		}
		histBuf, customBuckets := enc.HistogramSamples(input, nil)
		customBuf := enc.CustomBucketsHistogramSamples(customBuckets, nil)
		decHists, err := dec.HistogramSamples(histBuf, nil)
		require.NoError(t, err)
		decCustom, err := dec.HistogramSamples(customBuf, nil)
		require.NoError(t, err)
		require.Equal(t, input[1:], decHists)
		require.Equal(t, input[:1], decCustom)
	})

	// V2 int-histogram with custom bucket in the middle.
	t.Run("V2 int-histogram interleaved custom bucket", func(t *testing.T) {
		// histograms[2] uses custom buckets; placing it between regular histograms
		// exercises the prev-tracking fix so ref deltas are computed correctly.
		input := []RefHistogramSample{
			{Ref: 56, T: 1000, ST: 500, H: histograms[0].H},
			{Ref: 67, T: 2000, ST: 500, H: histograms[2].H},
			{Ref: 42, T: 3000, ST: 600, H: histograms[1].H},
		}
		histBuf, customBuckets := enc.HistogramSamples(input, nil)
		customBuf := enc.CustomBucketsHistogramSamples(customBuckets, nil)
		decHists, err := dec.HistogramSamples(histBuf, nil)
		require.NoError(t, err)
		decCustom, err := dec.HistogramSamples(customBuf, nil)
		require.NoError(t, err)
		require.Equal(t, []RefHistogramSample{input[0], input[2]}, decHists)
		require.Equal(t, input[1:2], decCustom)
	})

	// V2 float-histogram with custom bucket as first element.
	t.Run("V2 float-histogram leading custom bucket", func(t *testing.T) {
		customFH := histograms[2].H.ToFloat(nil)
		regularFH0 := histograms[0].H.ToFloat(nil)
		regularFH1 := histograms[1].H.ToFloat(nil)
		input := []RefFloatHistogramSample{
			{Ref: 67, T: 1000, ST: 500, FH: customFH},
			{Ref: 56, T: 2000, ST: 500, FH: regularFH0},
			{Ref: 42, T: 3000, ST: 600, FH: regularFH1},
		}
		floatBuf, customBuckets := enc.FloatHistogramSamples(input, nil)
		customBuf := enc.CustomBucketsFloatHistogramSamples(customBuckets, nil)
		decFloats, err := dec.FloatHistogramSamples(floatBuf, nil)
		require.NoError(t, err)
		decCustom, err := dec.FloatHistogramSamples(customBuf, nil)
		require.NoError(t, err)
		require.Equal(t, input[1:], decFloats)
		require.Equal(t, input[:1], decCustom)
	})

	// V2 float-histogram with custom bucket in the middle.
	t.Run("V2 float-histogram interleaved custom bucket", func(t *testing.T) {
		customFH := histograms[2].H.ToFloat(nil)
		regularFH0 := histograms[0].H.ToFloat(nil)
		regularFH1 := histograms[1].H.ToFloat(nil)
		input := []RefFloatHistogramSample{
			{Ref: 56, T: 1000, ST: 500, FH: regularFH0},
			{Ref: 67, T: 2000, ST: 500, FH: customFH},
			{Ref: 42, T: 3000, ST: 600, FH: regularFH1},
		}
		floatBuf, customBuckets := enc.FloatHistogramSamples(input, nil)
		customBuf := enc.CustomBucketsFloatHistogramSamples(customBuckets, nil)
		decFloats, err := dec.FloatHistogramSamples(floatBuf, nil)
		require.NoError(t, err)
		decCustom, err := dec.FloatHistogramSamples(customBuf, nil)
		require.NoError(t, err)
		require.Equal(t, []RefFloatHistogramSample{input[0], input[2]}, decFloats)
		require.Equal(t, input[1:2], decCustom)
	})
}

func TestRecord_DecodeInvalidHistogramSchema(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		for _, schema := range []int32{-100, 100} {
			t.Run(fmt.Sprintf("schema=%d,stStorage=%v", schema, enableSTStorage), func(t *testing.T) {
				enc := Encoder{EnableSTStorage: enableSTStorage}

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
}

func TestRecord_DecodeInvalidFloatHistogramSchema(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		for _, schema := range []int32{-100, 100} {
			t.Run(fmt.Sprintf("schema=%d,stStorage=%v", schema, enableSTStorage), func(t *testing.T) {
				enc := Encoder{EnableSTStorage: enableSTStorage}

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
}

func TestRecord_DecodeTooHighResolutionHistogramSchema(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		for _, schema := range []int32{9, 52} {
			t.Run(fmt.Sprintf("schema=%d,stStorage=%v", schema, enableSTStorage), func(t *testing.T) {
				enc := Encoder{EnableSTStorage: enableSTStorage}

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
}

func TestRecord_DecodeTooHighResolutionFloatHistogramSchema(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		for _, schema := range []int32{9, 52} {
			t.Run(fmt.Sprintf("schema=%d,stStorage=%v", schema, enableSTStorage), func(t *testing.T) {
				enc := Encoder{EnableSTStorage: enableSTStorage}

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
}

// TestRecord_Corrupted ensures that corrupted records return the correct error.
// Bugfix check for pull/521 and pull/523.
func TestRecord_Corrupted(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		enc := Encoder{EnableSTStorage: enableSTStorage}
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

	// With EnableSTStorage set, all Samples are V2.
	enc = Encoder{EnableSTStorage: true}
	samples = []RefSample{{Ref: 123, T: 12345, V: 1.2345}}
	recordType = dec.Type(enc.Samples(samples, nil))
	require.Equal(t, SamplesV2, recordType)

	samplesST := []RefSample{{Ref: 123, ST: 1, T: 12345, V: 1.2345}}
	recordType = dec.Type(enc.Samples(samplesST, nil))
	require.Equal(t, SamplesV2, recordType)

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
	// V1 histogram type recognition (requires EnableSTStorage off).
	enc = Encoder{}
	hists, customBucketsHistograms := enc.HistogramSamples(histograms, nil)
	recordType = dec.Type(hists)
	require.Equal(t, HistogramSamples, recordType)
	customBucketsHists := enc.CustomBucketsHistogramSamples(customBucketsHistograms, nil)
	recordType = dec.Type(customBucketsHists)
	require.Equal(t, CustomBucketsHistogramSamples, recordType)

	// V2 histogram type recognition.
	enc = Encoder{EnableSTStorage: true}
	hists, customBucketsHistograms = enc.HistogramSamples(histograms, nil)
	recordType = dec.Type(hists)
	require.Equal(t, HistogramSamplesV2, recordType)
	customBucketsHists = enc.CustomBucketsHistogramSamples(customBucketsHistograms, nil)
	recordType = dec.Type(customBucketsHists)
	require.Equal(t, CustomBucketsHistogramSamplesV2, recordType)

	// V2 float-histogram type recognition.
	floatHistograms := make([]RefFloatHistogramSample, len(histograms))
	for i, h := range histograms {
		floatHistograms[i] = RefFloatHistogramSample{
			Ref: h.Ref,
			T:   h.T,
			FH:  h.H.ToFloat(nil),
		}
	}
	floatHists, customBucketsFloatHistograms := enc.FloatHistogramSamples(floatHistograms, nil)
	recordType = dec.Type(floatHists)
	require.Equal(t, FloatHistogramSamplesV2, recordType)
	customBucketsFloatHists := enc.CustomBucketsFloatHistogramSamples(customBucketsFloatHistograms, nil)
	recordType = dec.Type(customBucketsFloatHists)
	require.Equal(t, CustomBucketsFloatHistogramSamplesV2, recordType)

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
		for _, enableSTStorage := range []bool{false, true} {
			for _, labelCount := range []int{0, 10, 50} {
				for _, histograms := range []int{10, 100, 1000} {
					for _, buckets := range []int{0, 1, 10, 100} {
						b.Run(fmt.Sprintf("type=%s/labels=%d/histograms=%d/buckets=%d", maker.name, labelCount, histograms, buckets), func(b *testing.B) {
							series, samples, nhcbs := maker.make(labelCount, histograms, buckets)
							enc := Encoder{EnableSTStorage: enableSTStorage}
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
}
