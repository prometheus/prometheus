// Copyright 2021 The Prometheus Authors
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

package textparse

import (
	"bytes"
	"encoding/binary"
	"io"
	"sort"
	"unicode/utf8"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/histogram"
	"github.com/prometheus/prometheus/pkg/labels"

	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
)

// ProtobufParser is a very inefficient way of unmarshaling the old Prometheus
// protobuf format and then present it as it if were parsed by a
// Prometheus-2-style text parser. This is only done so that we can easily plug
// in the protobuf format into Prometheus 2. For future use (with the final
// format that will be used for sparse histograms), we have to revisit the
// parsing. A lot of the efficiency tricks of the Prometheus-2-style parsing
// could be used in a similar fashion (byte-slice pointers into the raw
// payload), which requires some hand-coded protobuf handling. But the current
// parsers all expect the full series name (metric name plus label pairs) as one
// string, which is not how things are represented in the protobuf format. If
// the re-arrangement work is actually causing problems (which has to be seen),
// that expectation needs to be changed.
//
// TODO(beorn7): The parser currently ignores summaries and legacy histograms
// (those without sparse buckets) to keep things simple.
type ProtobufParser struct {
	in    []byte // The intput to parse.
	inPos int    // Position within the input.
	state Entry  // State is marked by the entry we are
	// processing. EntryInvalid implies that we have to
	// decode the next MetricFamily.
	metricPos int // Position within Metric slice.
	mf        *dto.MetricFamily

	// The following are just shenanigans to satisfy the Parser interface.
	metricBytes *bytes.Buffer // A somewhat fluid representation of the current metric.
}

func NewProtobufParser(b []byte) Parser {
	return &ProtobufParser{
		in:          b,
		state:       EntryInvalid,
		mf:          &dto.MetricFamily{},
		metricBytes: &bytes.Buffer{},
	}
}

// Series returns the bytes of a series with a simple float64 as a
// value, the timestamp if set, and the value of the current sample.
func (p *ProtobufParser) Series() ([]byte, *int64, float64) {
	var (
		m  = p.mf.GetMetric()[p.metricPos]
		ts = m.GetTimestampMs()
		v  float64
	)
	switch p.mf.GetType() {
	case dto.MetricType_COUNTER:
		v = m.GetCounter().GetValue()
	case dto.MetricType_GAUGE:
		v = m.GetGauge().GetValue()
	case dto.MetricType_UNTYPED:
		v = m.GetUntyped().GetValue()
	default:
		panic("encountered unexpected metric type, this is a bug")
	}
	if ts != 0 {
		return p.metricBytes.Bytes(), &ts, v
	}
	// Nasty hack: Assume that ts==0 means no timestamp. That's not true in
	// general, but proto3 has no distinction between unset and
	// default. Need to avoid in the final format.
	return p.metricBytes.Bytes(), nil, v
}

// Histogram returns the bytes of a series with a sparse histogram as a
// value, the timestamp if set, and the sparse histogram in the current
// sample.
func (p *ProtobufParser) Histogram() ([]byte, *int64, histogram.SparseHistogram) {
	var (
		m  = p.mf.GetMetric()[p.metricPos]
		ts = m.GetTimestampMs()
		h  = m.GetHistogram()
	)
	sh := histogram.SparseHistogram{
		Count:           h.GetSampleCount(),
		Sum:             h.GetSampleSum(),
		ZeroThreshold:   h.GetSbZeroThreshold(),
		ZeroCount:       h.GetSbZeroCount(),
		Schema:          h.GetSbSchema(),
		PositiveSpans:   make([]histogram.Span, len(h.GetSbPositive().GetSpan())),
		PositiveBuckets: h.GetSbPositive().GetDelta(),
		NegativeSpans:   make([]histogram.Span, len(h.GetSbNegative().GetSpan())),
		NegativeBuckets: h.GetSbNegative().GetDelta(),
	}
	for i, span := range h.GetSbPositive().GetSpan() {
		sh.PositiveSpans[i].Offset = span.GetOffset()
		sh.PositiveSpans[i].Length = span.GetLength()
	}
	for i, span := range h.GetSbNegative().GetSpan() {
		sh.NegativeSpans[i].Offset = span.GetOffset()
		sh.NegativeSpans[i].Length = span.GetLength()
	}
	if ts != 0 {
		return p.metricBytes.Bytes(), &ts, sh
	}
	// Nasty hack: Assume that ts==0 means no timestamp. That's not true in
	// general, but proto3 has no distinction between unset and
	// default. Need to avoid in the final format.
	return p.metricBytes.Bytes(), nil, sh
}

// Help returns the metric name and help text in the current entry.
// Must only be called after Next returned a help entry.
// The returned byte slices become invalid after the next call to Next.
func (p *ProtobufParser) Help() ([]byte, []byte) {
	return p.metricBytes.Bytes(), []byte(p.mf.GetHelp())
}

// Type returns the metric name and type in the current entry.
// Must only be called after Next returned a type entry.
// The returned byte slices become invalid after the next call to Next.
func (p *ProtobufParser) Type() ([]byte, MetricType) {
	n := p.metricBytes.Bytes()
	switch p.mf.GetType() {
	case dto.MetricType_COUNTER:
		return n, MetricTypeCounter
	case dto.MetricType_GAUGE:
		return n, MetricTypeGauge
	case dto.MetricType_HISTOGRAM:
		return n, MetricTypeHistogram
	}
	return n, MetricTypeUnknown
}

// Unit always returns (nil, nil) because units aren't supported by the protobuf
// format.
func (p *ProtobufParser) Unit() ([]byte, []byte) {
	return nil, nil
}

// Comment always returns nil because comments aren't supported by the protobuf
// format.
func (p *ProtobufParser) Comment() []byte {
	return nil
}

// Metric writes the labels of the current sample into the passed labels.
// It returns the string from which the metric was parsed.
func (p *ProtobufParser) Metric(l *labels.Labels) string {
	*l = append(*l, labels.Label{
		Name:  labels.MetricName,
		Value: p.mf.GetName(),
	})

	for _, lp := range p.mf.GetMetric()[p.metricPos].GetLabel() {
		*l = append(*l, labels.Label{
			Name:  lp.GetName(),
			Value: lp.GetValue(),
		})
	}

	// Sort labels to maintain the sorted labels invariant.
	sort.Sort(*l)

	return p.metricBytes.String()
}

// Exemplar always returns false because exemplars aren't supported yet by the
// protobuf format.
func (p *ProtobufParser) Exemplar(l *exemplar.Exemplar) bool {
	return false
}

// Next advances the parser to the next "sample" (emulating the behavior of a
// text format parser). It returns (EntryInvalid, io.EOF) if no samples were
// read.
func (p *ProtobufParser) Next() (Entry, error) {
	switch p.state {
	case EntryInvalid:
		p.metricPos = 0
		n, err := readDelimited(p.in[p.inPos:], p.mf)
		p.inPos += n
		if err != nil {
			return p.state, err
		}

		// Skip empty metric families. While checking for emptiness, ignore
		// summaries and legacy histograms for now.
		metricFound := false
		metricType := p.mf.GetType()
		for _, m := range p.mf.GetMetric() {
			if metricType == dto.MetricType_COUNTER ||
				metricType == dto.MetricType_GAUGE ||
				metricType == dto.MetricType_UNTYPED ||
				(metricType == dto.MetricType_HISTOGRAM &&
					// A histogram with a non-zero SbZerothreshold
					// is a sparse histogram.
					m.GetHistogram().GetSbZeroThreshold() != 0) {
				metricFound = true
				break
			}
		}
		if !metricFound {
			return p.Next()
		}

		// We are at the beginning of a metric family. Put only the name
		// into metricBytes and validate only name and help for now.
		name := p.mf.GetName()
		if !model.IsValidMetricName(model.LabelValue(name)) {
			return EntryInvalid, errors.Errorf("invalid metric name: %s", name)
		}
		if help := p.mf.GetHelp(); !utf8.ValidString(help) {
			return EntryInvalid, errors.Errorf("invalid help for metric %q: %s", name, help)
		}
		p.metricBytes.Reset()
		p.metricBytes.WriteString(name)

		p.state = EntryHelp
	case EntryHelp:
		p.state = EntryType
	case EntryType:
		if p.mf.GetType() == dto.MetricType_HISTOGRAM {
			p.state = EntryHistogram
		} else {
			p.state = EntrySeries
		}
		if err := p.updateMetricBytes(); err != nil {
			return EntryInvalid, err
		}
	case EntryHistogram, EntrySeries:
		p.metricPos++
		if p.metricPos >= len(p.mf.GetMetric()) {
			p.state = EntryInvalid
			return p.Next()
		}
		if err := p.updateMetricBytes(); err != nil {
			return EntryInvalid, err
		}
	default:
		return EntryInvalid, errors.Errorf("invalid protobuf parsing state: %d", p.state)
	}
	return p.state, nil
}

func (p *ProtobufParser) updateMetricBytes() error {
	b := p.metricBytes
	b.Reset()
	b.WriteString(p.mf.GetName())
	for _, lp := range p.mf.GetMetric()[p.metricPos].GetLabel() {
		b.WriteByte(model.SeparatorByte)
		n := lp.GetName()
		if !model.LabelName(n).IsValid() {
			return errors.Errorf("invalid label name: %s", n)
		}
		b.WriteString(n)
		b.WriteByte(model.SeparatorByte)
		v := lp.GetValue()
		if !utf8.ValidString(v) {
			return errors.Errorf("invalid label value: %s", v)
		}
		b.WriteString(v)
	}
	return nil
}

var errInvalidVarint = errors.New("protobufparse: invalid varint encountered")

// readDelimited is essentially doing what the function of the same name in
// github.com/matttproud/golang_protobuf_extensions/pbutil is doing, but it is
// specific to a MetricFamily, utilizes the more efficient gogo-protobuf
// unmarshaling, and acts on a byte slice directly without any additional
// staging buffers.
func readDelimited(b []byte, mf *dto.MetricFamily) (n int, err error) {
	if len(b) == 0 {
		return 0, io.EOF
	}
	messageLength, varIntLength := proto.DecodeVarint(b)
	if varIntLength == 0 || varIntLength > binary.MaxVarintLen32 {
		return 0, errInvalidVarint
	}
	totalLength := varIntLength + int(messageLength)
	if totalLength > len(b) {
		return 0, errors.Errorf("protobufparse: insufficient length of buffer, expected at least %d bytes, got %d bytes", totalLength, len(b))
	}
	mf.Reset()
	return totalLength, mf.Unmarshal(b[varIntLength:totalLength])
}
