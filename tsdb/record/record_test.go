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
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/tombstones"
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
	require.Equal(t, series, decSeries)

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
	require.Equal(t, exemplars, decExemplars)
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
		require.Equal(t, errors.Cause(err), encoding.ErrInvalidSize)
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
		require.Equal(t, errors.Cause(err), encoding.ErrInvalidSize)
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

	recordType = dec.Type(nil)
	require.Equal(t, Unknown, recordType)

	recordType = dec.Type([]byte{0})
	require.Equal(t, Unknown, recordType)
}

func TestRecord_MetadataDecodeUnknownFields(t *testing.T) {
	var enc encoding.Encbuf
	var encFields encoding.Encbuf
	var dec Decoder

	// Write record type.
	enc.PutByte(byte(Metadata))

	// Write first metadata entry, all known fields.
	enc.PutUvarint64(101)
	encFields.Reset()
	encFields.PutByte(byte(Counter))
	encFields.PutUvarintStr("")
	encFields.PutUvarintStr("some magic counter")
	// Write size of encoded fields then the encoded fields.
	enc.PutUvarint(encFields.Len())
	enc.B = append(enc.B, encFields.B...)

	// Write second metadata entry, known fields + unknown fields.
	enc.PutUvarint64(99)
	encFields.Reset()
	// Known fields.
	encFields.PutByte(byte(Counter))
	encFields.PutUvarintStr("seconds")
	encFields.PutUvarintStr("CPU time counter")
	// Unknown fields.
	encFields.PutUvarintStr("some unknown field")
	encFields.PutByte('a')
	encFields.PutUvarint64(42)
	// Write size of encoded fields then the encoded fields.
	enc.PutUvarint(encFields.Len())
	enc.B = append(enc.B, encFields.B...)

	// Write third metadata entry, all known fields.
	enc.PutUvarint64(47250)
	encFields.Reset()
	encFields.PutByte(byte(Gauge))
	encFields.PutUvarintStr("percentage")
	encFields.PutUvarintStr("current memory usage")
	// Write size of encoded fields then the encoded fields.
	enc.PutUvarint(encFields.Len())
	enc.B = append(enc.B, encFields.B...)

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
