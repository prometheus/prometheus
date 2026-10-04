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

//go:generate go get -u modernc.org/golex
//go:generate golex -o=openmetrics2lex.l.go openmetrics2lex.l

package textparse

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/util/convertnhcb"
)

const (
	sComposite = sETimestamp + 1 + iota
	sCompValue
)

// openMetrics2Lexer is the lexer for the OpenMetrics 2.0 text format.
type openMetrics2Lexer struct {
	b     []byte
	i     int
	start int
	err   error
	state int
}

// buf returns the bytes of the current token.
func (l *openMetrics2Lexer) buf() []byte {
	return l.b[l.start:l.i]
}

// next advances the openMetrics2Lexer to the next character and returns it.
func (l *openMetrics2Lexer) next() byte {
	l.i++
	if l.i >= len(l.b) {
		l.err = io.EOF
		return byte(tEOF)
	}
	// Lex struggles with null bytes. If we are in a label value or help
	// string, where they are allowed, consume them here immediately.
	for l.b[l.i] == 0 && (l.state == sLValue || l.state == sEValue || l.state == sMeta2 || l.state == sComment) {
		l.i++
		if l.i >= len(l.b) {
			l.err = io.EOF
			return byte(tEOF)
		}
	}
	return l.b[l.i]
}

// Error satisfies the golex interface for openMetrics2Lexer.
func (l *openMetrics2Lexer) Error(es string) {
	l.err = errors.New(es)
}

// pendingEntry holds a single exploded flat series to be drained from the
// pending queue that is built when a composite value (summary or classic
// histogram) is parsed.
//
// Labels are not constructed eagerly during Next() because warm scrape caches
// only inspect Series() bytes; Labels() reconstructs the label set lazily from
// p.seriesBuf[seriesStart:seriesEnd] and p.offsets on a cache miss.
type pendingEntry struct {
	seriesStart int
	seriesEnd   int
	nameLen     int
	injectKey   string
	val         float64
}

// om2CompositeScratch holds the buffers that back the exploded composite
// entries (pending queue and the series bytes it points into). They are
// pooled across parser instances because callers like the scrape loop create
// a new parser per scrape, and without pooling every scrape of a page with
// classic histograms or summaries re-grows both buffers to the size of its
// largest family (a few KiB per scrape).
type om2CompositeScratch struct {
	pending   []pendingEntry
	seriesBuf []byte
}

var om2CompositeScratchPool = sync.Pool{
	New: func() any {
		return &om2CompositeScratch{
			pending:   make([]pendingEntry, 0, 16),
			seriesBuf: make([]byte, 0, 1024),
		}
	},
}

// om2Exemplar holds the byte-offset range into p.eOffsets and parsed scalar
// fields for a single exemplar so that Exemplar() can construct labels lazily.
type om2Exemplar struct {
	offsetsStart int
	offsetsEnd   int
	val          float64
	ts           int64
}

// openMetrics2Parser parses samples from a byte slice in the OpenMetrics 2.0
// text exposition format.
// Specification: https://prometheus.io/docs/specs/om/open_metrics_spec_2_0/
//
// Note for exposer and client library implementers: this parser is not a
// conformance test for OpenMetrics 2.0. It is not yet generally available
// thus it might be stricter in the future.
type openMetrics2Parser struct {
	l       *openMetrics2Lexer
	builder labels.ScratchBuilder

	// Metadata for the current metric family. Reset by resetOnFamilyChange.
	mtype model.MetricType
	unit  string
	text  []byte

	// Current sample position in the input.
	series []byte

	// curFamilyName is a slice into l.b holding the last metric name seen by
	// resetOnFamilyChange.
	curFamilyName []byte

	// offsets encodes the metric name and label positions as absolute byte
	// offsets into l.b.  Layout: [nameStart, nameEnd, k1Start, k1End,
	// v1Start, v1End, k2Start, k2End, v2Start, v2End, ...].  Labels() uses
	// these to slice strings without allocating during the hot parse path.
	offsets []int
	start   int

	// Scalar sample value, timestamp.
	val   float64
	hasTS bool
	ts    int64

	// Inline start timestamp from "st@<ts>"; O(1) — no forward scan.
	st    int64
	hasST bool

	// Native histogram value; set when the composite value contains schema/spans.
	h  *histogram.Histogram
	fh *histogram.FloatHistogram

	// Multiple exemplars per sample (OM 2.0 allows zero or more).
	// eOffsets stores the flat [k1Start, k1End, v1Start, v1End, ...] byte
	// offsets into l.b across all exemplars on the current line.
	exemplars   []om2Exemplar
	eOffsets    []int
	exemplarIdx int

	// Pending queue for composite values exploded into flat EntrySeries.
	// pending[0..pendingIdx-1] are waiting to be served; Next() increments
	// pendingIdx and Series()/Labels() read pending[pendingIdx-1].
	pending    []pendingEntry
	pendingIdx int

	// seriesBuf backs pendingEntry series bytes; reused across composite lines
	// to avoid allocating one byte slice per exploded entry.
	seriesBuf []byte

	// scratch is the pooled owner of pending and seriesBuf's backing arrays,
	// taken lazily on the first composite line and returned on io.EOF. Nil
	// means no composite was parsed yet (or the buffers were already returned).
	scratch *om2CompositeScratch

	enableTypeAndUnitLabels bool
	ignoreNativeHistograms  bool

	// When true, a composite histogram that exposes both native fields and a
	// classic bucket list (or a classic histogram converted to NHCB) emits
	// the native histogram followed by classic _count/_sum/_bucket flat
	// series. Mirrors the protobuf parser's
	// KeepClassicOnClassicAndNativeHistograms option.
	keepClassicOnNativeHist bool

	// When true, classic histograms (without native schema/spans) are
	// converted directly to native histograms with custom buckets (NHCB).
	convertClassicHistToNHCB bool
	tmpNHCB                  convertnhcb.TempHistogram
}

// NewOpenMetrics2Parser returns a new parser for the OpenMetrics 2.0 text
// format.
func NewOpenMetrics2Parser(b []byte, st *labels.SymbolTable, opts ParserOptions) Parser {
	builder := labels.NewScratchBuilderWithSymbolTable(st, 16)
	builder.SetUnsafeAdd(true)
	p := &openMetrics2Parser{
		l:                        &openMetrics2Lexer{b: b},
		builder:                  builder,
		enableTypeAndUnitLabels:  opts.EnableTypeAndUnitLabels,
		ignoreNativeHistograms:   opts.IgnoreNativeHistograms,
		keepClassicOnNativeHist:  opts.KeepClassicOnClassicAndNativeHistograms,
		convertClassicHistToNHCB: opts.ConvertClassicHistogramsToNHCB,
	}
	if opts.ConvertClassicHistogramsToNHCB {
		p.tmpNHCB = convertnhcb.NewTempHistogram()
	}
	return p
}

// resetOnFamilyChange resets mtype/unit when name differs from curFamilyName.
func (p *openMetrics2Parser) resetOnFamilyChange(name []byte) {
	if !bytes.Equal(name, p.curFamilyName) {
		p.mtype = model.MetricTypeUnknown
		p.unit = ""
		// Only update curFamilyName when the family changes to avoid a pointer
		// write barrier on every sample within the same family.
		p.curFamilyName = name
	}
}

// hasFamilyNameLabel reports whether the current sample's labelsinclude a label key matching
// p.curFamilyName. Used to enforce the OM2 stateset requirements.
func (p *openMetrics2Parser) hasFamilyNameLabel() bool {
	for i := 2; i < len(p.offsets); i += 4 {
		if bytes.Equal(p.l.b[p.offsets[i]:p.offsets[i+1]], p.curFamilyName) {
			return true
		}
	}
	return false
}

// Series returns the bytes of the current series, the timestamp if set, and
// the sample value.
func (p *openMetrics2Parser) Series() ([]byte, *int64, float64) {
	if p.pendingIdx > 0 {
		pe := p.pending[p.pendingIdx-1]
		series := p.seriesBuf[pe.seriesStart:pe.seriesEnd:pe.seriesEnd]
		if p.hasTS {
			return series, &p.ts, pe.val
		}
		return series, nil, pe.val
	}
	if p.hasTS {
		return p.series, &p.ts, p.val
	}
	return p.series, nil, p.val
}

// Histogram returns the bytes of the current series, the timestamp if set,
// and the native histogram value.
func (p *openMetrics2Parser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	if p.hasTS {
		return p.series, &p.ts, p.h, p.fh
	}
	return p.series, nil, p.h, p.fh
}

// Help returns the metric name and help text of the current entry.
// Must only be called after Next returned EntryHelp.
func (p *openMetrics2Parser) Help() ([]byte, []byte) {
	m := p.l.b[p.offsets[0]:p.offsets[1]]
	if bytes.IndexByte(p.text, byte('\\')) >= 0 {
		return m, []byte(lvalReplacer.Replace(string(p.text)))
	}
	return m, p.text
}

// Type returns the metric name and type of the current entry.
// Must only be called after Next returned EntryType.
func (p *openMetrics2Parser) Type() ([]byte, model.MetricType) {
	return p.l.b[p.offsets[0]:p.offsets[1]], p.mtype
}

// Unit returns the metric name and unit of the current entry.
// Must only be called after Next returned EntryUnit.
func (p *openMetrics2Parser) Unit() ([]byte, []byte) {
	return p.l.b[p.offsets[0]:p.offsets[1]], []byte(p.unit)
}

// Comment returns the text of the current comment.
// Must only be called after Next returned EntryComment.
func (p *openMetrics2Parser) Comment() []byte {
	return p.text
}

// Labels writes the labels of the current sample into l.
func (p *openMetrics2Parser) Labels(l *labels.Labels) {
	if p.pendingIdx > 0 {
		pe := p.pending[p.pendingIdx-1]
		// p.builder has SetUnsafeAdd(true), so stringlabels and dedupelabels
		// copy/intern strings inside Labels() and slicelabels clones inside Add().
		s := yoloString(p.seriesBuf[pe.seriesStart:pe.seriesEnd])
		p.builder.Reset()
		metricName := unreplace(s[:pe.nameLen])
		m := schema.Metadata{
			Name: metricName,
			Type: p.mtype,
			Unit: p.unit,
		}
		pos := pe.nameLen
		if p.enableTypeAndUnitLabels {
			if !m.IsTypeEmpty() {
				pos += 1 + len(model.MetricTypeLabel) + 1 + len(p.mtype)
			}
			if p.unit != "" {
				pos += 1 + len(model.MetricUnitLabel) + 1 + len(p.unit)
			}
			m.AddToLabels(&p.builder)
		} else {
			p.builder.Add(model.MetricNameLabel, metricName)
		}
		for i := 2; i < len(p.offsets); i += 4 {
			pos++ // skip SeparatorByte
			kLen := p.offsets[i+1] - p.offsets[i]
			label := unreplace(s[pos : pos+kLen])
			pos += kLen + 1 // skip key and SeparatorByte
			vLen := p.offsets[i+3] - p.offsets[i+2]
			val := unreplace(s[pos : pos+vLen])
			pos += vLen
			if p.enableTypeAndUnitLabels && !m.IsEmptyFor(label) {
				continue
			}
			p.builder.Add(label, val)
		}
		if pe.injectKey != "" {
			pos += 1 + len(pe.injectKey) + 1 // skip SeparatorByte, injectKey, SeparatorByte
			p.builder.Add(pe.injectKey, s[pos:])
		}
		p.builder.Sort()
		*l = p.builder.Labels()
		return
	}
	// p.builder has SetUnsafeAdd(true), so yoloString is safe across all labels
	// implementations without allocating an intermediate string copy.
	s := yoloString(p.series)
	p.builder.Reset()
	metricName := unreplace(s[p.offsets[0]-p.start : p.offsets[1]-p.start])
	m := schema.Metadata{
		Name: metricName,
		Type: p.mtype,
		Unit: p.unit,
	}
	if p.enableTypeAndUnitLabels {
		m.AddToLabels(&p.builder)
	} else {
		p.builder.Add(model.MetricNameLabel, metricName)
	}
	for i := 2; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		label := unreplace(s[a:b])
		if p.enableTypeAndUnitLabels && !m.IsEmptyFor(label) {
			continue
		}
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start
		v := normalizeFloatsInLabelValues(p.mtype, label, unreplace(s[c:d]))
		p.builder.Add(label, v)
	}
	p.builder.Sort()
	*l = p.builder.Labels()
}

// Exemplar writes the next exemplar of the current sample into e and returns
// true.  Returns false when all exemplars have been consumed.
func (p *openMetrics2Parser) Exemplar(e *exemplar.Exemplar) bool {
	if p.exemplarIdx >= len(p.exemplars) {
		return false
	}
	ex := p.exemplars[p.exemplarIdx]
	p.exemplarIdx++

	e.Value = ex.val
	e.HasTs = true
	e.Ts = ex.ts
	p.builder.Reset()
	if ex.offsetsStart < ex.offsetsEnd {
		base := p.eOffsets[ex.offsetsStart]
		s := yoloString(p.l.b[base:p.eOffsets[ex.offsetsEnd-1]])
		for i := ex.offsetsStart; i < ex.offsetsEnd; i += 4 {
			a := p.eOffsets[i] - base
			b := p.eOffsets[i+1] - base
			c := p.eOffsets[i+2] - base
			d := p.eOffsets[i+3] - base
			p.builder.Add(unreplace(s[a:b]), unreplace(s[c:d]))
		}
		p.builder.Sort()
	}
	e.Labels = p.builder.Labels()
	return true
}

// StartTimestamp returns the inline start timestamp for the current sample
// (from the "st@<ts>" token), or 0 if none was present.  This is O(1); there
// is no forward scan.
func (p *openMetrics2Parser) StartTimestamp() int64 {
	if p.hasST {
		return p.st
	}
	return 0
}

func (p *openMetrics2Parser) nextToken() token {
	return p.l.Lex()
}

func (p *openMetrics2Parser) parseError(exp string, got token) error {
	e := min(len(p.l.b), p.l.i+1)
	if got == tInvalid && bytes.IndexByte(p.l.b[p.l.start:e], '\r') >= 0 {
		return fmt.Errorf("unexpected carriage return, got %q (%q) while parsing: %q", p.l.b[p.l.start:e], got, p.l.b[p.start:e])
	}
	return fmt.Errorf("%s, got %q (%q) while parsing: %q", exp, p.l.b[p.l.start:e], got, p.l.b[p.start:e])
}

// Next advances the parser to the next entry.
// It returns (EntryInvalid, io.EOF) when there are no more entries.
func (p *openMetrics2Parser) Next() (Entry, error) {
	// Drain pending composite-value entries.
	if p.pendingIdx < len(p.pending) {
		p.pendingIdx++
		return EntrySeries, nil
	}
	if len(p.pending) > 0 {
		p.pending = p.pending[:0]
		p.pendingIdx = 0
		p.seriesBuf = p.seriesBuf[:0]
	}

	var err error
	p.start = p.l.i
	p.offsets = p.offsets[:0]
	if len(p.exemplars) > 0 {
		p.exemplars = p.exemplars[:0]
		p.eOffsets = p.eOffsets[:0]
		p.exemplarIdx = 0
	}
	p.hasTS = false
	p.hasST = false

	switch t := p.nextToken(); t {
	case tEOFWord:
		if t := p.nextToken(); t != tEOF {
			return EntryInvalid, errors.New("unexpected data after # EOF")
		}
		// All pending entries were served above, so nothing references the
		// composite buffers anymore and they can go back to the pool. On
		// parse errors they are simply dropped together with the parser.
		p.releaseCompositeScratch()
		return EntryInvalid, io.EOF
	case tEOF:
		return EntryInvalid, errors.New("data does not end with # EOF")

	case tHelp, tType, tUnit:
		switch t2 := p.nextToken(); t2 {
		case tMName:
			mStart := p.l.start
			mEnd := p.l.i
			if p.l.b[mStart] == '"' && p.l.b[mEnd-1] == '"' {
				mStart++
				mEnd--
				if mStart == mEnd {
					return EntryInvalid, errors.New("metric name must not be empty")
				}
				if !utf8.Valid(p.l.b[mStart:mEnd]) {
					return EntryInvalid, fmt.Errorf("invalid UTF-8 metric name: %q", p.l.b[mStart:mEnd])
				}
			}
			p.offsets = append(p.offsets, mStart, mEnd)
			p.resetOnFamilyChange(p.l.b[mStart:mEnd])
		default:
			return EntryInvalid, p.parseError("expected metric name after "+t.String(), t2)
		}
		switch t2 := p.nextToken(); t2 {
		case tText:
			if len(p.l.buf()) > 1 {
				p.text = p.l.buf()[1 : len(p.l.buf())-1]
			} else {
				p.text = []byte{}
			}
			if len(p.text) > 0 && p.text[len(p.text)-1] == '\r' {
				return EntryInvalid, fmt.Errorf("unexpected carriage return in %s: %q", t.String(), p.text)
			}
		default:
			return EntryInvalid, fmt.Errorf("expected text in %s", t.String())
		}
		switch t {
		case tType:
			switch s := yoloString(p.text); s {
			case "counter":
				p.mtype = model.MetricTypeCounter
			case "gauge":
				p.mtype = model.MetricTypeGauge
			case "histogram":
				p.mtype = model.MetricTypeHistogram
			case "gaugehistogram":
				p.mtype = model.MetricTypeGaugeHistogram
			case "summary":
				p.mtype = model.MetricTypeSummary
			case "info":
				if !bytes.HasSuffix(p.curFamilyName, []byte("_info")) {
					return EntryInvalid, fmt.Errorf("info metric family name %q must end with _info", p.curFamilyName)
				}
				p.mtype = model.MetricTypeInfo
			case "stateset":
				p.mtype = model.MetricTypeStateset
			case "unknown":
				p.mtype = model.MetricTypeUnknown
			default:
				return EntryInvalid, fmt.Errorf("invalid metric type %q", s)
			}
		case tHelp:
			if !utf8.Valid(p.text) {
				return EntryInvalid, fmt.Errorf("help text %q is not a valid utf8 string", p.text)
			}
		}
		switch t {
		case tHelp:
			return EntryHelp, nil
		case tType:
			if (p.mtype == model.MetricTypeInfo || p.mtype == model.MetricTypeStateset) && p.unit != "" {
				return EntryInvalid, fmt.Errorf("%s metric %q must have an empty unit, got %q", p.mtype, p.curFamilyName, p.unit)
			}
			return EntryType, nil
		case tUnit:
			// OM2 only RECOMMENDS the unit be an underscore-separated suffix
			// of the MetricFamily name; it is not a hard requirement.
			p.unit = string(p.text)
			if (p.mtype == model.MetricTypeInfo || p.mtype == model.MetricTypeStateset) && p.unit != "" {
				return EntryInvalid, fmt.Errorf("%s metric %q must have an empty unit, got %q", p.mtype, p.curFamilyName, p.unit)
			}
			return EntryUnit, nil
		}

	case tBraceOpen:
		// UTF-8 metric name inside braces.
		if len(p.offsets) == 0 {
			p.offsets = []int{-1, -1}
		}
		if p.offsets, err = p.parseLVals(p.offsets, false); err != nil {
			return EntryInvalid, err
		}
		if p.offsets[0] != -1 {
			p.resetOnFamilyChange(p.l.b[p.offsets[0]:p.offsets[1]])
		}
		p.series = p.l.b[p.start:p.l.i]
		return p.parseSeriesEndOfLine(p.nextToken())

	case tMName:
		p.offsets = append(p.offsets, p.start, p.l.i)
		p.resetOnFamilyChange(p.l.b[p.start:p.l.i])
		p.series = p.l.b[p.start:p.l.i]

		t2 := p.nextToken()
		if t2 == tBraceOpen {
			p.offsets, err = p.parseLVals(p.offsets, false)
			if err != nil {
				return EntryInvalid, err
			}
			p.series = p.l.b[p.start:p.l.i]
			t2 = p.nextToken()
		}
		return p.parseSeriesEndOfLine(t2)

	default:
		err = p.parseError("expected a valid start token", t)
	}
	return EntryInvalid, err
}

// parseSeriesEndOfLine parses the rest of a data line starting from the value
// token.  It dispatches to composite or scalar value parsing.
func (p *openMetrics2Parser) parseSeriesEndOfLine(t token) (Entry, error) {
	if p.offsets[0] == -1 {
		return EntryInvalid, fmt.Errorf("metric name not set while parsing: %q", p.l.b[p.start:p.l.i])
	}
	if t == tCompOpen {
		return p.parseCompositeValue()
	}
	if t != tValue {
		return EntryInvalid, p.parseError("expected value after metric", t)
	}

	// Plain float value; strip the leading space.
	raw := p.l.buf()
	if len(raw) > 1 && raw[0] == ' ' {
		raw = raw[1:]
	}
	var err error
	p.val, err = parseFloat(yoloString(raw))
	if err != nil {
		return EntryInvalid, fmt.Errorf("%w while parsing: %q", err, p.l.b[p.start:p.l.i])
	}
	if math.IsNaN(p.val) {
		if p.mtype == model.MetricTypeCounter {
			return EntryInvalid, fmt.Errorf("counter sample value must not be NaN while parsing: %q", p.l.b[p.start:p.l.i])
		}
		p.val = math.Float64frombits(value.NormalNaN)
	}

	switch p.mtype {
	case model.MetricTypeHistogram, model.MetricTypeGaugeHistogram, model.MetricTypeSummary:
		// Histogram, GaugeHistogram, and Summary Samples MUST use a composite
		// value.
		return EntryInvalid, fmt.Errorf(
			"composite value required for metric type %q while parsing: %q",
			p.mtype, p.l.b[p.start:p.l.i],
		)
	case model.MetricTypeInfo:
		if p.val != 1 {
			return EntryInvalid, fmt.Errorf("info sample value must be 1, got %v while parsing: %q", p.val, p.l.b[p.start:p.l.i])
		}
	case model.MetricTypeStateset:
		if p.val != 0 && p.val != 1 {
			return EntryInvalid, fmt.Errorf("stateset sample value must be 0 or 1, got %v while parsing: %q", p.val, p.l.b[p.start:p.l.i])
		}
		if !p.hasFamilyNameLabel() {
			return EntryInvalid, fmt.Errorf("stateset sample must have a label matching the metric family name %q while parsing: %q", p.curFamilyName, p.l.b[p.start:p.l.i])
		}
	}

	// Fast path: plain sample lines end immediately with a linebreak.
	t2 := p.nextToken()
	if t2 == tLinebreak {
		return EntrySeries, nil
	}
	if err := p.parseAfterValue(t2); err != nil {
		return EntryInvalid, err
	}
	return EntrySeries, nil
}

// parseAfterValue consumes the optional sequence after the metric value:
//
//	[tTimestamp] [tStartTimestamp] [*tComment exemplar] tLinebreak
//
// starting from the already-read token t. It returns after consuming tLinebreak
// (either directly or via exemplar parsing).
func (p *openMetrics2Parser) parseAfterValue(t token) error {
	for {
		switch t {
		case tEOF:
			return errors.New("data does not end with # EOF")
		case tLinebreak:
			return nil
		case tTimestamp:
			if p.hasTS {
				return fmt.Errorf("duplicate timestamp: %q", p.l.b[p.start:p.l.i])
			}
			if p.hasST {
				return fmt.Errorf("timestamp must precede start timestamp: %q", p.l.b[p.start:p.l.i])
			}
			p.hasTS = true
			var ts float64
			var err error
			if ts, err = parseFloat(yoloString(p.l.buf()[1:])); err != nil {
				return fmt.Errorf("%w while parsing: %q", err, p.l.b[p.start:p.l.i])
			}
			if math.IsNaN(ts) || math.IsInf(ts, 0) {
				return fmt.Errorf("invalid timestamp %f", ts)
			}
			p.ts = int64(ts * 1000)
		case tStartTimestamp:
			if p.hasST {
				return fmt.Errorf("duplicate start timestamp: %q", p.l.b[p.start:p.l.i])
			}
			// buf is " st@<float>"; skip the leading " st@" (4 bytes).
			raw := p.l.buf()
			if len(raw) < 4 {
				return fmt.Errorf("invalid start timestamp token %q", raw)
			}
			var st float64
			var err error
			if st, err = parseFloat(yoloString(raw[4:])); err != nil {
				return fmt.Errorf("%w while parsing start timestamp: %q", err, p.l.b[p.start:p.l.i])
			}
			if math.IsNaN(st) || math.IsInf(st, 0) {
				return fmt.Errorf("invalid start timestamp %f", st)
			}
			p.st = int64(st * 1000)
			p.hasST = true
		case tComment:
			// Parse all exemplars on this line; parseExemplars consumes
			// up to and including the tLinebreak.
			return p.parseExemplars()
		default:
			return p.parseError("unexpected token after value", t)
		}
		t = p.nextToken()
	}
}

// parseExemplars parses one or more exemplars up to and including the
// tLinebreak.  It is called after tComment has been consumed.
func (p *openMetrics2Parser) parseExemplars() error {
	for {
		done, err := p.parseSingleExemplar()
		if err != nil {
			return err
		}
		if done {
			// tLinebreak was consumed inside parseSingleExemplar.
			return nil
		}
		// tComment was consumed; another exemplar follows.
	}
}

// parseSingleExemplar parses one exemplar label set + value + optional
// timestamp.  It reads one token after (from sETimestamp state):
//   - tLinebreak → done=true
//   - tComment   → done=false (caller loops for next exemplar)
func (p *openMetrics2Parser) parseSingleExemplar() (done bool, err error) {
	offsetsStart := len(p.eOffsets)

	// Parse exemplar label set (the "{" was opened by the tComment token) into
	// the shared p.eOffsets slice so Exemplar() can build labels lazily.
	p.eOffsets, err = p.parseLVals(p.eOffsets, true)
	if err != nil {
		return false, err
	}
	offsetsEnd := len(p.eOffsets)

	// Parse exemplar value.
	if t := p.nextToken(); t != tValue {
		return false, p.parseError("expected exemplar value", t)
	}
	val, err := parseFloat(yoloString(p.l.buf()[1:]))
	if err != nil {
		return false, fmt.Errorf("%w while parsing exemplar value: %q", err, p.l.b[p.start:p.l.i])
	}
	if math.IsNaN(val) {
		val = math.Float64frombits(value.NormalNaN)
	}

	// Read the token following the exemplar value.  OM2 requires every
	// exemplar to carry a timestamp, so anything other than tTimestamp here
	// (including end-of-line or the start of another exemplar) is an error.
	switch t2 := p.nextToken(); t2 {
	case tEOF:
		return false, errors.New("data does not end with # EOF")
	case tTimestamp:
		var ts float64
		if ts, err = parseFloat(yoloString(p.l.buf()[1:])); err != nil {
			return false, fmt.Errorf("%w while parsing exemplar timestamp: %q", err, p.l.b[p.start:p.l.i])
		}
		if math.IsNaN(ts) || math.IsInf(ts, 0) {
			return false, fmt.Errorf("invalid exemplar timestamp %f", ts)
		}
		p.exemplars = append(p.exemplars, om2Exemplar{
			offsetsStart: offsetsStart,
			offsetsEnd:   offsetsEnd,
			val:          val,
			ts:           int64(ts * 1000),
		})
		// After the exemplar timestamp, the line may end (tLinebreak) or
		// another exemplar may follow (tComment).
		switch t3 := p.nextToken(); t3 {
		case tEOF:
			return false, errors.New("data does not end with # EOF")
		case tLinebreak:
			return true, nil
		case tComment:
			return false, nil // caller loops
		default:
			return false, p.parseError("expected end of line or next exemplar", t3)
		}
	default:
		return false, p.parseError("expected exemplar timestamp", t2)
	}
}

// parseLVals parses the label set "{k="v",...}" and appends byte offsets.
func (p *openMetrics2Parser) parseLVals(offsets []int, isExemplar bool) ([]int, error) {
	t := p.nextToken()
	first := true
	for {
		isFirst := first
		first = false
		curTStart := p.l.start
		curTI := p.l.i
		var isQString bool
		switch t {
		case tBraceClose:
			return offsets, nil
		case tLName:
		case tQString:
			isQString = true
		default:
			return nil, p.parseError("expected label name", t)
		}

		t = p.nextToken()
		if isQString {
			if t == tComma || t == tBraceClose {
				if isExemplar {
					return nil, p.parseError("expected label name", t)
				}
				if !isFirst {
					return nil, errors.New("metric name must be the first item in the label set")
				}
				if offsets[0] != -1 || offsets[1] != -1 {
					return nil, fmt.Errorf("metric name already set while parsing: %q", p.l.b[p.start:p.l.i])
				}
				offsets[0] = curTStart + 1
				offsets[1] = curTI - 1
				if offsets[0] == offsets[1] {
					return nil, errors.New("metric name must not be empty")
				}
				if !utf8.Valid(p.l.b[offsets[0]:offsets[1]]) {
					return nil, fmt.Errorf("invalid UTF-8 metric name: %q", p.l.b[offsets[0]:offsets[1]])
				}
				if t == tBraceClose {
					return offsets, nil
				}
				t = p.nextToken()
				continue
			}
			// Quoted label name: strip surrounding quotes and validate non-empty UTF-8.
			// Unquoted tLName tokens are already guaranteed by the lexer to be non-empty ASCII.
			curTStart++
			curTI--
			if curTStart == curTI {
				return nil, errors.New("label name must not be empty")
			}
			if !utf8.Valid(p.l.b[curTStart:curTI]) {
				return nil, fmt.Errorf("invalid UTF-8 label name: %q", p.l.b[curTStart:curTI])
			}
		}
		offsets = append(offsets, curTStart, curTI)

		if t != tEqual {
			return nil, p.parseError("expected equal", t)
		}
		if t := p.nextToken(); t != tLValue {
			return nil, p.parseError("expected label value", t)
		}
		if !utf8.Valid(p.l.buf()) {
			return nil, fmt.Errorf("invalid UTF-8 label value: %q", p.l.buf())
		}
		offsets = append(offsets, p.l.start+1, p.l.i-1)

		t = p.nextToken()
		if t == tComma {
			t = p.nextToken()
		} else if t != tBraceClose {
			return nil, p.parseError("expected comma or brace close", t)
		}
	}
}

// parseCompositeValue dispatches on the current metric type and builds either
// a native histogram (EntryHistogram) or a list of pending flat series
// (EntrySeries) from the composite value tokens.
func (p *openMetrics2Parser) parseCompositeValue() (Entry, error) {
	switch p.mtype {
	case model.MetricTypeHistogram, model.MetricTypeGaugeHistogram:
		return p.parseHistogramComposite()
	case model.MetricTypeSummary:
		return p.parseSummaryComposite()
	default:
		return EntryInvalid, fmt.Errorf(
			"composite value not supported for metric type %q while parsing: %q",
			p.mtype, p.l.b[p.start:p.l.i],
		)
	}
}

// Composite field indices (ordered to match tCompCount..tCompQuantile).
const (
	compFieldCount = iota
	compFieldGCount
	compFieldSum
	compFieldGSum
	compFieldSchema
	compFieldZeroThreshold
	compFieldZeroCount
	compFieldNegativeSpans
	compFieldNegativeBuckets
	compFieldPositiveSpans
	compFieldPositiveBuckets
	compFieldBucket
	compFieldQuantile
	numCompFields
)

const (
	histogramCommonMask = (1 << compFieldSchema) | (1 << compFieldZeroThreshold) | (1 << compFieldZeroCount) |
		(1 << compFieldNegativeSpans) | (1 << compFieldNegativeBuckets) |
		(1 << compFieldPositiveSpans) | (1 << compFieldPositiveBuckets) |
		(1 << compFieldBucket)

	histogramAllowedMask      = histogramCommonMask | (1 << compFieldCount) | (1 << compFieldSum)
	gaugeHistogramAllowedMask = histogramCommonMask | (1 << compFieldGCount) | (1 << compFieldGSum)

	summaryAllowedMask = (1 << compFieldCount) | (1 << compFieldSum) | (1 << compFieldQuantile)
)

type compositeFields struct {
	fields [numCompFields][]byte
	seen   uint16
}

func (cf *compositeFields) has(field int) bool {
	return cf.seen&(1<<field) != 0
}

func (cf *compositeFields) get(field int) ([]byte, bool) {
	return cf.fields[field], cf.has(field)
}

func (p *openMetrics2Parser) parseCompositeFields(allowedMask uint16) (compositeFields, error) {
	var cf compositeFields
	t := p.nextToken()
	if t == tCompClose {
		return cf, nil
	}
	for {
		idx := int(t - tCompCount)
		if idx < 0 || idx >= numCompFields || allowedMask&(1<<idx) == 0 {
			return cf, p.parseError("unknown composite field", t)
		}
		if cf.has(idx) {
			return cf, p.parseError("duplicate composite field", t)
		}
		if t := p.nextToken(); t != tValue {
			return cf, p.parseError("expected composite value", t)
		}
		cf.seen |= 1 << idx
		cf.fields[idx] = p.l.buf()

		switch t = p.nextToken(); t {
		case tComma:
			t = p.nextToken()
		case tCompClose:
			return cf, nil
		default:
			return cf, p.parseError("expected comma or brace close in composite value", t)
		}
	}
}

func (p *openMetrics2Parser) parseHistogramFields(isGauge bool) (compositeFields, error) {
	allowedMask := uint16(histogramAllowedMask)
	countField, sumField := compFieldCount, compFieldSum
	countKey, sumKey := "count", "sum"
	if isGauge {
		allowedMask = gaugeHistogramAllowedMask
		countField, sumField = compFieldGCount, compFieldGSum
		countKey, sumKey = "gcount", "gsum"
	}
	cf, err := p.parseCompositeFields(allowedMask)
	if err != nil {
		return cf, err
	}
	if !cf.has(countField) {
		return cf, fmt.Errorf("missing required field: %s", countKey)
	}
	if !cf.has(sumField) {
		return cf, fmt.Errorf("missing required field: %s", sumKey)
	}

	if cf.has(compFieldSchema) {
		if !cf.has(compFieldZeroThreshold) {
			return cf, errors.New("missing required field: zero_threshold")
		}
		if !cf.has(compFieldZeroCount) {
			return cf, errors.New("missing required field: zero_count")
		}
	} else if !cf.has(compFieldBucket) {
		return cf, errors.New("missing required field: bucket")
	}

	return cf, nil
}

func (p *openMetrics2Parser) parseSummaryFields() (compositeFields, error) {
	cf, err := p.parseCompositeFields(summaryAllowedMask)
	if err != nil {
		return cf, err
	}
	if !cf.has(compFieldCount) {
		return cf, errors.New("missing required field: count")
	}
	if !cf.has(compFieldSum) {
		return cf, errors.New("missing required field: sum")
	}
	if !cf.has(compFieldQuantile) {
		return cf, errors.New("missing required field: quantile")
	}
	return cf, nil
}

// parseHistogramComposite parses a composite histogram value such as:
//
//	{count:12,sum:5.5,schema:0,zero_threshold:0.001,zero_count:2,
//	 negative_spans:[],negative_buckets:[],
//	 positive_spans:[0:3,2:1],positive_buckets:[1,2,1,3]}
//
// or a classic histogram:
//
//	{count:12,sum:5.5,bucket:[1.0:3,2.0:7,+Inf:12]}
//
// When native buckets are present it returns EntryHistogram.  Otherwise it
// populates the pending queue and returns EntrySeries for the first entry.
func (p *openMetrics2Parser) parseHistogramComposite() (Entry, error) {
	isGauge := p.mtype == model.MetricTypeGaugeHistogram
	cf, err := p.parseHistogramFields(isGauge)
	if err != nil {
		return EntryInvalid, err
	}
	// Consume the rest of the line (timestamp, st@, exemplars) before building
	// the pending entries, so the exemplar and ST fields are set correctly.
	if t2 := p.nextToken(); t2 != tLinebreak {
		if err := p.parseAfterValue(t2); err != nil {
			return EntryInvalid, err
		}
	}

	// schema is mandatory for native histograms and absent from classic
	// ones, so its presence is the sole reliable signal.
	isNative := cf.has(compFieldSchema)

	if isNative && !p.ignoreNativeHistograms {
		h, fh, err := buildNativeHistogram(cf, isGauge)
		if err != nil {
			return EntryInvalid, fmt.Errorf("error parsing native histogram composite: %w", err)
		}
		p.h = h
		p.fh = fh
		// When the composite also carries a classic bucket list and the
		// caller asked to keep it, queue the classic flat series so that
		// subsequent Next() calls drain them after the EntryHistogram.
		if cf.has(compFieldBucket) && p.keepClassicOnNativeHist {
			pending, err := p.buildClassicHistogramPending(cf, false)
			if err != nil {
				return EntryInvalid, fmt.Errorf("error parsing classic histogram composite: %w", err)
			}
			p.pending = pending
			p.pendingIdx = 0
		}
		return EntryHistogram, nil
	}

	if p.convertClassicHistToNHCB {
		h, fh, err := p.buildNHCBHistogram(cf, isNative)
		if err != nil {
			return EntryInvalid, fmt.Errorf("error parsing classic histogram composite: %w", err)
		}
		p.h = h
		p.fh = fh
		if p.keepClassicOnNativeHist {
			pending, err := p.buildClassicHistogramPending(cf, isNative)
			if err != nil {
				return EntryInvalid, fmt.Errorf("error parsing classic histogram composite: %w", err)
			}
			p.pending = pending
			p.pendingIdx = 0
		}
		return EntryHistogram, nil
	}

	// Classic histogram (or native histogram with ignoreNativeHistograms): explode into flat pending entries.
	pending, err := p.buildClassicHistogramPending(cf, isNative)
	if err != nil {
		return EntryInvalid, fmt.Errorf("error parsing classic histogram composite: %w", err)
	}
	return p.servePending(pending)
}

// parseSummaryComposite parses a composite summary value such as:
//
//	{count:12,sum:5.5,quantile:[0.5:1.0,0.9:2.0,0.99:3.0]}
func (p *openMetrics2Parser) parseSummaryComposite() (Entry, error) {
	cf, err := p.parseSummaryFields()
	if err != nil {
		return EntryInvalid, err
	}
	// Consume the rest of the line (timestamp, st@, exemplars) before building
	// the pending entries, so the exemplar and ST fields are set correctly.
	if t2 := p.nextToken(); t2 != tLinebreak {
		if err := p.parseAfterValue(t2); err != nil {
			return EntryInvalid, err
		}
	}

	pending, err := p.buildSummaryPending(cf)
	if err != nil {
		return EntryInvalid, fmt.Errorf("error parsing summary composite: %w", err)
	}
	return p.servePending(pending)
}

// servePending stores pending and returns the first entry.
func (p *openMetrics2Parser) servePending(pending []pendingEntry) (Entry, error) {
	if len(pending) == 0 {
		return EntryInvalid, errors.New("composite value produced no series")
	}
	p.pending = pending
	p.pendingIdx = 1 // we are about to serve pending[0]
	return EntrySeries, nil
}

func buildNativeHistogram(cf compositeFields, isGauge bool) (*histogram.Histogram, *histogram.FloatHistogram, error) {
	getFloat := func(field int) (float64, bool, error) {
		v, ok := cf.get(field)
		if !ok {
			return 0, false, nil
		}
		f, err := strconv.ParseFloat(yoloString(v), 64)
		return f, true, err
	}
	getInt := func(field int) (int64, bool, error) {
		v, ok := cf.get(field)
		if !ok {
			return 0, false, nil
		}
		n, err := strconv.ParseInt(yoloString(v), 10, 64)
		return n, true, err
	}

	// schema is guaranteed present: the caller only reaches buildNativeHistogram
	// when compFieldSchema exists.
	schema64, _, err := getInt(compFieldSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid schema: %w", err)
	}
	// Schema values outside [-4, 8] are reserved for future use and MUST NOT
	// be used.
	if schema64 < -4 || schema64 > 8 {
		return nil, nil, fmt.Errorf("schema must be between -4 and 8, got %d", schema64)
	}

	countField, sumField := compFieldCount, compFieldSum
	countKey, sumKey := "count", "sum"
	if isGauge {
		countField, sumField = compFieldGCount, compFieldGSum
		countKey, sumKey = "gcount", "gsum"
	}
	count, ok, err := getFloat(countField)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid %s: %w", countKey, err)
	}
	if !ok {
		return nil, nil, fmt.Errorf("missing required field: %s", countKey)
	}
	// A negative count wraps around in the conversion to uint64 on the integer
	// path below, and Validate only catches the result when sum is not NaN: a
	// NaN sum relaxes its check to "count is at least the bucket total", which
	// a wrapped count always satisfies.
	// The spec does permit a negative gcount, but neither histogram type can
	// hold one: Count is a uint64 in the integer form, and Validate rejects a
	// negative count in the float form.
	if count < 0 {
		return nil, nil, fmt.Errorf("%s must not be negative, got %v", countKey, count)
	}
	sum, ok, err := getFloat(sumField)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid %s: %w", sumKey, err)
	}
	if !ok {
		return nil, nil, fmt.Errorf("missing required field: %s", sumKey)
	}
	zeroThreshold, ok, err := getFloat(compFieldZeroThreshold)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid zero_threshold: %w", err)
	}
	if !ok {
		return nil, nil, errors.New("missing required field: zero_threshold")
	}

	if math.IsNaN(zeroThreshold) || math.IsInf(zeroThreshold, 0) || zeroThreshold < 0 {
		return nil, nil, fmt.Errorf("zero_threshold must be a non-negative, finite number, got %v", zeroThreshold)
	}
	zeroCount, ok, err := getFloat(compFieldZeroCount)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid zero_count: %w", err)
	}
	if !ok {
		return nil, nil, errors.New("missing required field: zero_count")
	}
	if zeroCount < 0 {
		return nil, nil, fmt.Errorf("zero_count must not be negative, got %v", zeroCount)
	}

	// Treat as FloatHistogram if count, zero_count, or any bucket value is non-integer.
	// sum is always float64 in both histogram types and does not determine the kind.
	// A count of MaxUint64 or more, +Inf included, has no uint64 representation and
	// would wrap around in the conversion below, so it takes the float form too.
	// Bucket strings are scanned for '.', 'e', or 'E' since OM2 emits floats with
	// decimals or exponent notation; the cheap pre-scan avoids parsing buckets twice.
	posBucketsRaw := cf.fields[compFieldPositiveBuckets]
	negBucketsRaw := cf.fields[compFieldNegativeBuckets]
	isFloat := count != math.Trunc(count) || zeroCount != math.Trunc(zeroCount) ||
		count >= float64(math.MaxUint64) || zeroCount >= float64(math.MaxUint64) ||
		bucketsHaveFloat(posBucketsRaw) || bucketsHaveFloat(negBucketsRaw)

	posSpans, err := parseSpans(cf.fields[compFieldPositiveSpans])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid positive_spans: %w", err)
	}
	negSpans, err := parseSpans(cf.fields[compFieldNegativeSpans])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid negative_spans: %w", err)
	}

	if isFloat {
		posBuckets, err := parseFloatBuckets(posBucketsRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid positive_buckets: %w", err)
		}
		negBuckets, err := parseFloatBuckets(negBucketsRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid negative_buckets: %w", err)
		}
		fh := &histogram.FloatHistogram{
			Schema:          int32(schema64),
			ZeroThreshold:   zeroThreshold,
			ZeroCount:       zeroCount,
			Count:           count,
			Sum:             sum,
			PositiveSpans:   posSpans,
			NegativeSpans:   negSpans,
			PositiveBuckets: posBuckets,
			NegativeBuckets: negBuckets,
		}
		if isGauge {
			fh.CounterResetHint = histogram.GaugeType
		}
		if err := fh.Validate(); err != nil {
			return nil, nil, fmt.Errorf("invalid float histogram: %w", err)
		}
		return nil, fh, nil
	}

	posBuckets, err := parseIntBuckets(posBucketsRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid positive_buckets: %w", err)
	}
	negBuckets, err := parseIntBuckets(negBucketsRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid negative_buckets: %w", err)
	}
	absoluteToIntDeltas(posBuckets)
	absoluteToIntDeltas(negBuckets)
	h := &histogram.Histogram{
		Schema:          int32(schema64),
		ZeroThreshold:   zeroThreshold,
		ZeroCount:       uint64(zeroCount),
		Count:           uint64(count),
		Sum:             sum,
		PositiveSpans:   posSpans,
		NegativeSpans:   negSpans,
		PositiveBuckets: posBuckets,
		NegativeBuckets: negBuckets,
	}
	if isGauge {
		h.CounterResetHint = histogram.GaugeType
	}
	if err := h.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid histogram: %w", err)
	}
	return h, nil, nil
}

// absoluteToIntDeltas converts a slice of absolute bucket counts (OM2 format)
// to the delta-encoded form expected by Prometheus internally. Operates in-place
// back-to-front to avoid overwriting values still needed.
func absoluteToIntDeltas(abs []int64) {
	for i := len(abs) - 1; i > 0; i-- {
		abs[i] -= abs[i-1]
	}
}

// parseSpans parses "[offset:length,...]" into a []histogram.Span.
func parseSpans(b []byte) ([]histogram.Span, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("[]")) {
		return nil, nil
	}
	if len(b) < 2 || b[0] != '[' || b[len(b)-1] != ']' {
		return nil, fmt.Errorf("spans must be wrapped in []: %q", b)
	}
	inner := bytes.TrimSpace(b[1 : len(b)-1])
	if len(inner) == 0 {
		return nil, nil
	}
	spans := make([]histogram.Span, 0, bytes.Count(inner, []byte{','})+1)
	for part := range bytes.SplitSeq(inner, []byte{','}) {
		part = bytes.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		before, after, ok := bytes.Cut(part, []byte{':'})
		if !ok {
			return nil, fmt.Errorf("span missing ':': %q", part)
		}
		offset, err := strconv.ParseInt(yoloString(bytes.TrimSpace(before)), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid span offset %q: %w", before, err)
		}
		length, err := strconv.ParseUint(yoloString(bytes.TrimSpace(after)), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid span length %q: %w", after, err)
		}
		spans = append(spans, histogram.Span{Offset: int32(offset), Length: uint32(length)})
	}
	return spans, nil
}

// bucketsHaveFloat reports whether the raw bucket bytes contain any non-integer
// value. OM2 formats floats with a decimal point or exponent, so a byte-level scan
// for '.', 'e', or 'E' is sufficient to discriminate integer from float buckets.
func bucketsHaveFloat(b []byte) bool {
	for _, ch := range b {
		switch ch {
		case '.', 'e', 'E':
			return true
		}
	}
	return false
}

// parseIntBuckets parses "[b1,b2,...]" into absolute []int64 bucket counts.
func parseIntBuckets(b []byte) ([]int64, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("[]")) {
		return nil, nil
	}
	if len(b) < 2 || b[0] != '[' || b[len(b)-1] != ']' {
		return nil, fmt.Errorf("buckets must be wrapped in []: %q", b)
	}
	inner := bytes.TrimSpace(b[1 : len(b)-1])
	if len(inner) == 0 {
		return nil, nil
	}
	buckets := make([]int64, 0, bytes.Count(inner, []byte{','})+1)
	for part := range bytes.SplitSeq(inner, []byte{','}) {
		part = bytes.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		v, err := strconv.ParseInt(yoloString(part), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid bucket %q: %w", part, err)
		}
		buckets = append(buckets, v)
	}
	return buckets, nil
}

// parseFloatBuckets parses "[b1,b2,...]" into []float64 bucket values.
func parseFloatBuckets(b []byte) ([]float64, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("[]")) {
		return nil, nil
	}
	if len(b) < 2 || b[0] != '[' || b[len(b)-1] != ']' {
		return nil, fmt.Errorf("float buckets must be wrapped in []: %q", b)
	}
	inner := bytes.TrimSpace(b[1 : len(b)-1])
	if len(inner) == 0 {
		return nil, nil
	}
	buckets := make([]float64, 0, bytes.Count(inner, []byte{','})+1)
	for part := range bytes.SplitSeq(inner, []byte{','}) {
		part = bytes.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(yoloString(part), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float bucket %q: %w", part, err)
		}
		buckets = append(buckets, v)
	}
	return buckets, nil
}

func (p *openMetrics2Parser) buildClassicHistogramPending(
	cf compositeFields,
	isNative bool,
) ([]pendingEntry, error) {
	// p.pending and p.seriesBuf have been reset to [:0] by Next() before any
	// parsing begins, so we reuse their backing arrays across composite parses.
	p.acquireCompositeScratch()
	pending := p.pending

	// GaugeHistogram Samples with Classic Buckets expose count/sum as
	// gcount/gsum, mirroring Count/Sum's role for a plain Histogram.
	// https://prometheus.io/docs/specs/om/open_metrics_spec_2_0/#gaugehistogram-1
	countField, sumField := compFieldCount, compFieldSum
	countKey, sumKey := "count", "sum"
	countSuffix, sumSuffix := "_count", "_sum"
	if p.mtype == model.MetricTypeGaugeHistogram {
		countField, sumField = compFieldGCount, compFieldGSum
		countKey, sumKey = "gcount", "gsum"
		countSuffix, sumSuffix = "_gcount", "_gsum"
	}

	cv, ok := cf.get(countField)
	if !ok {
		return nil, fmt.Errorf("missing required field: %s", countKey)
	}
	countVal, err := strconv.ParseFloat(yoloString(cv), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", countKey, err)
	}
	pending = append(pending, p.appendPendingSeriesBytes(countSuffix, "", 0, countVal))

	sv, ok := cf.get(sumField)
	if !ok {
		return nil, fmt.Errorf("missing required field: %s", sumKey)
	}
	v, err := strconv.ParseFloat(yoloString(sv), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", sumKey, err)
	}
	pending = append(pending, p.appendPendingSeriesBytes(sumSuffix, "", 0, v))

	// _bucket entries. The spec requires a Classic Bucket list to include a
	// +Inf threshold.
	bv, ok := cf.get(compFieldBucket)
	if !ok {
		if !isNative {
			return nil, errors.New("missing required field: bucket")
		}
		// When IgnoreNativeHistograms is enabled on a native-only histogram,
		// emit the +Inf bucket with the total count.
		pending = append(pending, p.appendPendingSeriesBytes("_bucket", "le", math.Inf(1), countVal))
		return pending, nil
	}
	hasPosInf := false
	for b, err := range parseBuckets(yoloString(bv)) {
		if err != nil {
			return nil, fmt.Errorf("invalid bucket: %w", err)
		}
		// Classic bucket values are counters for a histogram, which the spec
		// forbids from being negative. A gauge histogram's bucket values are
		// gauges, where the spec only discourages it.
		if b.count < 0 && p.mtype != model.MetricTypeGaugeHistogram {
			return nil, fmt.Errorf("invalid bucket: value must not be negative, got %v", b.count)
		}
		if math.IsInf(b.lef, 1) {
			hasPosInf = true
		}
		pending = append(pending, p.appendPendingSeriesBytes("_bucket", "le", b.lef, b.count))
	}
	if !hasPosInf {
		return nil, errors.New("classic histogram buckets must include a +Inf threshold")
	}

	return pending, nil
}

func (p *openMetrics2Parser) buildNHCBHistogram(cf compositeFields, isNative bool) (*histogram.Histogram, *histogram.FloatHistogram, error) {
	isGauge := p.mtype == model.MetricTypeGaugeHistogram
	countField, sumField := compFieldCount, compFieldSum
	countKey, sumKey := "count", "sum"
	if isGauge {
		countField, sumField = compFieldGCount, compFieldGSum
		countKey, sumKey = "gcount", "gsum"
	}

	cv, ok := cf.get(countField)
	if !ok {
		return nil, nil, fmt.Errorf("missing required field: %s", countKey)
	}
	count, err := strconv.ParseFloat(yoloString(cv), 64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid %s: %w", countKey, err)
	}

	sv, ok := cf.get(sumField)
	if !ok {
		return nil, nil, fmt.Errorf("missing required field: %s", sumKey)
	}
	sum, err := strconv.ParseFloat(yoloString(sv), 64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid %s: %w", sumKey, err)
	}

	p.tmpNHCB.Reset()
	if err := p.tmpNHCB.SetCount(count); err != nil {
		return nil, nil, err
	}
	if err := p.tmpNHCB.SetSum(sum); err != nil {
		return nil, nil, err
	}

	bv, ok := cf.get(compFieldBucket)
	if !ok {
		if !isNative {
			return nil, nil, errors.New("missing required field: bucket")
		}
	} else {
		hasPosInf := false
		for b, err := range parseBuckets(yoloString(bv)) {
			if err != nil {
				return nil, nil, fmt.Errorf("invalid bucket: %w", err)
			}
			if math.IsInf(b.lef, 1) {
				hasPosInf = true
			}
			if err := p.tmpNHCB.SetBucketCount(b.lef, b.count); err != nil {
				return nil, nil, err
			}
		}
		if !hasPosInf {
			return nil, nil, errors.New("classic histogram buckets must include a +Inf threshold")
		}
	}

	h, fh, err := p.tmpNHCB.Convert()
	if err != nil {
		return nil, nil, err
	}
	if isGauge {
		if h != nil {
			h.CounterResetHint = histogram.GaugeType
		} else {
			fh.CounterResetHint = histogram.GaugeType
		}
	}
	return h, fh, nil
}

// acquireCompositeScratch makes p.pending and p.seriesBuf point at pooled,
// empty backing arrays. It is a no-op once the parser owns a scratch.
func (p *openMetrics2Parser) acquireCompositeScratch() {
	if p.scratch != nil {
		return
	}
	p.scratch = om2CompositeScratchPool.Get().(*om2CompositeScratch)
	p.pending = p.scratch.pending[:0]
	p.seriesBuf = p.scratch.seriesBuf[:0]
}

// releaseCompositeScratch hands the (possibly grown) backing arrays back to
// the pool and detaches them from the parser, so a parser that is used after
// io.EOF cannot write into memory another parser may already own.
func (p *openMetrics2Parser) releaseCompositeScratch() {
	if p.scratch == nil {
		return
	}
	p.scratch.pending = p.pending[:0]
	p.scratch.seriesBuf = p.seriesBuf[:0]
	om2CompositeScratchPool.Put(p.scratch)
	p.scratch = nil
	p.pending = nil
	p.pendingIdx = 0
	p.seriesBuf = nil
}

func (p *openMetrics2Parser) buildSummaryPending(cf compositeFields) ([]pendingEntry, error) {
	// p.pending and p.seriesBuf have been reset to [:0] by Next() before any
	// parsing begins, so we reuse their backing arrays across composite parses.
	p.acquireCompositeScratch()
	pending := p.pending

	cv := cf.fields[compFieldCount]
	v, err := strconv.ParseFloat(yoloString(cv), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid count: %w", err)
	}
	pending = append(pending, p.appendPendingSeriesBytes("_count", "", 0, v))

	sv := cf.fields[compFieldSum]
	v, err = strconv.ParseFloat(yoloString(sv), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid sum: %w", err)
	}
	pending = append(pending, p.appendPendingSeriesBytes("_sum", "", 0, v))

	qv := cf.fields[compFieldQuantile]
	for q, err := range parseQuantiles(yoloString(qv)) {
		if err != nil {
			return nil, fmt.Errorf("invalid quantile: %w", err)
		}
		pending = append(pending, p.appendPendingSeriesBytes("", "quantile", q.qf, q.val))
	}

	return pending, nil
}

// appendPendingSeriesBytes formats a unique series byte identity for an
// exploded composite entry directly into p.seriesBuf from p.l.b and p.offsets
// without constructing labels.Labels. Labels() reconstructs the label set
// lazily from the appended bytes on a scrape series cache miss.
func (p *openMetrics2Parser) appendPendingSeriesBytes(suffix, injectKey string, injectVal, val float64) pendingEntry {
	start := len(p.seriesBuf)
	p.seriesBuf = append(p.seriesBuf, p.l.b[p.offsets[0]:p.offsets[1]]...)
	p.seriesBuf = append(p.seriesBuf, suffix...)
	nameLen := len(p.seriesBuf) - start
	if p.enableTypeAndUnitLabels {
		if p.mtype != "" && p.mtype != model.MetricTypeUnknown {
			p.seriesBuf = append(p.seriesBuf, model.SeparatorByte)
			p.seriesBuf = append(p.seriesBuf, model.MetricTypeLabel...)
			p.seriesBuf = append(p.seriesBuf, model.SeparatorByte)
			p.seriesBuf = append(p.seriesBuf, p.mtype...)
		}
		if p.unit != "" {
			p.seriesBuf = append(p.seriesBuf, model.SeparatorByte)
			p.seriesBuf = append(p.seriesBuf, model.MetricUnitLabel...)
			p.seriesBuf = append(p.seriesBuf, model.SeparatorByte)
			p.seriesBuf = append(p.seriesBuf, p.unit...)
		}
	}
	for i := 2; i < len(p.offsets); i += 4 {
		p.seriesBuf = append(p.seriesBuf, model.SeparatorByte)
		p.seriesBuf = append(p.seriesBuf, p.l.b[p.offsets[i]:p.offsets[i+1]]...)
		p.seriesBuf = append(p.seriesBuf, model.SeparatorByte)
		p.seriesBuf = append(p.seriesBuf, p.l.b[p.offsets[i+2]:p.offsets[i+3]]...)
	}
	if injectKey != "" {
		p.seriesBuf = append(p.seriesBuf, model.SeparatorByte)
		p.seriesBuf = append(p.seriesBuf, injectKey...)
		p.seriesBuf = append(p.seriesBuf, model.SeparatorByte)
		p.seriesBuf = labels.AppendOpenMetricsFloat(p.seriesBuf, injectVal)
	}
	return pendingEntry{
		seriesStart: start,
		seriesEnd:   len(p.seriesBuf),
		nameLen:     nameLen,
		injectKey:   injectKey,
		val:         val,
	}
}

// bucketEntry holds one parsed classic histogram bucket.
type bucketEntry struct {
	lef   float64
	count float64
}

// parseBuckets yields each entry from "[1.0:3,2.0:7,+Inf:12]" without
// materialising an intermediate slice.
func parseBuckets(s string) iter.Seq2[bucketEntry, error] {
	return func(yield func(bucketEntry, error) bool) {
		s = strings.TrimSpace(s)
		if s == "" || s == "[]" {
			return
		}
		if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
			yield(bucketEntry{}, fmt.Errorf("bucket must be wrapped in []: %q", s))
			return
		}
		inner := s[1 : len(s)-1]
		prevLe := math.Inf(-1)
		firstLe := true
		for part := range strings.SplitSeq(inner, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			idx := strings.LastIndexByte(part, ':')
			if idx < 0 {
				yield(bucketEntry{}, fmt.Errorf("bucket missing ':': %q", part))
				return
			}
			le := strings.TrimSpace(part[:idx])
			count, err := strconv.ParseFloat(strings.TrimSpace(part[idx+1:]), 64)
			if err != nil {
				yield(bucketEntry{}, fmt.Errorf("invalid bucket count %q: %w", part[idx+1:], err))
				return
			}
			lef, err := strconv.ParseFloat(le, 64)
			if err != nil {
				yield(bucketEntry{}, fmt.Errorf("invalid bucket threshold %q: %w", le, err))
				return
			}
			if math.IsNaN(lef) {
				yield(bucketEntry{}, fmt.Errorf("bucket threshold must not be NaN: %q", part))
				return
			}
			// OM2 permits -Inf as the first bucket threshold, so the ordering
			// check starts at the second one.
			if !firstLe && lef <= prevLe {
				yield(bucketEntry{}, fmt.Errorf("classic histogram buckets must be sorted in increasing order: %q", part))
				return
			}
			firstLe = false
			prevLe = lef
			if !yield(bucketEntry{lef: lef, count: count}, nil) {
				return
			}
		}
	}
}

// quantileEntry holds one parsed summary quantile.
type quantileEntry struct {
	qf  float64
	val float64
}

// parseQuantiles yields each entry from "[0.5:1.0,0.9:2.0]" without
// materialising an intermediate slice.
func parseQuantiles(s string) iter.Seq2[quantileEntry, error] {
	return func(yield func(quantileEntry, error) bool) {
		s = strings.TrimSpace(s)
		if s == "" || s == "[]" {
			return
		}
		if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
			yield(quantileEntry{}, fmt.Errorf("quantile must be wrapped in []: %q", s))
			return
		}
		inner := s[1 : len(s)-1]
		prevQ := math.Inf(-1)
		for part := range strings.SplitSeq(inner, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			before, after, ok := strings.Cut(part, ":")
			if !ok {
				yield(quantileEntry{}, fmt.Errorf("quantile missing ':': %q", part))
				return
			}
			q := strings.TrimSpace(before)
			val, err := strconv.ParseFloat(strings.TrimSpace(after), 64)
			if err != nil {
				yield(quantileEntry{}, fmt.Errorf("invalid quantile value %q: %w", after, err))
				return
			}
			qf, err := strconv.ParseFloat(q, 64)
			if err != nil {
				yield(quantileEntry{}, fmt.Errorf("invalid quantile %q: %w", q, err))
				return
			}
			if math.IsNaN(qf) {
				yield(quantileEntry{}, fmt.Errorf("quantile must not be NaN: %q", part))
				return
			}
			if qf < 0 || qf > 1 {
				yield(quantileEntry{}, fmt.Errorf("quantile must be between 0.0 and 1.0, got %q", q))
				return
			}
			if qf <= prevQ {
				yield(quantileEntry{}, fmt.Errorf("quantiles must be sorted in increasing order: %q", part))
				return
			}
			prevQ = qf
			if !yield(quantileEntry{qf: qf, val: val}, nil) {
				return
			}
		}
	}
}
