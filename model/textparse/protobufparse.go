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
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
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
	in        []byte // The input to parse.
	inPos     int    // Position within the input.
	metricPos int    // Position within Metric slice.
	// fieldPos is the position within a Summary or (legacy) Histogram. -2
	// is the count. -1 is the sum. Otherwise it is the index within
	// quantiles/buckets.
	fieldPos    int
	fieldsDone  bool // true if no more fields of a Summary or (legacy) Histogram to be processed.
	redoClassic bool // true after parsing a native histogram if we need to parse it again as a classic histogram.
	// exemplarPos is the position within the exemplars slice of a native histogram.
	exemplarPos int

	// exemplarReturned is set to true each time an exemplar has been
	// returned, and set back to false upon each Next() call.
	exemplarReturned bool

	// state is marked by the entry we are processing. EntryInvalid implies
	// that we have to decode the next MetricFamily.
	state Entry

	builder labels.ScratchBuilder // held here to reduce allocations when building Labels

	mf *dto.MetricFamily

	// Whether to also parse a classic histogram that is also present as a
	// native histogram.
	parseClassicHistograms bool

	// The following are just shenanigans to satisfy the Parser interface.
	metricBytes *bytes.Buffer // A somewhat fluid representation of the current metric.
}

// NewProtobufParser returns a parser for the payload in the byte slice.
func NewProtobufParser(b []byte, parseClassicHistograms bool, st *labels.SymbolTable) Parser {
	return &ProtobufParser{
		in:                     b,
		state:                  EntryInvalid,
		mf:                     &dto.MetricFamily{},
		metricBytes:            &bytes.Buffer{},
		parseClassicHistograms: parseClassicHistograms,
		builder:                labels.NewScratchBuilderWithSymbolTable(st, 16),
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
	// TODO(beorn7): We assume here that ts==0 means no timestamp. That's
	// not true in general, but proto3 originally has no distinction between
	// unset and default. At a later stage, the `optional` keyword was
	// (re-)introduced in proto3, but gogo-protobuf never got updated to
	// support it. (Note that setting `[(gogoproto.nullable) = true]` for
	// the `timestamp_ms` field doesn't help, either.) We plan to migrate
	// away from gogo-protobuf to an actively maintained protobuf
	// implementation. Once that's done, we can simply use the `optional`
	// keyword and check for the unset state explicitly.
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
func (p *ProtobufParser) Type() ([]byte, model.MetricType) {
	n := p.metricBytes.Bytes()
	switch p.mf.GetType() {
	case dto.MetricType_COUNTER:
		return n, model.MetricTypeCounter
	case dto.MetricType_GAUGE:
		return n, model.MetricTypeGauge
	case dto.MetricType_HISTOGRAM:
		return n, model.MetricTypeHistogram
	case dto.MetricType_GAUGE_HISTOGRAM:
		return n, model.MetricTypeGaugeHistogram
	case dto.MetricType_SUMMARY:
		return n, model.MetricTypeSummary
	}
	return n, model.MetricTypeUnknown
}

// Unit returns the metric unit in the current entry.
// Must only be called after Next returned a unit entry.
// The returned byte slices become invalid after the next call to Next.
func (p *ProtobufParser) Unit() ([]byte, []byte) {
	return p.metricBytes.Bytes(), []byte(p.mf.GetUnit())
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
// histogram, the exemplars in the native histogram will be returned.
// If this field is empty, the classic bucket section is still used for exemplars.
// To ingest all exemplars, call the Exemplar method repeatedly until it returns false.
func (p *ProtobufParser) Exemplar(ex *exemplar.Exemplar) bool {
	if p.exemplarReturned && p.state == EntrySeries {
		// We only ever return one exemplar per (non-native-histogram) series.
		return false
	}
	m := p.mf.GetMetric()[p.metricPos]
	var exProto *dto.Exemplar
	switch p.mf.GetType() {
	case dto.MetricType_COUNTER:
		exProto = m.GetCounter().GetExemplar()
	case dto.MetricType_HISTOGRAM, dto.MetricType_GAUGE_HISTOGRAM:
		isClassic := p.state == EntrySeries
		if !isClassic && len(m.GetHistogram().GetExemplars()) > 0 {
			exs := m.GetHistogram().GetExemplars()
			for p.exemplarPos < len(exs) {
				exProto = exs[p.exemplarPos]
				p.exemplarPos++
				if exProto != nil && exProto.GetTimestamp() != nil {
					break
				}
			}
			if exProto != nil && exProto.GetTimestamp() == nil {
				return false
			}
		} else {
			bb := m.GetHistogram().GetBucket()
			if p.fieldPos < 0 {
				if isClassic {
					return false // At _count or _sum.
				}
				p.fieldPos = 0 // Start at 1st bucket for native histograms.
			}
			for p.fieldPos < len(bb) {
				exProto = bb[p.fieldPos].GetExemplar()
				if isClassic {
					break
				}
				p.fieldPos++
				// We deliberately drop exemplars with no timestamp only for native histograms.
				if exProto != nil && (isClassic || exProto.GetTimestamp() != nil) {
					break // Found a classic histogram exemplar or a native histogram exemplar with a timestamp.
				}
			}
			// If the last exemplar for native histograms has no timestamp, ignore it.
			if !isClassic && exProto.GetTimestamp() == nil {
				return false
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
	p.exemplarReturned = true
	return true
}

// CreatedTimestamp returns CT or nil if CT is not present or
// invalid (as timestamp e.g. negative value) on counters, summaries or histograms.
func (p *ProtobufParser) CreatedTimestamp() *int64 {
	var ct *types.Timestamp
	switch p.mf.GetType() {
	case dto.MetricType_COUNTER:
		ct = p.mf.GetMetric()[p.metricPos].GetCounter().GetCreatedTimestamp()
	case dto.MetricType_SUMMARY:
		ct = p.mf.GetMetric()[p.metricPos].GetSummary().GetCreatedTimestamp()
	case dto.MetricType_HISTOGRAM, dto.MetricType_GAUGE_HISTOGRAM:
		ct = p.mf.GetMetric()[p.metricPos].GetHistogram().GetCreatedTimestamp()
	default:
	}
	ctAsTime, err := types.TimestampFromProto(ct)
	if err != nil {
		// Errors means ct == nil or invalid timestamp, which we silently ignore.
		return nil
	}
	ctMilis := ctAsTime.UnixMilli()
	return &ctMilis
}

// Next advances the parser to the next "sample" (emulating the behavior of a
// text format parser). It returns (EntryInvalid, io.EOF) if no samples were
// read.
func (p *ProtobufParser) Next() (Entry, error) {
	p.exemplarReturned = false
	switch p.state {
	case EntryInvalid:
		p.metricPos = 0
		p.exemplarPos = 0
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
			return EntryInvalid, fmt.Errorf("invalid metric name: %s", name)
		}
		if help := p.mf.GetHelp(); !utf8.ValidString(help) {
			return EntryInvalid, fmt.Errorf("invalid help for metric %q: %s", name, help)
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
			return EntryInvalid, fmt.Errorf("unknown metric type for metric %q: %s", name, p.mf.GetType())
		}
		unit := p.mf.GetUnit()
		if len(unit) > 0 {
			if p.mf.GetType() == dto.MetricType_COUNTER && strings.HasSuffix(name, "_total") {
				if !strings.HasSuffix(name[:len(name)-6], unit) || len(name)-6 < len(unit)+1 || name[len(name)-6-len(unit)-1] != '_' {
					return EntryInvalid, fmt.Errorf("unit %q not a suffix of counter %q", unit, name)
				}
			} else if !strings.HasSuffix(name, unit) || len(name) < len(unit)+1 || name[len(name)-len(unit)-1] != '_' {
				return EntryInvalid, fmt.Errorf("unit %q not a suffix of metric %q", unit, name)
			}
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
			p.exemplarPos = 0
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
		return EntryInvalid, fmt.Errorf("invalid protobuf parsing state: %d", p.state)
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
			return fmt.Errorf("invalid label name: %s", n)
		}
		b.WriteString(n)
		b.WriteByte(model.SeparatorByte)
		v := lp.GetValue()
		if !utf8.ValidString(v) {
			return fmt.Errorf("invalid label value: %s", v)
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
		return 0, fmt.Errorf("protobufparse: insufficient length of buffer, expected at least %d bytes, got %d bytes", totalLength, len(b))
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

// isNativeHistogram returns false iff the provided histograms has no spans at
// all (neither positive nor negative) and a zero threshold of 0 and a zero
// count of 0. In principle, this could still be meant to be a native histogram
// with a zero threshold of 0 and no observations yet. In that case,
// instrumentation libraries should add a "no-op" span (e.g. length zero, offset
// zero) to signal that the histogram is meant to be parsed as a native
// histogram. Failing to do so will cause Prometheus to parse it as a classic
// histogram as long as no observations have happened.
func isNativeHistogram(h *dto.Histogram) bool {
	return len(h.GetPositiveSpan()) > 0 ||
		len(h.GetNegativeSpan()) > 0 ||
		h.GetZeroThreshold() > 0 ||
		h.GetZeroCount() > 0
}
