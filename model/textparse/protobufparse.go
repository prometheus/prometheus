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
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"

	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
)

// ProtobufParser is a very inefficient way of unmarshaling the old Prometheus
// protobuf format and then present it as it if were parsed by a
// Prometheus-2-style text parser. This is only done so that we can easily plug
// in the protobuf format into Prometheus 2. For future use (with the final
// format that will be used for native histograms), we have to revisit the
// parsing. A lot of the efficiency tricks of the Prometheus-2-style parsing
// could be used in a similar fashion (byte-slice pointers into the raw
// payload), which requires some hand-coded protobuf handling. But the current
// parsers all expect the full series name (metric name plus label pairs) as one
// string, which is not how things are represented in the protobuf format. If
// the re-arrangement work is actually causing problems (which has to be seen),
// that expectation needs to be changed.
type ProtobufParser struct {
	in        []byte // The intput to parse.
	inPos     int    // Position within the input.
	metricPos int    // Position within Metric slice.
	// fieldPos is the position within a Summary or (legacy) Histogram. -2
	// is the count. -1 is the sum. Otherwise it is the index within
	// quantiles/buckets.
	fieldPos    int
	fieldsDone  bool // true if no more fields of a Summary or (legacy) Histogram to be processed.
	redoClassic bool // true after parsing a native histogram if we need to parse it again as a classic histogram.

	// state is marked by the entry we are processing. EntryInvalid implies
	// that we have to decode the next MetricFamily.
	state Entry

	builder labels.ScratchBuilder // held here to reduce allocations when building Labels

	mf *dto.MetricFamily

	// Wether to also parse a classic histogram that is also present as a
	// native histogram.
	parseClassicHistograms bool

	// The following are just shenanigans to satisfy the Parser interface.
	metricBytes *bytes.Buffer // A somewhat fluid representation of the current metric.
}

// NewProtobufParser returns a parser for the payload in the byte slice.
func NewProtobufParser(b []byte, parseClassicHistograms bool) Parser {
	return &ProtobufParser{
		in:                     b,
		state:                  EntryInvalid,
		mf:                     &dto.MetricFamily{},
		metricBytes:            &bytes.Buffer{},
		parseClassicHistograms: parseClassicHistograms,
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
	case dto.MetricType_SUMMARY:
		s := m.GetSummary()
		switch p.fieldPos {
		case -2:
			v = float64(s.GetSampleCount())
		case -1:
			v = s.GetSampleSum()
			// Need to detect summaries without quantile here.
			if len(s.GetQuantile()) == 0 {
				p.fieldsDone = true
			}
		default:
			v = s.GetQuantile()[p.fieldPos].GetValue()
		}
	case dto.MetricType_HISTOGRAM, dto.MetricType_GAUGE_HISTOGRAM:
		// This should only happen for a classic histogram.
		h := m.GetHistogram()
		switch p.fieldPos {
		case -2:
			v = h.GetSampleCountFloat()
			if v == 0 {
				v = float64(h.GetSampleCount())
			}
		case -1:
			v = h.GetSampleSum()
		default:
			bb := h.GetBucket()
			if p.fieldPos >= len(bb) {
				v = h.GetSampleCountFloat()
				if v == 0 {
					v = float64(h.GetSampleCount())
				}
			} else {
				v = bb[p.fieldPos].GetCumulativeCountFloat()
				if v == 0 {
					v = float64(bb[p.fieldPos].GetCumulativeCount())
				}
			}
		}
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

// Histogram returns the bytes of a series with a native histogram as a value,
// the timestamp if set, and the native histogram in the current sample.
//
// The Compact method is called before returning the Histogram (or FloatHistogram).
//
// If the SampleCountFloat or the ZeroCountFloat in the proto message is > 0,
// the histogram is parsed and returned as a FloatHistogram and nil is returned
// as the (integer) Histogram return value. Otherwise, it is parsed and returned
// as an (integer) Histogram and nil is returned as the FloatHistogram return
// value.
func (p *ProtobufParser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	var (
		m  = p.mf.GetMetric()[p.metricPos]
		ts = m.GetTimestampMs()
		h  = m.GetHistogram()
	)
	if p.parseClassicHistograms && len(h.GetBucket()) > 0 {
		p.redoClassic = true
	}
	if h.GetSampleCountFloat() > 0 || h.GetZeroCountFloat() > 0 {
		// It is a float histogram.
		fh := histogram.FloatHistogram{
			Count:           h.GetSampleCountFloat(),
			Sum:             h.GetSampleSum(),
			ZeroThreshold:   h.GetZeroThreshold(),
			ZeroCount:       h.GetZeroCountFloat(),
			Schema:          h.GetSchema(),
			PositiveSpans:   make([]histogram.Span, len(h.GetPositiveSpan())),
			PositiveBuckets: h.GetPositiveCount(),
			NegativeSpans:   make([]histogram.Span, len(h.GetNegativeSpan())),
			NegativeBuckets: h.GetNegativeCount(),
		}
		for i, span := range h.GetPositiveSpan() {
			fh.PositiveSpans[i].Offset = span.GetOffset()
			fh.PositiveSpans[i].Length = span.GetLength()
		}
		for i, span := range h.GetNegativeSpan() {
			fh.NegativeSpans[i].Offset = span.GetOffset()
			fh.NegativeSpans[i].Length = span.GetLength()
		}
		if p.mf.GetType() == dto.MetricType_GAUGE_HISTOGRAM {
			fh.CounterResetHint = histogram.GaugeType
		}
		fh.Compact(0)
		if ts != 0 {
			return p.metricBytes.Bytes(), &ts, nil, &fh
		}
		// Nasty hack: Assume that ts==0 means no timestamp. That's not true in
		// general, but proto3 has no distinction between unset and
		// default. Need to avoid in the final format.
		return p.metricBytes.Bytes(), nil, nil, &fh
	}

	sh := histogram.Histogram{
		Count:           h.GetSampleCount(),
		Sum:             h.GetSampleSum(),
		ZeroThreshold:   h.GetZeroThreshold(),
		ZeroCount:       h.GetZeroCount(),
		Schema:          h.GetSchema(),
		PositiveSpans:   make([]histogram.Span, len(h.GetPositiveSpan())),
		PositiveBuckets: h.GetPositiveDelta(),
		NegativeSpans:   make([]histogram.Span, len(h.GetNegativeSpan())),
		NegativeBuckets: h.GetNegativeDelta(),
	}
	for i, span := range h.GetPositiveSpan() {
		sh.PositiveSpans[i].Offset = span.GetOffset()
		sh.PositiveSpans[i].Length = span.GetLength()
	}
	for i, span := range h.GetNegativeSpan() {
		sh.NegativeSpans[i].Offset = span.GetOffset()
		sh.NegativeSpans[i].Length = span.GetLength()
	}
	if p.mf.GetType() == dto.MetricType_GAUGE_HISTOGRAM {
		sh.CounterResetHint = histogram.GaugeType
	}
	sh.Compact(0)
	if ts != 0 {
		return p.metricBytes.Bytes(), &ts, &sh, nil
	}
	return p.metricBytes.Bytes(), nil, &sh, nil
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
	case dto.MetricType_GAUGE_HISTOGRAM:
		return n, MetricTypeGaugeHistogram
	case dto.MetricType_SUMMARY:
		return n, MetricTypeSummary
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
	p.builder.Reset()
	p.builder.Add(labels.MetricName, p.getMagicName())

	for _, lp := range p.mf.GetMetric()[p.metricPos].GetLabel() {
		p.builder.Add(lp.GetName(), lp.GetValue())
	}
	if needed, name, value := p.getMagicLabel(); needed {
		p.builder.Add(name, value)
	}

	// Sort labels to maintain the sorted labels invariant.
	p.builder.Sort()
	*l = p.builder.Labels()

	return p.metricBytes.String()
}

// Exemplar writes the exemplar of the current sample into the passed
// exemplar. It returns if an exemplar exists or not. In case of a native
// histogram, the legacy bucket section is still used for exemplars. To ingest
// all examplars, call the Exemplar method repeatedly until it returns false.
func (p *ProtobufParser) Exemplar(ex *exemplar.Exemplar) bool {
	m := p.mf.GetMetric()[p.metricPos]
	var exProto *dto.Exemplar
	switch p.mf.GetType() {
	case dto.MetricType_COUNTER:
		exProto = m.GetCounter().GetExemplar()
	case dto.MetricType_HISTOGRAM, dto.MetricType_GAUGE_HISTOGRAM:
		bb := m.GetHistogram().GetBucket()
		if p.fieldPos < 0 {
			if p.state == EntrySeries {
				return false // At _count or _sum.
			}
			p.fieldPos = 0 // Start at 1st bucket for native histograms.
		}
		for p.fieldPos < len(bb) {
			exProto = bb[p.fieldPos].GetExemplar()
			if p.state == EntrySeries {
				break
			}
			p.fieldPos++
			if exProto != nil {
				break
			}
		}
	default:
		return false
	}
	if exProto == nil {
		return false
	}
	ex.Value = exProto.GetValue()
	if ts := exProto.GetTimestamp(); ts != nil {
		ex.HasTs = true
		ex.Ts = ts.GetSeconds()*1000 + int64(ts.GetNanos()/1_000_000)
	}
	p.builder.Reset()
	for _, lp := range exProto.GetLabel() {
		p.builder.Add(lp.GetName(), lp.GetValue())
	}
	p.builder.Sort()
	ex.Labels = p.builder.Labels()
	return true
}

// Next advances the parser to the next "sample" (emulating the behavior of a
// text format parser). It returns (EntryInvalid, io.EOF) if no samples were
// read.
func (p *ProtobufParser) Next() (Entry, error) {
	switch p.state {
	case EntryInvalid:
		p.metricPos = 0
		p.fieldPos = -2
		n, err := readDelimited(p.in[p.inPos:], p.mf)
		p.inPos += n
		if err != nil {
			return p.state, err
		}

		// Skip empty metric families.
		if len(p.mf.GetMetric()) == 0 {
			return p.Next()
		}

		// We are at the beginning of a metric family. Put only the name
		// into metricBytes and validate only name, help, and type for now.
		name := p.mf.GetName()
		if !model.IsValidMetricName(model.LabelValue(name)) {
			return EntryInvalid, errors.Errorf("invalid metric name: %s", name)
		}
		if help := p.mf.GetHelp(); !utf8.ValidString(help) {
			return EntryInvalid, errors.Errorf("invalid help for metric %q: %s", name, help)
		}
		switch p.mf.GetType() {
		case dto.MetricType_COUNTER,
			dto.MetricType_GAUGE,
			dto.MetricType_HISTOGRAM,
			dto.MetricType_GAUGE_HISTOGRAM,
			dto.MetricType_SUMMARY,
			dto.MetricType_UNTYPED:
			// All good.
		default:
			return EntryInvalid, errors.Errorf("unknown metric type for metric %q: %s", name, p.mf.GetType())
		}
		p.metricBytes.Reset()
		p.metricBytes.WriteString(name)

		p.state = EntryHelp
	case EntryHelp:
		p.state = EntryType
	case EntryType:
		t := p.mf.GetType()
		if (t == dto.MetricType_HISTOGRAM || t == dto.MetricType_GAUGE_HISTOGRAM) &&
			isNativeHistogram(p.mf.GetMetric()[0].GetHistogram()) {
			p.state = EntryHistogram
		} else {
			p.state = EntrySeries
		}
		if err := p.updateMetricBytes(); err != nil {
			return EntryInvalid, err
		}
	case EntryHistogram, EntrySeries:
		if p.redoClassic {
			p.redoClassic = false
			p.state = EntrySeries
			p.fieldPos = -3
			p.fieldsDone = false
		}
		t := p.mf.GetType()
		if p.state == EntrySeries && !p.fieldsDone &&
			(t == dto.MetricType_SUMMARY ||
				t == dto.MetricType_HISTOGRAM ||
				t == dto.MetricType_GAUGE_HISTOGRAM) {
			p.fieldPos++
		} else {
			p.metricPos++
			p.fieldPos = -2
			p.fieldsDone = false
			// If this is a metric family containing native
			// histograms, we have to switch back to native
			// histograms after parsing a classic histogram.
			if p.state == EntrySeries &&
				(t == dto.MetricType_HISTOGRAM || t == dto.MetricType_GAUGE_HISTOGRAM) &&
				isNativeHistogram(p.mf.GetMetric()[0].GetHistogram()) {
				p.state = EntryHistogram
			}
		}
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
	b.WriteString(p.getMagicName())
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
	if needed, n, v := p.getMagicLabel(); needed {
		b.WriteByte(model.SeparatorByte)
		b.WriteString(n)
		b.WriteByte(model.SeparatorByte)
		b.WriteString(v)
	}
	return nil
}

// getMagicName usually just returns p.mf.GetType() but adds a magic suffix
// ("_count", "_sum", "_bucket") if needed according to the current parser
// state.
func (p *ProtobufParser) getMagicName() string {
	t := p.mf.GetType()
	if p.state == EntryHistogram || (t != dto.MetricType_HISTOGRAM && t != dto.MetricType_GAUGE_HISTOGRAM && t != dto.MetricType_SUMMARY) {
		return p.mf.GetName()
	}
	if p.fieldPos == -2 {
		return p.mf.GetName() + "_count"
	}
	if p.fieldPos == -1 {
		return p.mf.GetName() + "_sum"
	}
	if t == dto.MetricType_HISTOGRAM || t == dto.MetricType_GAUGE_HISTOGRAM {
		return p.mf.GetName() + "_bucket"
	}
	return p.mf.GetName()
}

// getMagicLabel returns if a magic label ("quantile" or "le") is needed and, if
// so, its name and value. It also sets p.fieldsDone if applicable.
func (p *ProtobufParser) getMagicLabel() (bool, string, string) {
	if p.state == EntryHistogram || p.fieldPos < 0 {
		return false, "", ""
	}
	switch p.mf.GetType() {
	case dto.MetricType_SUMMARY:
		qq := p.mf.GetMetric()[p.metricPos].GetSummary().GetQuantile()
		q := qq[p.fieldPos]
		p.fieldsDone = p.fieldPos == len(qq)-1
		return true, model.QuantileLabel, formatOpenMetricsFloat(q.GetQuantile())
	case dto.MetricType_HISTOGRAM, dto.MetricType_GAUGE_HISTOGRAM:
		bb := p.mf.GetMetric()[p.metricPos].GetHistogram().GetBucket()
		if p.fieldPos >= len(bb) {
			p.fieldsDone = true
			return true, model.BucketLabel, "+Inf"
		}
		b := bb[p.fieldPos]
		p.fieldsDone = math.IsInf(b.GetUpperBound(), +1)
		return true, model.BucketLabel, formatOpenMetricsFloat(b.GetUpperBound())
	}
	return false, "", ""
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

// formatOpenMetricsFloat works like the usual Go string formatting of a fleat
// but appends ".0" if the resulting number would otherwise contain neither a
// "." nor an "e".
func formatOpenMetricsFloat(f float64) string {
	// A few common cases hardcoded.
	switch {
	case f == 1:
		return "1.0"
	case f == 0:
		return "0.0"
	case f == -1:
		return "-1.0"
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, +1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	s := fmt.Sprint(f)
	if strings.ContainsAny(s, "e.") {
		return s
	}
	return s + ".0"
}

// isNativeHistogram returns false iff the provided histograms has no sparse
// buckets and a zero threshold of 0 and a zero count of 0. In principle, this
// could still be meant to be a native histogram (with a zero threshold of 0 and
// no observations yet), but for now, we'll treat this case as a conventional
// histogram.
//
// TODO(beorn7): In the final format, there should be an unambiguous way of
// deciding if a histogram should be ingested as a conventional one or a native
// one.
func isNativeHistogram(h *dto.Histogram) bool {
	return h.GetZeroThreshold() > 0 ||
		h.GetZeroCount() > 0 ||
		len(h.GetNegativeDelta()) > 0 ||
		len(h.GetPositiveDelta()) > 0 ||
		len(h.GetNegativeCount()) > 0 ||
		len(h.GetPositiveCount()) > 0
}
