// Copyright 2025 The Prometheus Authors
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
	"strings"
	"unicode/utf8"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/util/convertnhcb"
)

type openMetrics2Lexer struct {
	b     []byte
	i     int
	start int
	err   error
	state int
}

// buf returns the buffer of the current token.
func (l *openMetrics2Lexer) buf() []byte {
	return l.b[l.start:l.i]
}

// next advances the openMetricsLexer to the next character.
func (l *openMetrics2Lexer) next() byte {
	l.i++
	if l.i >= len(l.b) {
		l.err = io.EOF
		return byte(tEOF)
	}
	// Lex struggles with null bytes. If we are in a label value or help string, where
	// they are allowed, consume them here immediately.
	for l.b[l.i] == 0 && (l.state == sLValue || l.state == sMeta2 || l.state == sComment) {
		l.i++
		if l.i >= len(l.b) {
			l.err = io.EOF
			return byte(tEOF)
		}
	}
	return l.b[l.i]
}

func (l *openMetrics2Lexer) Error(es string) {
	l.err = errors.New(es)
}

// OpenMetrics2Parser text exposition format.
// Specification can be found at https://prometheus.io/docs/specs/om/open_metrics_spec_2_0/
type OpenMetrics2Parser struct {
	l         *openMetrics2Lexer
	builder   labels.ScratchBuilder
	series    []byte
	mfNameLen int // length of metric family name to get from series.
	text      []byte
	mtype     model.MetricType
	unit      string

	val      float64
	tempHist convertnhcb.TempHistogram
	h        *histogram.Histogram
	fh       *histogram.FloatHistogram
	// TODO: Implement summary compelx type.

	ts, sts int64
	hasTS   bool
	start   int
	// offsets is a list of offsets into series that describe the positions
	// of the metric name and label names and values for this series.
	// p.offsets[0] is the start character of the metric name.
	// p.offsets[1] is the end of the metric name.
	// Subsequently, p.offsets is a pair of pair of offsets for the positions
	// of the label name and value start and end characters.
	offsets []int

	eOffsets      []int
	exemplar      []byte
	exemplarVal   float64
	exemplarTs    int64
	hasExemplarTs bool

	// ignoreExemplar instructs the parser to not overwrite exemplars (to keep them while peeking ahead).
	ignoreExemplar          bool
	enableTypeAndUnitLabels bool
	unrollComplexTypes      bool
}

type openMetrics2ParserOptions struct {
	enableTypeAndUnitLabels bool
	// TODO: Probably this option should be per metric name (:
	unrollComplexTypes bool
}

type OpenMetrics2Option func(*openMetrics2ParserOptions)

// WithOM2ParserTypeAndUnitLabels enables type-and-unit-labels mode
// in which parser injects __type__ and __unit__ into labels.
func WithOM2ParserTypeAndUnitLabels() OpenMetrics2Option {
	return func(o *openMetrics2ParserOptions) {
		o.enableTypeAndUnitLabels = true
	}
}

// NewOpenMetrics2Parser returns a new parser for the byte slice with option to skip CT series parsing.
func NewOpenMetrics2Parser(b []byte, st *labels.SymbolTable, opts ...OpenMetrics2Option) Parser {
	options := &openMetrics2ParserOptions{}

	for _, opt := range opts {
		opt(options)
	}

	parser := &OpenMetrics2Parser{
		l:                       &openMetrics2Lexer{b: b},
		builder:                 labels.NewScratchBuilderWithSymbolTable(st, 16),
		enableTypeAndUnitLabels: options.enableTypeAndUnitLabels,
		unrollComplexTypes:      options.unrollComplexTypes,
		tempHist:                convertnhcb.NewTempHistogram(),
	}
	return parser
}

// Series returns the bytes of the series, the timestamp if set, and the value
// of the current sample.
func (p *OpenMetrics2Parser) Series() ([]byte, *int64, float64) {
	if p.hasTS {
		ts := p.ts
		return p.series, &ts, p.val
	}
	return p.series, nil, p.val
}

// Histogram returns the bytes of the series, the timestamp if set, and one of
// the value (float of integer histogram) of the current complex sample representing histogram.
func (p *OpenMetrics2Parser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	if p.hasTS {
		ts := p.ts
		return p.series, &ts, p.h, p.fh
	}
	return p.series, nil, p.h, p.fh
}

// Help returns the metric name and help text in the current entry.
// Must only be called after Next returned a help entry.
// The returned byte slices become invalid after the next call to Next.
func (p *OpenMetrics2Parser) Help() ([]byte, []byte) {
	m := p.l.b[p.offsets[0]:p.offsets[1]]

	// Replacer causes allocations. Replace only when necessary.
	if bytes.IndexByte(p.text, byte('\\')) >= 0 {
		// OpenMetrics always uses the Prometheus format label value escaping.
		return m, []byte(lvalReplacer.Replace(string(p.text)))
	}
	return m, p.text
}

// Type returns the metric name and type in the current entry.
// Must only be called after Next returned a type entry.
// The returned byte slices become invalid after the next call to Next.
func (p *OpenMetrics2Parser) Type() ([]byte, model.MetricType) {
	return p.l.b[p.offsets[0]:p.offsets[1]], p.mtype
}

// Unit returns the metric name and unit in the current entry.
// Must only be called after Next returned a unit entry.
// The returned byte slices become invalid after the next call to Next.
func (p *OpenMetrics2Parser) Unit() ([]byte, []byte) {
	return p.l.b[p.offsets[0]:p.offsets[1]], []byte(p.unit)
}

// Comment returns the text of the current comment.
// Must only be called after Next returned a comment entry.
// The returned byte slice becomes invalid after the next call to Next.
func (p *OpenMetrics2Parser) Comment() []byte {
	return p.text
}

// Labels writes the labels of the current sample into the passed labels.
func (p *OpenMetrics2Parser) Labels(l *labels.Labels) {
	// Defensive copy in case the following keeps a reference.
	// See https://github.com/prometheus/prometheus/issues/16490
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
		p.builder.Add(labels.MetricName, metricName)
	}
	for i := 2; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		label := unreplace(s[a:b])
		if p.enableTypeAndUnitLabels && !m.IsEmptyFor(label) {
			// Dropping user provided metadata labels, if found in the OM metadata.
			continue
		}
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start
		value := normalizeFloatsInLabelValues(p.mtype, label, unreplace(s[c:d]))
		p.builder.Add(label, value)
	}

	p.builder.Sort()
	*l = p.builder.Labels()
}

// Exemplar writes the exemplar of the current sample into the passed exemplar.
// It returns whether an exemplar exists. As OpenMetrics only ever has one
// exemplar per sample, every call after the first (for the same sample) will
// always return false.
func (p *OpenMetrics2Parser) Exemplar(e *exemplar.Exemplar) bool {
	if len(p.exemplar) == 0 {
		return false
	}

	// Allocate the full immutable string immediately, so we just
	// have to create references on it below.
	s := string(p.exemplar)

	e.Value = p.exemplarVal
	if p.hasExemplarTs {
		e.HasTs = true
		e.Ts = p.exemplarTs
	}

	p.builder.Reset()
	for i := 0; i < len(p.eOffsets); i += 4 {
		a := p.eOffsets[i] - p.start
		b := p.eOffsets[i+1] - p.start
		c := p.eOffsets[i+2] - p.start
		d := p.eOffsets[i+3] - p.start

		p.builder.Add(s[a:b], s[c:d])
	}

	p.builder.Sort()
	e.Labels = p.builder.Labels()

	// Wipe exemplar so that future calls return false.
	p.exemplar = p.exemplar[:0]
	return true
}

// CreatedTimestamp returns the created timestamp for a current Metric if exists or nil.
func (p *OpenMetrics2Parser) CreatedTimestamp() int64 {
	// TODO: Implement.
	return p.sts
}

// nextToken returns the next token from the openMetricsLexer.
func (p *OpenMetrics2Parser) nextToken() token {
	t := p.l.Lex()
	return t
}

func (p *OpenMetrics2Parser) parseError(exp string, got token) error {
	e := min(len(p.l.b), p.l.i+1)
	return fmt.Errorf("%s, got %q (%q) while parsing: %q", exp, p.l.b[p.l.start:e], got, p.l.b[p.start:e])
}

// Next advances the parser to the next sample.
// It returns (EntryInvalid, io.EOF) if no samples were read.
func (p *OpenMetrics2Parser) Next() (Entry, error) {
	var err error

	p.start = p.l.i
	p.offsets = p.offsets[:0]
	if !p.ignoreExemplar {
		p.eOffsets = p.eOffsets[:0]
		p.exemplar = p.exemplar[:0]
		p.exemplarVal = 0
		p.hasExemplarTs = false
	}

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
			if len(p.unit) > 0 {
				if !strings.HasSuffix(m, p.unit) || len(m) < len(p.unit)+1 || p.l.b[p.offsets[1]-len(p.unit)-1] != '_' {
					return EntryInvalid, fmt.Errorf("unit %q not a suffix of metric %q", p.unit, m)
				}
			}
			return EntryUnit, nil
		}

	case tBraceOpen:
		// We found a brace, so make room for the eventual metric name. If these
		// values aren't updated, then the metric name was not set inside the
		// braces and we can return an error.
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

func (p *OpenMetrics2Parser) parseComment() error {
	var err error

	if p.ignoreExemplar {
		for t := p.nextToken(); t != tLinebreak; t = p.nextToken() {
			if t == tEOF {
				return errors.New("data does not end with # EOF")
			}
		}
		return nil
	}

	// Parse the labels.
	p.eOffsets, err = p.parseLVals(p.eOffsets, true)
	if err != nil {
		return err
	}
	p.exemplar = p.l.b[p.start:p.l.i]

	// Get the value.
	p.exemplarVal, err = p.parseFloatValue(p.nextToken(), "exemplar labels")
	if err != nil {
		return err
	}

	// Read the optional timestamp.
	p.hasExemplarTs = false
	switch t2 := p.nextToken(); t2 {
	case tEOF:
		return errors.New("data does not end with # EOF")
	case tLinebreak:
		break
	case tTimestamp:
		p.hasExemplarTs = true
		var ts float64
		// A float is enough to hold what we need for millisecond resolution.
		if ts, err = parseFloat(yoloString(p.l.buf()[1:])); err != nil {
			return fmt.Errorf("%w while parsing: %q", err, p.l.b[p.start:p.l.i])
		}
		if math.IsNaN(ts) || math.IsInf(ts, 0) {
			return fmt.Errorf("invalid exemplar timestamp %f", ts)
		}
		p.exemplarTs = int64(ts * 1000)
		switch t3 := p.nextToken(); t3 {
		case tLinebreak:
		default:
			return p.parseError("expected next entry after exemplar timestamp", t3)
		}
	default:
		return p.parseError("expected timestamp or comment", t2)
	}
	return nil
}

func (p *OpenMetrics2Parser) parseLVals(offsets []int, isExemplar bool) ([]int, error) {
	t := p.nextToken()
	for {
		curTStart := p.l.start
		curTI := p.l.i
		switch t {
		case tBraceClose:
			return offsets, nil
		case tLName:
		case tQString:
		default:
			return nil, p.parseError("expected label name", t)
		}

		t = p.nextToken()
		// A quoted string followed by a comma or brace is a metric name. Set the
		// offsets and continue processing. If this is an exemplar, this format
		// is not allowed.
		if t == tComma || t == tBraceClose {
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
		// We have a label name, and it might be quoted.
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

		// The openMetricsLexer ensures the value string is quoted. Strip first
		// and last character.
		offsets = append(offsets, p.l.start+1, p.l.i-1)

		// Free trailing commas are allowed.
		t = p.nextToken()
		if t == tComma {
			t = p.nextToken()
		} else if t != tBraceClose {
			return nil, p.parseError("expected comma or brace close", t)
		}
	}
}

// parseSeriesEndOfLine parses the series end of the line (value, optional
// timestamp, commentary, etc.) after the metric name and labels.
// It starts parsing with the provided token.
func (p *OpenMetrics2Parser) parseSeriesEndOfLine(t token) (e Entry, err error) {
	if p.offsets[0] == -1 {
		return EntryInvalid, fmt.Errorf("metric name not set while parsing: %q", p.l.b[p.start:p.l.i])
	}
	switch p.l.state {
	case sTimestamp:
		p.val, err = p.parseFloatValue(t, "metric")
		if err != nil {
			return EntryInvalid, err
		}
		e = EntrySeries
	case sComplexValue:
		e, err = p.parseComplexValue(t, "metric")
		if err != nil {
			return EntryInvalid, err
		}
	default:
		return EntryInvalid, p.parseError(fmt.Sprintf("unexpected parser state %v, expect float or complex value", p.l.state), t)
	}

	p.hasTS = false
	p.sts = 0
	switch t2 := p.nextToken(); t2 {
	case tEOF:
		return EntryInvalid, errors.New("data does not end with # EOF")
	case tLinebreak:
		break
	case tComment:
		if err := p.parseComment(); err != nil {
			return EntryInvalid, err
		}
	case tStartTimestamp: // TODO: DRY with above, also support st + ts(!!!)
		var sts float64
		// A float is enough to hold what we need for millisecond resolution.
		// Add 4 chars for " st@" prefix.
		if sts, err = parseFloat(yoloString(p.l.buf()[4:])); err != nil {
			return EntryInvalid, fmt.Errorf("%w while parsing: %q", err, p.l.b[p.start:p.l.i])
		}
		if math.IsNaN(sts) || math.IsInf(sts, 0) {
			return EntryInvalid, fmt.Errorf("invalid start timestamp %f", sts)
		}
		p.sts = int64(sts * 1000)
		switch t3 := p.nextToken(); t3 {
		case tLinebreak:
		case tComment:
			if err := p.parseComment(); err != nil {
				return EntryInvalid, err
			}
		default:
			return EntryInvalid, p.parseError("expected next entry after start timestamp", t3)
		}
	case tTimestamp:
		p.hasTS = true
		var ts float64
		// A float is enough to hold what we need for millisecond resolution.
		if ts, err = parseFloat(yoloString(p.l.buf()[1:])); err != nil {
			return EntryInvalid, fmt.Errorf("%w while parsing: %q", err, p.l.b[p.start:p.l.i])
		}
		if math.IsNaN(ts) || math.IsInf(ts, 0) {
			return EntryInvalid, fmt.Errorf("invalid timestamp %f", ts)
		}
		p.ts = int64(ts * 1000)
		switch t3 := p.nextToken(); t3 {
		case tLinebreak:
		case tComment:
			if err := p.parseComment(); err != nil {
				return EntryInvalid, err
			}
		default:
			return EntryInvalid, p.parseError("expected next entry after timestamp", t3)
		}
	}
	return e, nil
}

func (p *OpenMetrics2Parser) parseFloatValue(t token, after string) (float64, error) {
	if t != tValue {
		return 0, p.parseError(fmt.Sprintf("expected value after %v", after), t)
	}
	val, err := parseFloat(yoloString(p.l.buf()[1:]))
	if err != nil {
		return 0, fmt.Errorf("%w while parsing: %q", err, p.l.b[p.start:p.l.i])
	}
	// Ensure canonical NaN value.
	if math.IsNaN(p.exemplarVal) {
		val = math.Float64frombits(value.NormalNaN)
	}
	return val, nil
}

func (p *OpenMetrics2Parser) parseComplexValue(t token, after string) (_ Entry, err error) {
	if t != tBraceOpen {
		return EntryInvalid, p.parseError(fmt.Sprintf("expected brace open after %v", after), t)
	}

	switch p.mtype {
	default:
		return EntryInvalid, p.parseError("unexpected parser type", t)
	case model.MetricTypeSummary:
		return EntryInvalid, p.parseError("summary complex value parsing not yet implemented", t)
	case model.MetricTypeHistogram:
		if p.unrollComplexTypes {
			// TODO: Implement this.
			panic("not implemented")
		}

		defer p.tempHist.Reset()

		if err := p.parseComplexValueHistogram(); err != nil {
			return EntryInvalid, err
		}
		p.h, p.fh, err = p.tempHist.Convert()
		if err != nil {
			return EntryInvalid, p.parseError(fmt.Sprintf("histogram complex value parsing failed: %v", err), t)
		}
		if p.h != nil {
			if err := p.h.Validate(); err != nil {
				return EntryInvalid, p.parseError(fmt.Sprintf("invalid histogram: %v", err), t)
			}
		} else if p.fh != nil {
			if err := p.fh.Validate(); err != nil {
				return EntryInvalid, p.parseError(fmt.Sprintf("invalid float histogram: %v", err), t)
			}
		}
		return EntryHistogram, nil
	}
}

func (p *OpenMetrics2Parser) parseComplexValueHistogram() (err error) {
	// The opening brace has already been consumed.
	t := p.nextToken()

	// Handle empty complex value, e.g., {}.
	if t == tBraceClose {
		return nil
	}

	for {
		// Expect a key (e.g., "count", "sum", "bucket").
		if t != tLName {
			return p.parseError("expected key in complex value", t)
		}
		key := yoloString(p.l.buf())

		if t2 := p.nextToken(); t2 != tColon {
			return p.parseError("expected colon after complex value key", t2)
		}

		// Handle the value based on the key.
		switch key {
		case "count":
			if t3 := p.nextToken(); t3 != tValue {
				return p.parseError("expected count value", t3)
			}
			val, err := parseFloat(yoloString(p.l.buf()))
			if err != nil {
				return fmt.Errorf("%w while parsing count: %q", err, p.l.b[p.start:p.l.i])
			}
			if err := p.tempHist.SetCount(val); err != nil {
				return fmt.Errorf("%w while parsing count for histogram: %v", err, p.l.b[p.start:p.l.i])
			}
		case "sum":
			if t3 := p.nextToken(); t3 != tValue {
				return p.parseError("expected sum value", t3)
			}
			val, err := parseFloat(yoloString(p.l.buf()))
			if err != nil {
				return fmt.Errorf("%w while parsing sum: %q", err, p.l.b[p.start:p.l.i])
			}
			if err := p.tempHist.SetSum(val); err != nil {
				return fmt.Errorf("%w while parsing sum for histogram: %v", err, p.l.b[p.start:p.l.i])
			}
		case "bucket":
			if t3 := p.nextToken(); t3 != tBracketOpen {
				return p.parseError("expected opening bracket for buckets", t3)
			}
			if err := p.parseBuckets(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown key in complex value: %q", key)
		}

		// After a key-value pair, expect a comma or the closing brace.
		t = p.nextToken()
		if t == tBraceClose {
			return nil
		}
		if t != tComma {
			return p.parseError("expected comma or closing brace after value", t)
		}

		// If we saw a comma, get the next token, which should be the next key.
		t = p.nextToken()
	}
}

// parseBuckets parses the content of a bucket list, e.g., [bound1:count1,bound2:count2].
func (p *OpenMetrics2Parser) parseBuckets() error {
	// Handle empty bucket list, e.g., [].
	t := p.nextToken()
	if t == tBracketClose {
		return nil
	}

	for {
		// Expect a bucket definition like "bound:count".
		if t != tValue {
			return p.parseError("expected bucket bound", t)
		}
		bound, err := parseFloat(yoloString(p.l.buf()))
		if err != nil {
			return fmt.Errorf("%w while parsing bucket bound: %q", err, p.l.b[p.start:p.l.i])
		}
		if t2 := p.nextToken(); t2 != tColon {
			return p.parseError("expected colon after bucket bound", t2)
		}
		if t3 := p.nextToken(); t3 != tValue {
			return p.parseError("expected bucket count", t3)
		}
		// The bucket count must be an integer.
		count, err := parseFloat(yoloString(p.l.buf()))
		if err != nil {
			return fmt.Errorf("%w while parsing bucket count: %q", err, p.l.b[p.start:p.l.i])
		}
		if err := p.tempHist.SetBucketCount(bound, count); err != nil {
			return fmt.Errorf("%w while parsing bucket bound and count: %q", err, p.l.b[p.start:p.l.i])
		}

		// Check for a comma or the closing bracket.
		t = p.nextToken()
		if t == tBracketClose {
			return nil
		}
		if t != tComma {
			return p.parseError("expected comma or closing bracket in bucket list", t)
		}
		// If we saw a comma, get the next token for the start of the next bucket.
		t = p.nextToken()
	}
}
