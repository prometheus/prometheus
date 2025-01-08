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
	"errors"
	"fmt"
	"math"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
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
	// MmapMarkers is used to match OOO WBL records of type MmapMarkers.
	MmapMarkers Type = 5
	// Metadata is used to match WAL records of type Metadata.
	Metadata Type = 6
	// HistogramSamples is used to match WAL records of type Histograms.
	HistogramSamples Type = 7
	// FloatHistogramSamples is used to match WAL records of type Float Histograms.
	FloatHistogramSamples Type = 8
	// CustomBucketsHistogramSamples is used to match WAL records of type Histogram with custom buckets.
	CustomBucketsHistogramSamples Type = 9
	// CustomBucketsFloatHistogramSamples is used to match WAL records of type Float Histogram with custom buckets.
	CustomBucketsFloatHistogramSamples Type = 10
)

func (rt Type) String() string {
	switch rt {
	case Series:
		return "series"
	case Samples:
		return "samples"
	case Tombstones:
		return "tombstones"
	case Exemplars:
		return "exemplars"
	case HistogramSamples:
		return "histogram_samples"
	case FloatHistogramSamples:
		return "float_histogram_samples"
	case CustomBucketsHistogramSamples:
		return "custom_buckets_histogram_samples"
	case CustomBucketsFloatHistogramSamples:
		return "custom_buckets_float_histogram_samples"
	case MmapMarkers:
		return "mmapmarkers"
	case Metadata:
		return "metadata"
	default:
		return "unknown"
	}
}

// MetricType represents the type of a series.
type MetricType uint8

const (
	UnknownMT       MetricType = 0
	Counter         MetricType = 1
	Gauge           MetricType = 2
	HistogramSample MetricType = 3
	GaugeHistogram  MetricType = 4
	Summary         MetricType = 5
	Info            MetricType = 6
	Stateset        MetricType = 7
)

func GetMetricType(t model.MetricType) uint8 {
	switch t {
	case model.MetricTypeCounter:
		return uint8(Counter)
	case model.MetricTypeGauge:
		return uint8(Gauge)
	case model.MetricTypeHistogram:
		return uint8(HistogramSample)
	case model.MetricTypeGaugeHistogram:
		return uint8(GaugeHistogram)
	case model.MetricTypeSummary:
		return uint8(Summary)
	case model.MetricTypeInfo:
		return uint8(Info)
	case model.MetricTypeStateset:
		return uint8(Stateset)
	default:
		return uint8(UnknownMT)
	}
}

func ToMetricType(m uint8) model.MetricType {
	switch m {
	case uint8(Counter):
		return model.MetricTypeCounter
	case uint8(Gauge):
		return model.MetricTypeGauge
	case uint8(HistogramSample):
		return model.MetricTypeHistogram
	case uint8(GaugeHistogram):
		return model.MetricTypeGaugeHistogram
	case uint8(Summary):
		return model.MetricTypeSummary
	case uint8(Info):
		return model.MetricTypeInfo
	case uint8(Stateset):
		return model.MetricTypeStateset
	default:
		return model.MetricTypeUnknown
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
// TODO(beorn7): Perhaps make this "polymorphic", including histogram and float-histogram pointers? Then get rid of RefHistogramSample.
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

// RefExemplar is an exemplar with the labels, timestamp, value the exemplar was collected/observed with, and a reference to a series.
type RefExemplar struct {
	Ref    chunks.HeadSeriesRef
	T      int64
	V      float64
	Labels labels.Labels
}

// RefHistogramSample is a histogram.
type RefHistogramSample struct {
	Ref chunks.HeadSeriesRef
	T   int64
	H   *histogram.Histogram
}

// RefFloatHistogramSample is a float histogram.
type RefFloatHistogramSample struct {
	Ref chunks.HeadSeriesRef
	T   int64
	FH  *histogram.FloatHistogram
}

// RefMmapMarker marks that the all the samples of the given series until now have been m-mapped to disk.
type RefMmapMarker struct {
	Ref     chunks.HeadSeriesRef
	MmapRef chunks.ChunkDiskMapperRef
}

// Decoder decodes series, sample, metadata and tombstone records.
type Decoder struct {
	builder labels.ScratchBuilder
}

func NewDecoder(t *labels.SymbolTable) Decoder { // FIXME remove t
	return Decoder{builder: labels.NewScratchBuilder(0)}
}

// Type returns the type of the record.
// Returns RecordUnknown if no valid record type is found.
func (d *Decoder) Type(rec []byte) Type {
	if len(rec) < 1 {
		return Unknown
	}
	switch t := Type(rec[0]); t {
	case Series, Samples, Tombstones, Exemplars, MmapMarkers, Metadata, HistogramSamples, FloatHistogramSamples, CustomBucketsHistogramSamples, CustomBucketsFloatHistogramSamples:
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
		return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
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
		return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return metadata, nil
}

// DecodeLabels decodes one set of labels from buf.
func (d *Decoder) DecodeLabels(dec *encoding.Decbuf) labels.Labels {
	d.builder.Reset()
	nLabels := dec.Uvarint()
	for i := 0; i < nLabels; i++ {
		lName := dec.UvarintBytes()
		lValue := dec.UvarintBytes()
		d.builder.UnsafeAddBytes(lName, lValue)
	}
	return d.builder.Labels()
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
	// Allow 1 byte for each varint and 8 for the value; the output slice must be at least that big.
	if minSize := dec.Len() / (1 + 1 + 8); cap(samples) < minSize {
		samples = make([]RefSample, 0, minSize)
	}
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
		return nil, fmt.Errorf("decode error after %d samples: %w", len(samples), dec.Err())
	}
	if len(dec.B) > 0 {
		return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
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
		return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
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
		return nil, fmt.Errorf("decode error after %d exemplars: %w", len(exemplars), dec.Err())
	}
	if len(dec.B) > 0 {
		return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return exemplars, nil
}

func (d *Decoder) MmapMarkers(rec []byte, markers []RefMmapMarker) ([]RefMmapMarker, error) {
	dec := encoding.Decbuf{B: rec}
	t := Type(dec.Byte())
	if t != MmapMarkers {
		return nil, errors.New("invalid record type")
	}

	if dec.Len() == 0 {
		return markers, nil
	}
	for len(dec.B) > 0 && dec.Err() == nil {
		ref := chunks.HeadSeriesRef(dec.Be64())
		mmapRef := chunks.ChunkDiskMapperRef(dec.Be64())
		markers = append(markers, RefMmapMarker{
			Ref:     ref,
			MmapRef: mmapRef,
		})
	}

	if dec.Err() != nil {
		return nil, fmt.Errorf("decode error after %d mmap markers: %w", len(markers), dec.Err())
	}
	if len(dec.B) > 0 {
		return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return markers, nil
}

func (d *Decoder) HistogramSamples(rec []byte, histograms []RefHistogramSample) ([]RefHistogramSample, error) {
	dec := encoding.Decbuf{B: rec}
	t := Type(dec.Byte())
	if t != HistogramSamples && t != CustomBucketsHistogramSamples {
		return nil, errors.New("invalid record type")
	}
	if dec.Len() == 0 {
		return histograms, nil
	}
	var (
		baseRef  = dec.Be64()
		baseTime = dec.Be64int64()
	)
	for len(dec.B) > 0 && dec.Err() == nil {
		dref := dec.Varint64()
		dtime := dec.Varint64()

		rh := RefHistogramSample{
			Ref: chunks.HeadSeriesRef(baseRef + uint64(dref)),
			T:   baseTime + dtime,
			H:   &histogram.Histogram{},
		}

		DecodeHistogram(&dec, rh.H)
		histograms = append(histograms, rh)
	}

	if dec.Err() != nil {
		return nil, fmt.Errorf("decode error after %d histograms: %w", len(histograms), dec.Err())
	}
	if len(dec.B) > 0 {
		return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return histograms, nil
}

// DecodeHistogram decodes a Histogram from a byte slice.
func DecodeHistogram(buf *encoding.Decbuf, h *histogram.Histogram) {
	h.CounterResetHint = histogram.CounterResetHint(buf.Byte())

	h.Schema = int32(buf.Varint64())
	h.ZeroThreshold = math.Float64frombits(buf.Be64())

	h.ZeroCount = buf.Uvarint64()
	h.Count = buf.Uvarint64()
	h.Sum = math.Float64frombits(buf.Be64())

	l := buf.Uvarint()
	if l > 0 {
		h.PositiveSpans = make([]histogram.Span, l)
	}
	for i := range h.PositiveSpans {
		h.PositiveSpans[i].Offset = int32(buf.Varint64())
		h.PositiveSpans[i].Length = buf.Uvarint32()
	}

	l = buf.Uvarint()
	if l > 0 {
		h.NegativeSpans = make([]histogram.Span, l)
	}
	for i := range h.NegativeSpans {
		h.NegativeSpans[i].Offset = int32(buf.Varint64())
		h.NegativeSpans[i].Length = buf.Uvarint32()
	}

	l = buf.Uvarint()
	if l > 0 {
		h.PositiveBuckets = make([]int64, l)
	}
	for i := range h.PositiveBuckets {
		h.PositiveBuckets[i] = buf.Varint64()
	}

	l = buf.Uvarint()
	if l > 0 {
		h.NegativeBuckets = make([]int64, l)
	}
	for i := range h.NegativeBuckets {
		h.NegativeBuckets[i] = buf.Varint64()
	}

	if histogram.IsCustomBucketsSchema(h.Schema) {
		l = buf.Uvarint()
		if l > 0 {
			h.CustomValues = make([]float64, l)
		}
		for i := range h.CustomValues {
			h.CustomValues[i] = buf.Be64Float64()
		}
	}
}

func (d *Decoder) FloatHistogramSamples(rec []byte, histograms []RefFloatHistogramSample) ([]RefFloatHistogramSample, error) {
	dec := encoding.Decbuf{B: rec}
	t := Type(dec.Byte())
	if t != FloatHistogramSamples && t != CustomBucketsFloatHistogramSamples {
		return nil, errors.New("invalid record type")
	}
	if dec.Len() == 0 {
		return histograms, nil
	}
	var (
		baseRef  = dec.Be64()
		baseTime = dec.Be64int64()
	)
	for len(dec.B) > 0 && dec.Err() == nil {
		dref := dec.Varint64()
		dtime := dec.Varint64()

		rh := RefFloatHistogramSample{
			Ref: chunks.HeadSeriesRef(baseRef + uint64(dref)),
			T:   baseTime + dtime,
			FH:  &histogram.FloatHistogram{},
		}

		DecodeFloatHistogram(&dec, rh.FH)
		histograms = append(histograms, rh)
	}

	if dec.Err() != nil {
		return nil, fmt.Errorf("decode error after %d histograms: %w", len(histograms), dec.Err())
	}
	if len(dec.B) > 0 {
		return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}
	return histograms, nil
}

// DecodeFloatHistogram decodes a Histogram from a byte slice.
func DecodeFloatHistogram(buf *encoding.Decbuf, fh *histogram.FloatHistogram) {
	fh.CounterResetHint = histogram.CounterResetHint(buf.Byte())

	fh.Schema = int32(buf.Varint64())
	fh.ZeroThreshold = buf.Be64Float64()

	fh.ZeroCount = buf.Be64Float64()
	fh.Count = buf.Be64Float64()
	fh.Sum = buf.Be64Float64()

	l := buf.Uvarint()
	if l > 0 {
		fh.PositiveSpans = make([]histogram.Span, l)
	}
	for i := range fh.PositiveSpans {
		fh.PositiveSpans[i].Offset = int32(buf.Varint64())
		fh.PositiveSpans[i].Length = buf.Uvarint32()
	}

	l = buf.Uvarint()
	if l > 0 {
		fh.NegativeSpans = make([]histogram.Span, l)
	}
	for i := range fh.NegativeSpans {
		fh.NegativeSpans[i].Offset = int32(buf.Varint64())
		fh.NegativeSpans[i].Length = buf.Uvarint32()
	}

	l = buf.Uvarint()
	if l > 0 {
		fh.PositiveBuckets = make([]float64, l)
	}
	for i := range fh.PositiveBuckets {
		fh.PositiveBuckets[i] = buf.Be64Float64()
	}

	l = buf.Uvarint()
	if l > 0 {
		fh.NegativeBuckets = make([]float64, l)
	}
	for i := range fh.NegativeBuckets {
		fh.NegativeBuckets[i] = buf.Be64Float64()
	}

	if histogram.IsCustomBucketsSchema(fh.Schema) {
		l = buf.Uvarint()
		if l > 0 {
			fh.CustomValues = make([]float64, l)
		}
		for i := range fh.CustomValues {
			fh.CustomValues[i] = buf.Be64Float64()
		}
	}
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
	// TODO: reconsider if this function could be pushed down into labels.Labels to be more efficient.
	buf.PutUvarint(lbls.Len())

	lbls.Range(func(l labels.Label) {
		buf.PutUvarintStr(l.Name)
		buf.PutUvarintStr(l.Value)
	})
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

func (e *Encoder) MmapMarkers(markers []RefMmapMarker, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(MmapMarkers))

	for _, s := range markers {
		buf.PutBE64(uint64(s.Ref))
		buf.PutBE64(uint64(s.MmapRef))
	}

	return buf.Get()
}

func (e *Encoder) HistogramSamples(histograms []RefHistogramSample, b []byte) ([]byte, []RefHistogramSample) {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(HistogramSamples))

	if len(histograms) == 0 {
		return buf.Get(), nil
	}
	var customBucketHistograms []RefHistogramSample

	// Store base timestamp and base reference number of first histogram.
	// All histograms encode their timestamp and ref as delta to those.
	first := histograms[0]
	buf.PutBE64(uint64(first.Ref))
	buf.PutBE64int64(first.T)

	for _, h := range histograms {
		if h.H.UsesCustomBuckets() {
			customBucketHistograms = append(customBucketHistograms, h)
			continue
		}
		buf.PutVarint64(int64(h.Ref) - int64(first.Ref))
		buf.PutVarint64(h.T - first.T)

		EncodeHistogram(&buf, h.H)
	}

	// Reset buffer if only custom bucket histograms existed in list of histogram samples.
	if len(histograms) == len(customBucketHistograms) {
		buf.Reset()
	}

	return buf.Get(), customBucketHistograms
}

func (e *Encoder) CustomBucketsHistogramSamples(histograms []RefHistogramSample, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(CustomBucketsHistogramSamples))

	if len(histograms) == 0 {
		return buf.Get()
	}

	// Store base timestamp and base reference number of first histogram.
	// All histograms encode their timestamp and ref as delta to those.
	first := histograms[0]
	buf.PutBE64(uint64(first.Ref))
	buf.PutBE64int64(first.T)

	for _, h := range histograms {
		buf.PutVarint64(int64(h.Ref) - int64(first.Ref))
		buf.PutVarint64(h.T - first.T)

		EncodeHistogram(&buf, h.H)
	}

	return buf.Get()
}

// EncodeHistogram encodes a Histogram into a byte slice.
func EncodeHistogram(buf *encoding.Encbuf, h *histogram.Histogram) {
	buf.PutByte(byte(h.CounterResetHint))

	buf.PutVarint64(int64(h.Schema))
	buf.PutBE64(math.Float64bits(h.ZeroThreshold))

	buf.PutUvarint64(h.ZeroCount)
	buf.PutUvarint64(h.Count)
	buf.PutBE64(math.Float64bits(h.Sum))

	buf.PutUvarint(len(h.PositiveSpans))
	for _, s := range h.PositiveSpans {
		buf.PutVarint64(int64(s.Offset))
		buf.PutUvarint32(s.Length)
	}

	buf.PutUvarint(len(h.NegativeSpans))
	for _, s := range h.NegativeSpans {
		buf.PutVarint64(int64(s.Offset))
		buf.PutUvarint32(s.Length)
	}

	buf.PutUvarint(len(h.PositiveBuckets))
	for _, b := range h.PositiveBuckets {
		buf.PutVarint64(b)
	}

	buf.PutUvarint(len(h.NegativeBuckets))
	for _, b := range h.NegativeBuckets {
		buf.PutVarint64(b)
	}

	if histogram.IsCustomBucketsSchema(h.Schema) {
		buf.PutUvarint(len(h.CustomValues))
		for _, v := range h.CustomValues {
			buf.PutBEFloat64(v)
		}
	}
}

func (e *Encoder) FloatHistogramSamples(histograms []RefFloatHistogramSample, b []byte) ([]byte, []RefFloatHistogramSample) {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(FloatHistogramSamples))

	if len(histograms) == 0 {
		return buf.Get(), nil
	}

	var customBucketsFloatHistograms []RefFloatHistogramSample

	// Store base timestamp and base reference number of first histogram.
	// All histograms encode their timestamp and ref as delta to those.
	first := histograms[0]
	buf.PutBE64(uint64(first.Ref))
	buf.PutBE64int64(first.T)

	for _, h := range histograms {
		if h.FH.UsesCustomBuckets() {
			customBucketsFloatHistograms = append(customBucketsFloatHistograms, h)
			continue
		}
		buf.PutVarint64(int64(h.Ref) - int64(first.Ref))
		buf.PutVarint64(h.T - first.T)

		EncodeFloatHistogram(&buf, h.FH)
	}

	// Reset buffer if only custom bucket histograms existed in list of histogram samples
	if len(histograms) == len(customBucketsFloatHistograms) {
		buf.Reset()
	}

	return buf.Get(), customBucketsFloatHistograms
}

func (e *Encoder) CustomBucketsFloatHistogramSamples(histograms []RefFloatHistogramSample, b []byte) []byte {
	buf := encoding.Encbuf{B: b}
	buf.PutByte(byte(CustomBucketsFloatHistogramSamples))

	if len(histograms) == 0 {
		return buf.Get()
	}

	// Store base timestamp and base reference number of first histogram.
	// All histograms encode their timestamp and ref as delta to those.
	first := histograms[0]
	buf.PutBE64(uint64(first.Ref))
	buf.PutBE64int64(first.T)

	for _, h := range histograms {
		buf.PutVarint64(int64(h.Ref) - int64(first.Ref))
		buf.PutVarint64(h.T - first.T)

		EncodeFloatHistogram(&buf, h.FH)
	}

	return buf.Get()
}

// EncodeFloatHistogram encodes the Float Histogram into a byte slice.
func EncodeFloatHistogram(buf *encoding.Encbuf, h *histogram.FloatHistogram) {
	buf.PutByte(byte(h.CounterResetHint))

	buf.PutVarint64(int64(h.Schema))
	buf.PutBEFloat64(h.ZeroThreshold)

	buf.PutBEFloat64(h.ZeroCount)
	buf.PutBEFloat64(h.Count)
	buf.PutBEFloat64(h.Sum)

	buf.PutUvarint(len(h.PositiveSpans))
	for _, s := range h.PositiveSpans {
		buf.PutVarint64(int64(s.Offset))
		buf.PutUvarint32(s.Length)
	}

	buf.PutUvarint(len(h.NegativeSpans))
	for _, s := range h.NegativeSpans {
		buf.PutVarint64(int64(s.Offset))
		buf.PutUvarint32(s.Length)
	}

	buf.PutUvarint(len(h.PositiveBuckets))
	for _, b := range h.PositiveBuckets {
		buf.PutBEFloat64(b)
	}

	buf.PutUvarint(len(h.NegativeBuckets))
	for _, b := range h.NegativeBuckets {
		buf.PutBEFloat64(b)
	}

	if histogram.IsCustomBucketsSchema(h.Schema) {
		buf.PutUvarint(len(h.CustomValues))
		for _, v := range h.CustomValues {
			buf.PutBEFloat64(v)
		}
	}
}
