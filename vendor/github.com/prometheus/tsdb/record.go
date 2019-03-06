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

package tsdb

import (
	"math"
	"sort"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/encoding"
	"github.com/prometheus/tsdb/labels"
)

// RecordType represents the data type of a record.
type RecordType uint8

const (
	// RecordInvalid is returned for unrecognised WAL record types.
	RecordInvalid RecordType = 255
	// RecordSeries is used to match WAL records of type Series.
	RecordSeries RecordType = 1
	// RecordSamples is used to match WAL records of type Samples.
	RecordSamples RecordType = 2
	// RecordTombstones is used to match WAL records of type Tombstones.
	RecordTombstones RecordType = 3
)

// RecordDecoder decodes series, sample, and tombstone records.
// The zero value is ready to use.
type RecordDecoder struct {
}

// Type returns the type of the record.
// Return RecordInvalid if no valid record type is found.
func (d *RecordDecoder) Type(rec []byte) RecordType {
	if len(rec) < 1 {
		return RecordInvalid
	}
	switch t := RecordType(rec[0]); t {
	case RecordSeries, RecordSamples, RecordTombstones:
		return t
	}
	return RecordInvalid
}

// Series appends series in rec to the given slice.
func (d *RecordDecoder) Series(rec []byte, series []RefSeries) ([]RefSeries, error) {
	dec := encoding.Decbuf{B: rec}

	if RecordType(dec.Byte()) != RecordSeries {
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

// Samples appends samples in rec to the given slice.
func (d *RecordDecoder) Samples(rec []byte, samples []RefSample) ([]RefSample, error) {
	dec := encoding.Decbuf{B: rec}

	if RecordType(dec.Byte()) != RecordSamples {
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
func (d *RecordDecoder) Tombstones(rec []byte, tstones []Stone) ([]Stone, error) {
	dec := encoding.Decbuf{B: rec}

	if RecordType(dec.Byte()) != RecordTombstones {
		return nil, errors.New("invalid record type")
	}
	for dec.Len() > 0 && dec.Err() == nil {
		tstones = append(tstones, Stone{
			ref: dec.Be64(),
			intervals: Intervals{
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

// RecordEncoder encodes series, sample, and tombstones records.
// The zero value is ready to use.
type RecordEncoder struct {
}

// Series appends the encoded series to b and returns the resulting slice.
func (e *RecordEncoder) Series(series []RefSeries, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(RecordSeries))

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

// Samples appends the encoded samples to b and returns the resulting slice.
func (e *RecordEncoder) Samples(samples []RefSample, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(RecordSamples))

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
func (e *RecordEncoder) Tombstones(tstones []Stone, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(RecordTombstones))

	for _, s := range tstones {
		for _, iv := range s.intervals {
			buf.PutBE64(s.ref)
			buf.PutVarint64(iv.Mint)
			buf.PutVarint64(iv.Maxt)
		}
	}
	return buf.Get()
}
