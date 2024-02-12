// Copyright 2018 The Prometheus Authors

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
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestRecord_EncodeDecode(t *testing.T) {
	var enc Encoder
	var dec Decoder

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
		{Ref: 0, T: 12423423, V: 1.2345, Labels: labels.FromStrings("traceID", "qwerty")},
		{Ref: 123, T: -1231, V: -123, Labels: labels.FromStrings("traceID", "asdf")},
		{Ref: 2, T: 0, V: 99999, Labels: labels.FromStrings("traceID", "zxcv")},
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
	}

	decHistograms, err := dec.HistogramSamples(enc.HistogramSamples(histograms, nil), nil)
	require.NoError(t, err)
	require.Equal(t, histograms, decHistograms)

	floatHistograms := make([]RefFloatHistogramSample, len(histograms))
	for i, h := range histograms {
		floatHistograms[i] = RefFloatHistogramSample{
			Ref: h.Ref,
			T:   h.T,
			FH:  h.H.ToFloat(nil),
		}
	}
	decFloatHistograms, err := dec.FloatHistogramSamples(enc.FloatHistogramSamples(floatHistograms, nil), nil)
	require.NoError(t, err)
	require.Equal(t, floatHistograms, decFloatHistograms)

	// Gauge ingeger histograms.
	for i := range histograms {
		histograms[i].H.CounterResetHint = histogram.GaugeType
	}
	decHistograms, err = dec.HistogramSamples(enc.HistogramSamples(histograms, nil), nil)
	require.NoError(t, err)
	require.Equal(t, histograms, decHistograms)

	// Gauge float histograms.
	for i := range floatHistograms {
		floatHistograms[i].FH.CounterResetHint = histogram.GaugeType
	}
	decFloatHistograms, err = dec.FloatHistogramSamples(enc.FloatHistogramSamples(floatHistograms, nil), nil)
	require.NoError(t, err)
	require.Equal(t, floatHistograms, decFloatHistograms)
}

// TestRecord_Corrupted ensures that corrupted records return the correct error.
// Bugfix check for pull/521 and pull/523.
func TestRecord_Corrupted(t *testing.T) {
	var enc Encoder
	var dec Decoder

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
			{Ref: 0, T: 12423423, V: 1.2345, Labels: labels.FromStrings("traceID", "asdf")},
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
		}

		corrupted := enc.HistogramSamples(histograms, nil)[:8]
		_, err := dec.HistogramSamples(corrupted, nil)
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
	}
	recordType = dec.Type(enc.HistogramSamples(histograms, nil))
	require.Equal(t, HistogramSamples, recordType)

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
