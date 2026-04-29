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
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/schema"
)

// Error satisfies the golex interface for openMetrics2Lexer.
func (l *openMetrics2Lexer) Error(es string) {
	l.err = errors.New(es)
}

// pendingEntry holds a single exploded flat series to be drained from the
// pending queue that is built when a composite value (summary or classic
// histogram) is parsed.
type pendingEntry struct {
	series []byte
	lset   labels.Labels
	val    float64
	ts     *int64
}

// om2Exemplar holds a fully parsed exemplar.
type om2Exemplar struct {
	e exemplar.Exemplar
}

// OpenMetrics2Parser parses samples from a byte slice in the OpenMetrics 2.0
// text exposition format.
// Specification: https://github.com/prometheus/OpenMetrics/blob/main/specification/OpenMetric.md
type OpenMetrics2Parser struct {
	l       *openMetrics2Lexer
	builder labels.ScratchBuilder

	// Metadata for the current metric family.  These fields are set once per
	// # TYPE / # UNIT line and intentionally persist across every series line
	// in the same family — they are NOT reset between series entries.
	mtype model.MetricType
	unit  string
	text  []byte

	// Current sample position in the input.
	series    []byte
	mfNameLen int // byte length of metric family name within series
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
	exemplars   []om2Exemplar
	exemplarIdx int

	// Pending queue for composite values exploded into flat EntrySeries.
	// pending[0..pendingIdx-1] are waiting to be served; Next() increments
	// pendingIdx and Series()/Labels() read pending[pendingIdx-1].
	pending    []pendingEntry
	pendingIdx int

	enableTypeAndUnitLabels bool
}

type openMetrics2ParserOptions struct {
	enableTypeAndUnitLabels bool
}

// OpenMetrics2Option is a functional option for OpenMetrics2Parser.
type OpenMetrics2Option func(*openMetrics2ParserOptions)

// WithOM2TypeAndUnitLabels enables injection of __type__ and __unit__ labels
// into parsed series.
func WithOM2TypeAndUnitLabels() OpenMetrics2Option {
	return func(o *openMetrics2ParserOptions) {
		o.enableTypeAndUnitLabels = true
	}
}

// NewOpenMetrics2Parser returns a new parser for the OpenMetrics 2.0 text
// format.
func NewOpenMetrics2Parser(b []byte, st *labels.SymbolTable, opts ...OpenMetrics2Option) Parser {
	options := &openMetrics2ParserOptions{}
	for _, opt := range opts {
		opt(options)
	}
	return &OpenMetrics2Parser{
		l:                       &openMetrics2Lexer{b: b},
		builder:                 labels.NewScratchBuilderWithSymbolTable(st, 16),
		enableTypeAndUnitLabels: options.enableTypeAndUnitLabels,
	}
}

// Series returns the bytes of the current series, the timestamp if set, and
// the sample value.
func (p *OpenMetrics2Parser) Series() ([]byte, *int64, float64) {
	if p.pendingIdx > 0 {
		pe := p.pending[p.pendingIdx-1]
		return pe.series, pe.ts, pe.val
	}
	if p.hasTS {
		ts := p.ts
		return p.series, &ts, p.val
	}
	return p.series, nil, p.val
}

// Histogram returns the bytes of the current series, the timestamp if set,
// and the native histogram value.
func (p *OpenMetrics2Parser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	if p.hasTS {
		ts := p.ts
		return p.series, &ts, p.h, p.fh
	}
	return p.series, nil, p.h, p.fh
}

// Help returns the metric name and help text of the current entry.
// Must only be called after Next returned EntryHelp.
func (p *OpenMetrics2Parser) Help() ([]byte, []byte) {
	m := p.l.b[p.offsets[0]:p.offsets[1]]
	if bytes.IndexByte(p.text, byte('\\')) >= 0 {
		return m, []byte(lvalReplacer.Replace(string(p.text)))
	}
	return m, p.text
}

// Type returns the metric name and type of the current entry.
// Must only be called after Next returned EntryType.
func (p *OpenMetrics2Parser) Type() ([]byte, model.MetricType) {
	return p.l.b[p.offsets[0]:p.offsets[1]], p.mtype
}

// Unit returns the metric name and unit of the current entry.
// Must only be called after Next returned EntryUnit.
func (p *OpenMetrics2Parser) Unit() ([]byte, []byte) {
	return p.l.b[p.offsets[0]:p.offsets[1]], []byte(p.unit)
}

// Comment returns the text of the current comment.
// Must only be called after Next returned EntryComment.
func (p *OpenMetrics2Parser) Comment() []byte {
	return p.text
}

// Labels writes the labels of the current sample into l.
func (p *OpenMetrics2Parser) Labels(l *labels.Labels) {
	if p.pendingIdx > 0 {
		*l = p.pending[p.pendingIdx-1].lset
		return
	}
	s := string(p.series)
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
func (p *OpenMetrics2Parser) Exemplar(e *exemplar.Exemplar) bool {
	if p.exemplarIdx >= len(p.exemplars) {
		return false
	}
	*e = p.exemplars[p.exemplarIdx].e
	p.exemplarIdx++
	return true
}

// StartTimestamp returns the inline start timestamp for the current sample
// (from the "st@<ts>" token), or 0 if none was present.  This is O(1); there
// is no forward scan.
func (p *OpenMetrics2Parser) StartTimestamp() int64 {
	if p.hasST {
		return p.st
	}
	return 0
}

func (p *OpenMetrics2Parser) nextToken() token {
	return p.l.Lex()
}

func (p *OpenMetrics2Parser) parseError(exp string, got token) error {
	e := min(len(p.l.b), p.l.i+1)
	return fmt.Errorf("%s, got %q (%q) while parsing: %q", exp, p.l.b[p.l.start:e], got, p.l.b[p.start:e])
}

// Next advances the parser to the next entry.
// It returns (EntryInvalid, io.EOF) when there are no more entries.
func (p *OpenMetrics2Parser) Next() (Entry, error) {
	// Drain pending composite-value entries.
	if p.pendingIdx < len(p.pending) {
		p.pendingIdx++
		return EntrySeries, nil
	}
	p.pending = p.pending[:0]
	p.pendingIdx = 0

	var err error
	p.start = p.l.i
	p.offsets = p.offsets[:0]
	p.exemplars = p.exemplars[:0]
	p.exemplarIdx = 0
	p.hasTS = false
	p.hasST = false
	p.h = nil
	p.fh = nil

	switch t := p.nextToken(); t {
	case tEOFWord:
		if t := p.nextToken(); t != tEOF {
			return EntryInvalid, errors.New("unexpected data after # EOF")
		}
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
			}
			p.mfNameLen = mEnd - mStart
			p.offsets = append(p.offsets, mStart, mEnd)
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
			return EntryType, nil
		case tUnit:
			p.unit = string(p.text)
			m := yoloString(p.l.b[p.offsets[0]:p.offsets[1]])
			if p.unit != "" {
				if !strings.HasSuffix(m, p.unit) || len(m) < len(p.unit)+1 || p.l.b[p.offsets[1]-len(p.unit)-1] != '_' {
					return EntryInvalid, fmt.Errorf("unit %q not a suffix of metric %q", p.unit, m)
				}
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
		p.series = p.l.b[p.start:p.l.i]
		return p.parseSeriesEndOfLine(p.nextToken())

	case tMName:
		p.offsets = append(p.offsets, p.start, p.l.i)
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
func (p *OpenMetrics2Parser) parseSeriesEndOfLine(t token) (Entry, error) {
	if p.offsets[0] == -1 {
		return EntryInvalid, fmt.Errorf("metric name not set while parsing: %q", p.l.b[p.start:p.l.i])
	}
	if t != tValue {
		return EntryInvalid, p.parseError("expected value after metric", t)
	}

	// raw is " {..." or " <float>"; strip the leading space.
	raw := p.l.buf()
	if len(raw) > 1 && raw[0] == ' ' {
		raw = raw[1:]
	}

	if len(raw) > 0 && raw[0] == '{' {
		return p.parseCompositeValue(raw)
	}

	// Plain float value.
	var err error
	p.val, err = parseFloat(yoloString(raw))
	if err != nil {
		return EntryInvalid, fmt.Errorf("%w while parsing: %q", err, p.l.b[p.start:p.l.i])
	}
	if math.IsNaN(p.val) {
		p.val = math.Float64frombits(value.NormalNaN)
	}

	if err := p.parseAfterValue(); err != nil {
		return EntryInvalid, err
	}
	return EntrySeries, nil
}

// parseAfterValue consumes the optional sequence after the metric value:
//
//	[tTimestamp] [tStartTimestamp] [*tComment exemplar] tLinebreak
//
// It returns after consuming tLinebreak (either directly or via exemplar
// parsing).
func (p *OpenMetrics2Parser) parseAfterValue() error {
	for {
		switch t := p.nextToken(); t {
		case tEOF:
			return errors.New("data does not end with # EOF")
		case tLinebreak:
			return nil
		case tTimestamp:
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
	}
}

// parseExemplars parses one or more exemplars up to and including the
// tLinebreak.  It is called after tComment has been consumed.
func (p *OpenMetrics2Parser) parseExemplars() error {
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
func (p *OpenMetrics2Parser) parseSingleExemplar() (done bool, err error) {
	var ex om2Exemplar

	// Parse exemplar label set (the "{..." was opened by tComment).
	eStart, eOffsets, err := p.parseExemplarLVals()
	if err != nil {
		return false, err
	}

	// Parse exemplar value.
	if t := p.nextToken(); t != tValue {
		return false, p.parseError("expected exemplar value", t)
	}
	ex.e.Value, err = parseFloat(yoloString(p.l.buf()[1:]))
	if err != nil {
		return false, fmt.Errorf("%w while parsing exemplar value: %q", err, p.l.b[p.start:p.l.i])
	}
	if math.IsNaN(ex.e.Value) {
		ex.e.Value = math.Float64frombits(value.NormalNaN)
	}

	// Build exemplar labels from the byte offsets collected above.
	p.builder.Reset()
	for i := 0; i < len(eOffsets); i += 4 {
		a := eOffsets[i]
		b := eOffsets[i+1]
		c := eOffsets[i+2]
		d := eOffsets[i+3]
		_ = eStart
		p.builder.Add(string(p.l.b[a:b]), unreplace(string(p.l.b[c:d])))
	}
	p.builder.Sort()
	ex.e.Labels = p.builder.Labels()

	// Read the token following the exemplar value (sETimestamp state).
	switch t2 := p.nextToken(); t2 {
	case tEOF:
		return false, errors.New("data does not end with # EOF")
	case tLinebreak:
		p.exemplars = append(p.exemplars, ex)
		return true, nil
	case tTimestamp:
		ex.e.HasTs = true
		var ts float64
		if ts, err = parseFloat(yoloString(p.l.buf()[1:])); err != nil {
			return false, fmt.Errorf("%w while parsing exemplar timestamp: %q", err, p.l.b[p.start:p.l.i])
		}
		if math.IsNaN(ts) || math.IsInf(ts, 0) {
			return false, fmt.Errorf("invalid exemplar timestamp %f", ts)
		}
		ex.e.Ts = int64(ts * 1000)
		p.exemplars = append(p.exemplars, ex)
		// After exemplar timestamp, sETimestamp state may yield tLinebreak
		// or tComment (for a next exemplar per OM2 yyrule32).
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
	case tComment:
		// No timestamp for this exemplar; another exemplar follows.
		p.exemplars = append(p.exemplars, ex)
		return false, nil
	default:
		return false, p.parseError("expected exemplar timestamp or newline", t2)
	}
}

// parseExemplarLVals parses the label set of an exemplar (the "{...}" part
// after tComment).  Returns the start offset and a flat list of
// [lName_start, lName_end, lValue_start, lValue_end, ...] offsets into p.l.b.
func (p *OpenMetrics2Parser) parseExemplarLVals() (int, []int, error) {
	eStart := p.l.i
	var offsets []int
	t := p.nextToken()
	for {
		switch t {
		case tBraceClose:
			return eStart, offsets, nil
		case tLName:
		default:
			return 0, nil, p.parseError("expected exemplar label name", t)
		}
		lStart := p.l.start
		lEnd := p.l.i
		if t = p.nextToken(); t != tEqual {
			return 0, nil, p.parseError("expected '=' in exemplar label", t)
		}
		if t = p.nextToken(); t != tLValue {
			return 0, nil, p.parseError("expected exemplar label value", t)
		}
		if !utf8.Valid(p.l.buf()) {
			return 0, nil, fmt.Errorf("invalid UTF-8 exemplar label value: %q", p.l.buf())
		}
		// Strip surrounding quotes from the value.
		offsets = append(offsets, lStart, lEnd, p.l.start+1, p.l.i-1)
		t = p.nextToken()
		if t == tComma {
			t = p.nextToken()
		} else if t != tBraceClose {
			return 0, nil, p.parseError("expected ',' or '}' in exemplar labels", t)
		}
	}
}

// parseLVals parses the label set "{k="v",...}" and appends byte offsets.
func (p *OpenMetrics2Parser) parseLVals(offsets []int, isExemplar bool) ([]int, error) {
	t := p.nextToken()
	for {
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
		if isQString && (t == tComma || t == tBraceClose) {
			if isExemplar {
				return nil, p.parseError("expected label name", t)
			}
			if offsets[0] != -1 || offsets[1] != -1 {
				return nil, fmt.Errorf("metric name already set while parsing: %q", p.l.b[p.start:p.l.i])
			}
			offsets[0] = curTStart + 1
			offsets[1] = curTI - 1
			if t == tBraceClose {
				return offsets, nil
			}
			t = p.nextToken()
			continue
		}
		if p.l.b[curTStart] == '"' {
			curTStart++
			curTI--
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
// (EntrySeries) from the composite value token.
//
// raw is the raw composite value bytes including the surrounding {}.
func (p *OpenMetrics2Parser) parseCompositeValue(raw []byte) (Entry, error) {
	// Consume the rest of the line (timestamp, st@, exemplars) before building
	// the pending entries, so the exemplar and ST fields are set correctly.
	if err := p.parseAfterValue(); err != nil {
		return EntryInvalid, err
	}

	switch p.mtype {
	case model.MetricTypeHistogram, model.MetricTypeGaugeHistogram:
		return p.parseHistogramComposite(raw)
	case model.MetricTypeSummary:
		return p.parseSummaryComposite(raw)
	default:
		return EntryInvalid, fmt.Errorf(
			"composite value not supported for metric type %q while parsing: %q",
			p.mtype, raw,
		)
	}
}

// kvMap parses "{k:v,...}" into a string map.  Values are returned verbatim
// (not unquoted).  Nested brackets in values are supported.
func kvMap(raw []byte) (map[string]string, error) {
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return nil, fmt.Errorf("composite value must be wrapped in {}: %q", raw)
	}
	inner := raw[1 : len(raw)-1]
	m := map[string]string{}
	for _, part := range splitCompositeFields(inner) {
		part = bytes.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		before, after, ok := bytes.Cut(part, []byte{':'})
		if !ok {
			return nil, fmt.Errorf("invalid composite field (missing ':'): %q", part)
		}
		k := string(bytes.TrimSpace(before))
		v := string(bytes.TrimSpace(after))
		m[k] = v
	}
	return m, nil
}

// splitCompositeFields splits b at top-level commas (not inside brackets).
func splitCompositeFields(b []byte) [][]byte {
	var result [][]byte
	depth := 0
	start := 0
	for i, ch := range b {
		switch ch {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ',':
			if depth == 0 {
				result = append(result, b[start:i])
				start = i + 1
			}
		}
	}
	return append(result, b[start:])
}

// parseHistogramComposite parses a composite histogram value such as:
//
//	{count:12,sum:5.5,schema:0,zero_threshold:0.001,zero_count:2,
//	 positive_spans:[0:3,2:1],positive_deltas:[1,1,-1,2],
//	 negative_spans:[],negative_deltas:[]}
//
// or a classic histogram:
//
//	{count:12,sum:5.5,bucket:[+Inf:12,1.0:3,2.0:7]}
//
// When native buckets are present it returns EntryHistogram.  Otherwise it
// populates the pending queue and returns EntrySeries for the first entry.
func (p *OpenMetrics2Parser) parseHistogramComposite(raw []byte) (Entry, error) {
	kv, err := kvMap(raw)
	if err != nil {
		return EntryInvalid, err
	}

	isNative := false
	for _, k := range []string{
		"schema", "positive_spans", "positive_deltas",
		"negative_spans", "negative_deltas",
	} {
		if _, ok := kv[k]; ok {
			isNative = true
			break
		}
	}

	if isNative {
		h, fh, err := buildNativeHistogram(kv, p.mtype == model.MetricTypeGaugeHistogram)
		if err != nil {
			return EntryInvalid, fmt.Errorf("error parsing native histogram composite: %w", err)
		}
		p.h = h
		p.fh = fh
		return EntryHistogram, nil
	}

	// Classic histogram: explode into flat pending entries.
	pending, err := buildClassicHistogramPending(kv, p.series, p.mfNameLen, p.mtype, p.hasTS, p.ts)
	if err != nil {
		return EntryInvalid, fmt.Errorf("error parsing classic histogram composite: %w", err)
	}
	return p.servePending(pending)
}

// parseSummaryComposite parses a composite summary value such as:
//
//	{count:12,sum:5.5,quantile:[0.5:1.0,0.9:2.0,0.99:3.0]}
func (p *OpenMetrics2Parser) parseSummaryComposite(raw []byte) (Entry, error) {
	kv, err := kvMap(raw)
	if err != nil {
		return EntryInvalid, err
	}

	pending, err := buildSummaryPending(kv, p.series, p.mfNameLen, p.hasTS, p.ts)
	if err != nil {
		return EntryInvalid, fmt.Errorf("error parsing summary composite: %w", err)
	}
	return p.servePending(pending)
}

// servePending stores pending and returns the first entry.
func (p *OpenMetrics2Parser) servePending(pending []pendingEntry) (Entry, error) {
	if len(pending) == 0 {
		return EntryInvalid, errors.New("composite value produced no series")
	}
	p.pending = pending
	p.pendingIdx = 1 // we are about to serve pending[0]
	return EntrySeries, nil
}

func buildNativeHistogram(kv map[string]string, isGauge bool) (*histogram.Histogram, *histogram.FloatHistogram, error) {
	getFloat := func(key string) (float64, bool, error) {
		v, ok := kv[key]
		if !ok {
			return 0, false, nil
		}
		f, err := strconv.ParseFloat(v, 64)
		return f, true, err
	}
	getInt := func(key string) (int64, bool, error) {
		v, ok := kv[key]
		if !ok {
			return 0, false, nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		return n, true, err
	}

	schema64, _, err := getInt("schema")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid schema: %w", err)
	}

	count, _, err := getFloat("count")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid count: %w", err)
	}
	sum, _, err := getFloat("sum")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid sum: %w", err)
	}
	zeroThreshold, _, err := getFloat("zero_threshold")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid zero_threshold: %w", err)
	}
	zeroCount, _, err := getFloat("zero_count")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid zero_count: %w", err)
	}

	// Treat as FloatHistogram only if the count or zero_count are non-integer.
	// sum is always float64 in both histogram types and does not determine the kind.
	isFloat := count != math.Trunc(count) || zeroCount != math.Trunc(zeroCount)

	posSpans, err := parseSpans(kv["positive_spans"])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid positive_spans: %w", err)
	}
	negSpans, err := parseSpans(kv["negative_spans"])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid negative_spans: %w", err)
	}

	if isFloat {
		posDeltas, err := parseFloatDeltas(kv["positive_deltas"])
		if err != nil {
			return nil, nil, fmt.Errorf("invalid positive_deltas: %w", err)
		}
		negDeltas, err := parseFloatDeltas(kv["negative_deltas"])
		if err != nil {
			return nil, nil, fmt.Errorf("invalid negative_deltas: %w", err)
		}
		fh := &histogram.FloatHistogram{
			Schema:          int32(schema64),
			ZeroThreshold:   zeroThreshold,
			ZeroCount:       zeroCount,
			Count:           count,
			Sum:             sum,
			PositiveSpans:   posSpans,
			NegativeSpans:   negSpans,
			PositiveBuckets: posDeltas,
			NegativeBuckets: negDeltas,
		}
		if isGauge {
			fh.CounterResetHint = histogram.GaugeType
		}
		return nil, fh, nil
	}

	posDeltas, err := parseIntDeltas(kv["positive_deltas"])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid positive_deltas: %w", err)
	}
	negDeltas, err := parseIntDeltas(kv["negative_deltas"])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid negative_deltas: %w", err)
	}
	h := &histogram.Histogram{
		Schema:          int32(schema64),
		ZeroThreshold:   zeroThreshold,
		ZeroCount:       uint64(zeroCount),
		Count:           uint64(count),
		Sum:             sum,
		PositiveSpans:   posSpans,
		NegativeSpans:   negSpans,
		PositiveBuckets: posDeltas,
		NegativeBuckets: negDeltas,
	}
	if isGauge {
		h.CounterResetHint = histogram.GaugeType
	}
	return h, nil, nil
}

// parseSpans parses "[offset:length,...]" into a []histogram.Span.
func parseSpans(s string) ([]histogram.Span, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil, nil
	}
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("spans must be wrapped in []: %q", s)
	}
	inner := s[1 : len(s)-1]
	if strings.TrimSpace(inner) == "" {
		return nil, nil
	}
	var spans []histogram.Span
	for part := range strings.SplitSeq(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		before, after, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("span missing ':': %q", part)
		}
		offset, err := strconv.ParseInt(strings.TrimSpace(before), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid span offset %q: %w", before, err)
		}
		length, err := strconv.ParseUint(strings.TrimSpace(after), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid span length %q: %w", after, err)
		}
		spans = append(spans, histogram.Span{Offset: int32(offset), Length: uint32(length)})
	}
	return spans, nil
}

// parseIntDeltas parses "[d1,d2,...]" into delta-encoded []int64 buckets.
func parseIntDeltas(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil, nil
	}
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("deltas must be wrapped in []: %q", s)
	}
	inner := s[1 : len(s)-1]
	if strings.TrimSpace(inner) == "" {
		return nil, nil
	}
	var deltas []int64
	for part := range strings.SplitSeq(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		d, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid delta %q: %w", part, err)
		}
		deltas = append(deltas, d)
	}
	return deltas, nil
}

// parseFloatDeltas parses "[d1,d2,...]" into []float64 bucket values.
func parseFloatDeltas(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil, nil
	}
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("float deltas must be wrapped in []: %q", s)
	}
	inner := s[1 : len(s)-1]
	if strings.TrimSpace(inner) == "" {
		return nil, nil
	}
	var deltas []float64
	for part := range strings.SplitSeq(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		d, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float delta %q: %w", part, err)
		}
		deltas = append(deltas, d)
	}
	return deltas, nil
}

func buildClassicHistogramPending(
	kv map[string]string,
	series []byte,
	mfNameLen int,
	_ model.MetricType,
	hasTS bool,
	ts int64,
) ([]pendingEntry, error) {
	mfName := extractMFName(series, mfNameLen)
	extraLabels := extractExtraLabels(series, mfNameLen)

	var tsPtr *int64
	if hasTS {
		tsPtr = &ts
	}

	var pending []pendingEntry

	// _count
	if cv, ok := kv["count"]; ok {
		v, err := strconv.ParseFloat(cv, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid count: %w", err)
		}
		name := mfName + "_count"
		pending = append(pending, pendingEntry{
			series: []byte(name),
			lset:   buildPendingLabels(name, extraLabels, "", ""),
			val:    v,
			ts:     tsPtr,
		})
	}

	// _sum
	if sv, ok := kv["sum"]; ok {
		v, err := strconv.ParseFloat(sv, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid sum: %w", err)
		}
		name := mfName + "_sum"
		pending = append(pending, pendingEntry{
			series: []byte(name),
			lset:   buildPendingLabels(name, extraLabels, "", ""),
			val:    v,
			ts:     tsPtr,
		})
	}

	// _bucket entries
	if bv, ok := kv["bucket"]; ok {
		buckets, err := parseBuckets(bv)
		if err != nil {
			return nil, fmt.Errorf("invalid bucket: %w", err)
		}
		name := mfName + "_bucket"
		for _, b := range buckets {
			pending = append(pending, pendingEntry{
				series: []byte(name),
				lset:   buildPendingLabels(name, extraLabels, "le", b.le),
				val:    b.count,
				ts:     tsPtr,
			})
		}
	}

	return pending, nil
}

func buildSummaryPending(
	kv map[string]string,
	series []byte,
	mfNameLen int,
	hasTS bool,
	ts int64,
) ([]pendingEntry, error) {
	mfName := extractMFName(series, mfNameLen)
	extraLabels := extractExtraLabels(series, mfNameLen)

	var tsPtr *int64
	if hasTS {
		tsPtr = &ts
	}

	var pending []pendingEntry

	if cv, ok := kv["count"]; ok {
		v, err := strconv.ParseFloat(cv, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid count: %w", err)
		}
		name := mfName + "_count"
		pending = append(pending, pendingEntry{
			series: []byte(name),
			lset:   buildPendingLabels(name, extraLabels, "", ""),
			val:    v,
			ts:     tsPtr,
		})
	}

	if sv, ok := kv["sum"]; ok {
		v, err := strconv.ParseFloat(sv, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid sum: %w", err)
		}
		name := mfName + "_sum"
		pending = append(pending, pendingEntry{
			series: []byte(name),
			lset:   buildPendingLabels(name, extraLabels, "", ""),
			val:    v,
			ts:     tsPtr,
		})
	}

	if qv, ok := kv["quantile"]; ok {
		quantiles, err := parseQuantiles(qv)
		if err != nil {
			return nil, fmt.Errorf("invalid quantile: %w", err)
		}
		for _, q := range quantiles {
			pending = append(pending, pendingEntry{
				series: []byte(mfName),
				lset:   buildPendingLabels(mfName, extraLabels, "quantile", q.q),
				val:    q.val,
				ts:     tsPtr,
			})
		}
	}

	return pending, nil
}

// extractMFName extracts the metric family name from the series bytes.
func extractMFName(series []byte, mfNameLen int) string {
	if len(series) > 1 && series[0] == '{' && series[1] == '"' {
		return string(series[2 : mfNameLen+2])
	}
	return string(series[:mfNameLen])
}

// extractExtraLabels extracts label name-value pairs from the "{...}" part of
// the series bytes.
func extractExtraLabels(series []byte, mfNameLen int) []labels.Label {
	_, inner, ok := bytes.Cut(series, []byte{'{'})
	if !ok {
		return nil
	}
	if len(inner) == 0 || inner[len(inner)-1] != '}' {
		return nil
	}
	inner = inner[:len(inner)-1]
	// For UTF-8 metric names the series starts with {"name",labels...}.
	// Skip past the quoted name so the label scanner only sees key=value pairs.
	if len(inner) > 0 && inner[0] == '"' {
		inner = inner[mfNameLen+2:] // skip opening quote + name bytes + closing quote
		if len(inner) > 0 && inner[0] == ',' {
			inner = inner[1:]
		}
	}
	if len(bytes.TrimSpace(inner)) == 0 {
		return nil
	}
	var result []labels.Label
	for len(inner) > 0 {
		eq := bytes.IndexByte(inner, '=')
		if eq < 0 {
			break
		}
		k := string(bytes.TrimSpace(inner[:eq]))
		inner = inner[eq+1:]
		if len(inner) == 0 || inner[0] != '"' {
			break
		}
		end := 1
		for end < len(inner) {
			if inner[end] == '\\' {
				end += 2
				continue
			}
			if inner[end] == '"' {
				end++
				break
			}
			end++
		}
		v := unreplace(string(inner[1 : end-1]))
		result = append(result, labels.Label{Name: k, Value: v})
		inner = inner[end:]
		if len(inner) > 0 && inner[0] == ',' {
			inner = inner[1:]
		}
	}
	return result
}

// buildPendingLabels builds labels for a pending entry.  injectKey and
// injectVal are the extra label to inject (e.g. "le"/"quantile") — pass empty
// strings for _count and _sum series.
func buildPendingLabels(name string, extra []labels.Label, injectKey, injectVal string) labels.Labels {
	b := labels.NewBuilder(labels.EmptyLabels())
	b.Set(labels.MetricName, name)
	for _, l := range extra {
		b.Set(l.Name, l.Value)
	}
	if injectKey != "" {
		b.Set(injectKey, injectVal)
	}
	return b.Labels()
}

// bucketEntry holds one parsed classic histogram bucket.
type bucketEntry struct {
	le    string
	count float64
}

// parseBuckets parses "[+Inf:12,1.0:3,2.0:7]" into bucket entries.
func parseBuckets(s string) ([]bucketEntry, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil, nil
	}
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("bucket must be wrapped in []: %q", s)
	}
	inner := s[1 : len(s)-1]
	var buckets []bucketEntry
	for part := range strings.SplitSeq(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Use LastIndexByte to handle "+Inf" which contains no ':' ambiguity;
		// but "+Inf" has no ':', so use IndexByte.
		idx := strings.LastIndexByte(part, ':')
		if idx < 0 {
			return nil, fmt.Errorf("bucket missing ':': %q", part)
		}
		le := strings.TrimSpace(part[:idx])
		count, err := strconv.ParseFloat(strings.TrimSpace(part[idx+1:]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid bucket count %q: %w", part[idx+1:], err)
		}
		// Normalise le to OpenMetrics float format.
		if lef, err := strconv.ParseFloat(le, 64); err == nil {
			le = labels.FormatOpenMetricsFloat(lef)
		}
		buckets = append(buckets, bucketEntry{le: le, count: count})
	}
	return buckets, nil
}

// quantileEntry holds one parsed summary quantile.
type quantileEntry struct {
	q   string
	val float64
}

// parseQuantiles parses "[0.5:1.0,0.9:2.0]" into quantile entries.
func parseQuantiles(s string) ([]quantileEntry, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil, nil
	}
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("quantile must be wrapped in []: %q", s)
	}
	inner := s[1 : len(s)-1]
	var quantiles []quantileEntry
	for part := range strings.SplitSeq(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		before, after, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("quantile missing ':': %q", part)
		}
		q := strings.TrimSpace(before)
		val, err := strconv.ParseFloat(strings.TrimSpace(after), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid quantile value %q: %w", after, err)
		}
		// Normalise quantile label to OpenMetrics float format.
		if qf, err := strconv.ParseFloat(q, 64); err == nil {
			q = labels.FormatOpenMetricsFloat(qf)
		}
		quantiles = append(quantiles, quantileEntry{q: q, val: val})
	}
	return quantiles, nil
}
