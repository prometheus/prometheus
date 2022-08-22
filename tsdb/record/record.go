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

// Package record contains the various record types used for encoding various Head block data in the WAL and in-memory snapshot.
package record

import (
	"math"
	"sort"

	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
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
	// Exemplars is used to match WAL records of type Exemplars.
	Exemplars Type = 4
	// Metadata is used to match WAL records of type Metadata.
	Metadata Type = 6
)

func (rt Type) String() string {
	switch rt {
	case Series:
		return "series"
	case Samples:
		return "samples"
	case Exemplars:
		return "exemplars"
	case Tombstones:
		return "tombstones"
	case Metadata:
		return "metadata"
	default:
		return "unknown"
	}
}

// MetricType represents the type of a series.
type MetricType uint8

const (
	UnknownMT      MetricType = 0
	Counter        MetricType = 1
	Gauge          MetricType = 2
	Histogram      MetricType = 3
	GaugeHistogram MetricType = 4
	Summary        MetricType = 5
	Info           MetricType = 6
	Stateset       MetricType = 7
)

func GetMetricType(t textparse.MetricType) uint8 {
	switch t {
	case textparse.MetricTypeCounter:
		return uint8(Counter)
	case textparse.MetricTypeGauge:
		return uint8(Gauge)
	case textparse.MetricTypeHistogram:
		return uint8(Histogram)
	case textparse.MetricTypeGaugeHistogram:
		return uint8(GaugeHistogram)
	case textparse.MetricTypeSummary:
		return uint8(Summary)
	case textparse.MetricTypeInfo:
		return uint8(Info)
	case textparse.MetricTypeStateset:
		return uint8(Stateset)
	default:
		return uint8(UnknownMT)
	}
}

func ToTextparseMetricType(m uint8) textparse.MetricType {
	switch m {
	case uint8(Counter):
		return textparse.MetricTypeCounter
	case uint8(Gauge):
		return textparse.MetricTypeGauge
	case uint8(Histogram):
		return textparse.MetricTypeHistogram
	case uint8(GaugeHistogram):
		return textparse.MetricTypeGaugeHistogram
	case uint8(Summary):
		return textparse.MetricTypeSummary
	case uint8(Info):
		return textparse.MetricTypeInfo
	case uint8(Stateset):
		return textparse.MetricTypeStateset
	default:
		return textparse.MetricTypeUnknown
	}
}

const (
	unitMetaName = "UNIT"
	helpMetaName = "HELP"
)

// ErrNotFound is returned if a looked up resource was not found. Duplicate ErrNotFound from head.go.
var ErrNotFound = errors.New("not found")

// RefSeries is the series labels with the series ID.
type RefSeries struct {
	Ref    chunks.HeadSeriesRef
	Labels labels.Labels
}

// RefSample is a timestamp/value pair associated with a reference to a series.
type RefSample struct {
	Ref chunks.HeadSeriesRef
	T   int64
	V   float64
}

// RefMetadata is the metadata associated with a series ID.
type RefMetadata struct {
	Ref  chunks.HeadSeriesRef
	Type uint8
	Unit string
	Help string
}

// RefExemplar is an exemplar with it's labels, timestamp, value the exemplar was collected/observed with, and a reference to a series.
type RefExemplar struct {
	Ref    chunks.HeadSeriesRef
	T      int64
	V      float64
	Labels labels.Labels
}

// Decoder decodes series, sample, metadata and tombstone records.
// The zero value is ready to use.
type Decoder struct{}

// Type returns the type of the record.
// Returns RecordUnknown if no valid record type is found.
func (d *Decoder) Type(rec []byte) Type {
	if len(rec) < 1 {
		return Unknown
	}
	switch t := Type(rec[0]); t {
	case Series, Samples, Tombstones, Exemplars, Metadata:
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
		ref := storage.SeriesRef(dec.Be64())
		lset := d.DecodeLabels(&dec)

		series = append(series, RefSeries{
			Ref:    chunks.HeadSeriesRef(ref),
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
		typ := dec.Byte()
		numFields := dec.Uvarint()

		// We're currently aware of two more metadata fields other than TYPE; that is UNIT and HELP.
		// We can skip the rest of the fields (if we encounter any), but we must decode them anyway
		// so we can correctly align with the start with the next metadata record.
		var unit, help string
		for i := 0; i < numFields; i++ {
			fieldName := dec.UvarintStr()
			fieldValue := dec.UvarintStr()
			switch fieldName {
			case unitMetaName:
				unit = fieldValue
			case helpMetaName:
				help = fieldValue
			}
		}

		metadata = append(metadata, RefMetadata{
			Ref:  chunks.HeadSeriesRef(ref),
			Type: typ,
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

// DecodeLabels decodes one set of labels from buf.
func (d *Decoder) DecodeLabels(dec *encoding.Decbuf) labels.Labels {
	lset := make(labels.Labels, dec.Uvarint())

	for i := range lset {
		lset[i].Name = dec.UvarintStr()
		lset[i].Value = dec.UvarintStr()
	}
	sort.Sort(lset)
	return lset
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
			Ref: chunks.HeadSeriesRef(int64(baseRef) + dref),
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
			Ref: storage.SeriesRef(dec.Be64()),
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

func (d *Decoder) Exemplars(rec []byte, exemplars []RefExemplar) ([]RefExemplar, error) {
	dec := encoding.Decbuf{B: rec}
	t := Type(dec.Byte())
	if t != Exemplars {
		return nil, errors.New("invalid record type")
	}

	return d.ExemplarsFromBuffer(&dec, exemplars)
}

func (d *Decoder) ExemplarsFromBuffer(dec *encoding.Decbuf, exemplars []RefExemplar) ([]RefExemplar, error) {
	if dec.Len() == 0 {
		return exemplars, nil
	}
	var (
		baseRef  = dec.Be64()
		baseTime = dec.Be64int64()
	)
	for len(dec.B) > 0 && dec.Err() == nil {
		dref := dec.Varint64()
		dtime := dec.Varint64()
		val := dec.Be64()
		lset := d.DecodeLabels(dec)

		exemplars = append(exemplars, RefExemplar{
			Ref:    chunks.HeadSeriesRef(baseRef + uint64(dref)),
			T:      baseTime + dtime,
			V:      math.Float64frombits(val),
			Labels: lset,
		})
	}

	if dec.Err() != nil {
		return nil, errors.Wrapf(dec.Err(), "decode error after %d exemplars", len(exemplars))
	}
	if len(dec.B) > 0 {
		return nil, errors.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return exemplars, nil
}

// Encoder encodes series, sample, and tombstones records.
// The zero value is ready to use.
type Encoder struct{}

// Series appends the encoded series to b and returns the resulting slice.
func (e *Encoder) Series(series []RefSeries, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(Series))

	for _, s := range series {
		buf.PutBE64(uint64(s.Ref))
		EncodeLabels(&buf, s.Labels)
	}
	return buf.Get()
}

// Metadata appends the encoded metadata to b and returns the resulting slice.
func (e *Encoder) Metadata(metadata []RefMetadata, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(Metadata))

	for _, m := range metadata {
		buf.PutUvarint64(uint64(m.Ref))

		buf.PutByte(m.Type)

		buf.PutUvarint(2) // num_fields: We currently have two more metadata fields, UNIT and HELP.
		buf.PutUvarintStr(unitMetaName)
		buf.PutUvarintStr(m.Unit)
		buf.PutUvarintStr(helpMetaName)
		buf.PutUvarintStr(m.Help)
	}

	return buf.Get()
}

// EncodeLabels encodes the contents of labels into buf.
func EncodeLabels(buf *encoding.Encbuf, lbls labels.Labels) {
	buf.PutUvarint(len(lbls))

	for _, l := range lbls {
		buf.PutUvarintStr(l.Name)
		buf.PutUvarintStr(l.Value)
	}
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

	buf.PutBE64(uint64(first.Ref))
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
			buf.PutBE64(uint64(s.Ref))
			buf.PutVarint64(iv.Mint)
			buf.PutVarint64(iv.Maxt)
		}
	}
	return buf.Get()
}

func (e *Encoder) Exemplars(exemplars []RefExemplar, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(Exemplars))

	if len(exemplars) == 0 {
		return buf.Get()
	}

	e.EncodeExemplarsIntoBuffer(exemplars, &buf)

	return buf.Get()
}

func (e *Encoder) EncodeExemplarsIntoBuffer(exemplars []RefExemplar, buf *encoding.Encbuf) {
	// Store base timestamp and base reference number of first sample.
	// All samples encode their timestamp and ref as delta to those.
	first := exemplars[0]

	buf.PutBE64(uint64(first.Ref))
	buf.PutBE64int64(first.T)

	for _, ex := range exemplars {
		buf.PutVarint64(int64(ex.Ref) - int64(first.Ref))
		buf.PutVarint64(ex.T - first.T)
		buf.PutBE64(math.Float64bits(ex.V))
		EncodeLabels(buf, ex.Labels)
	}
}
