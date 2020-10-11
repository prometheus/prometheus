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
	"math"
	"sort"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

// Type represents the data type of a record.
type Type uint8

const (
	// Unknown is returned for unrecognised WAL record types.
	Unknown Type = 255
	// Series is used to match WAL records of type Series.
	Series Type = 1
	// Samples is used to match WAL records of type Samples.
	Samples Type = 2
	// Tombstones is used to match WAL records of type Tombstones.
	Tombstones Type = 3
	// Metadata is used to match WAL records of type Metadata.
	Metadata Type = 4
)

var (
	// ErrNotFound is returned if a looked up resource was not found. Duplicate ErrNotFound from head.go.
	ErrNotFound = errors.New("not found")
)

// RefSeries is the series labels with the series ID.
type RefSeries struct {
	Ref    uint64
	Labels labels.Labels
}

// RefMetadata is the series metadata.
type RefMetadata struct {
	Ref  uint64
	Type textparse.MetricType
	Unit string
	Help string
}

// RefSample is a timestamp/value pair associated with a reference to a series.
type RefSample struct {
	Ref uint64
	T   int64
	V   float64
}

// Decoder decodes series, sample, and tombstone records.
// The zero value is ready to use.
type Decoder struct {
}

// Type returns the type of the record.
// Returns RecordUnknown if no valid record type is found.
func (d *Decoder) Type(rec []byte) Type {
	if len(rec) < 1 {
		return Unknown
	}
	switch t := Type(rec[0]); t {
	case Series, Samples, Tombstones, Metadata:
		return t
	}
	return Unknown
}

// Series appends series in rec to the given slice.
func (d *Decoder) Series(rec []byte, series []RefSeries) ([]RefSeries, error) {
	dec := encoding.Decbuf{B: rec}

	if Type(dec.Byte()) != Series {
		return nil, errors.New("invalid record type")
	}
	for len(dec.B) > 0 && dec.Err() == nil {
		ref := dec.Be64()

		lset := make(labels.Labels, dec.Uvarint())

		for i := range lset {
			lset[i].Name = dec.UvarintStr()
			lset[i].Value = dec.UvarintStr()
		}
		sort.Sort(lset)

		series = append(series, RefSeries{
			Ref:    ref,
			Labels: lset,
		})
	}
	if dec.Err() != nil {
		return nil, dec.Err()
	}
	if len(dec.B) > 0 {
		return nil, errors.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return series, nil
}

// Metadata appends metadata in rec to the given slice.
func (d *Decoder) Metadata(rec []byte, metadata []RefMetadata) ([]RefMetadata, error) {
	dec := encoding.Decbuf{B: rec}

	if Type(dec.Byte()) != Metadata {
		return nil, errors.New("invalid record type")
	}
	for len(dec.B) > 0 && dec.Err() == nil {
		ref := dec.Uvarint64()
		size := dec.Uvarint()

		remainingBeforeReadFields := dec.Len()

		typ := dec.UvarintStr()
		unit := dec.UvarintStr()
		help := dec.UvarintStr()

		// The bytes consumed will have shrunk, therefore delta between
		// bytes unread before and bytes unread after is how much we consumed.
		remainingAfterReadFields := dec.Len()
		sizeFieldsRead := remainingBeforeReadFields - remainingAfterReadFields
		if sizeFieldsUnread := size - sizeFieldsRead; sizeFieldsUnread > 0 {
			// Need to skip fields that we didn't read and don't know about.
			dec.Skip(sizeFieldsUnread)
		}

		metadata = append(metadata, RefMetadata{
			Ref:  ref,
			Type: textparse.MetricType(typ),
			Unit: unit,
			Help: help,
		})
	}
	if dec.Err() != nil {
		return nil, dec.Err()
	}
	if len(dec.B) > 0 {
		return nil, errors.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return metadata, nil
}

// Samples appends samples in rec to the given slice.
func (d *Decoder) Samples(rec []byte, samples []RefSample) ([]RefSample, error) {
	dec := encoding.Decbuf{B: rec}

	if Type(dec.Byte()) != Samples {
		return nil, errors.New("invalid record type")
	}
	if dec.Len() == 0 {
		return samples, nil
	}
	var (
		baseRef  = dec.Be64()
		baseTime = dec.Be64int64()
	)
	for len(dec.B) > 0 && dec.Err() == nil {
		dref := dec.Varint64()
		dtime := dec.Varint64()
		val := dec.Be64()

		samples = append(samples, RefSample{
			Ref: uint64(int64(baseRef) + dref),
			T:   baseTime + dtime,
			V:   math.Float64frombits(val),
		})
	}

	if dec.Err() != nil {
		return nil, errors.Wrapf(dec.Err(), "decode error after %d samples", len(samples))
	}
	if len(dec.B) > 0 {
		return nil, errors.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return samples, nil
}

// Tombstones appends tombstones in rec to the given slice.
func (d *Decoder) Tombstones(rec []byte, tstones []tombstones.Stone) ([]tombstones.Stone, error) {
	dec := encoding.Decbuf{B: rec}

	if Type(dec.Byte()) != Tombstones {
		return nil, errors.New("invalid record type")
	}
	for dec.Len() > 0 && dec.Err() == nil {
		tstones = append(tstones, tombstones.Stone{
			Ref: dec.Be64(),
			Intervals: tombstones.Intervals{
				{Mint: dec.Varint64(), Maxt: dec.Varint64()},
			},
		})
	}
	if dec.Err() != nil {
		return nil, dec.Err()
	}
	if len(dec.B) > 0 {
		return nil, errors.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return tstones, nil
}

// Encoder encodes series, sample, and tombstones records.
// The zero value is ready to use.
type Encoder struct {
}

// Series appends the encoded series to b and returns the resulting slice.
func (e *Encoder) Series(series []RefSeries, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(Series))

	for _, s := range series {
		buf.PutBE64(s.Ref)
		buf.PutUvarint(len(s.Labels))

		for _, l := range s.Labels {
			buf.PutUvarintStr(l.Name)
			buf.PutUvarintStr(l.Value)
		}
	}

	return buf.Get()
}

// Metadata encodes series metadata.
func (e *Encoder) Metadata(series []RefMetadata, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(Metadata))

	for _, s := range series {
		buf.PutUvarint64(s.Ref)

		sizeAfterSeriesRef := buf.Len()

		// Reserve space to write the size of the metadata fields
		// so once we have encoded the fields, we can rewind, write the size
		// then copy the encoded fields backwards. This allows for the
		// buffer to be used both for staging the encoding of the
		// fields, then also moving them into place once size is known
		// and written to the stream.
		buf.PutBE64(math.MaxUint64)

		sizeBeforeEncodeFields := buf.Len()
		buf.PutUvarintStr(string(s.Type))
		buf.PutUvarintStr(string(s.Unit))
		buf.PutUvarintStr(string(s.Help))
		sizeAfterEncodeFields := buf.Len()
		sizeFieldsWritten := sizeAfterEncodeFields - sizeBeforeEncodeFields

		// Take slice of bytes for encoded fields.
		b := buf.B
		encodedFields := b[sizeBeforeEncodeFields:sizeAfterEncodeFields]

		// Rewind the buffer to before any metadata was written and
		// then encode the size, then copy back the metadata fields into place.
		buf.B = b[:sizeAfterSeriesRef]
		// PutUvarint will always use less space than writing fixed size
		// big endian uint64 that we wrote to the stream to reserve space,
		// thus not overwriting the byte buffer with the encoded fields content.
		buf.PutUvarint(sizeFieldsWritten)

		// Copy back the encoded fields into place.
		buf.B = append(buf.B, encodedFields...)
	}

	return buf.Get()
}

// Samples appends the encoded samples to b and returns the resulting slice.
func (e *Encoder) Samples(samples []RefSample, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(Samples))

	if len(samples) == 0 {
		return buf.Get()
	}

	// Store base timestamp and base reference number of first sample.
	// All samples encode their timestamp and ref as delta to those.
	first := samples[0]

	buf.PutBE64(first.Ref)
	buf.PutBE64int64(first.T)

	for _, s := range samples {
		buf.PutVarint64(int64(s.Ref) - int64(first.Ref))
		buf.PutVarint64(s.T - first.T)
		buf.PutBE64(math.Float64bits(s.V))
	}
	return buf.Get()
}

// Tombstones appends the encoded tombstones to b and returns the resulting slice.
func (e *Encoder) Tombstones(tstones []tombstones.Stone, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(Tombstones))

	for _, s := range tstones {
		for _, iv := range s.Intervals {
			buf.PutBE64(s.Ref)
			buf.PutVarint64(iv.Mint)
			buf.PutVarint64(iv.Maxt)
		}
	}
	return buf.Get()
}
