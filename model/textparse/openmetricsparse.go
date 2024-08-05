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

//go:generate go get -u modernc.org/golex
//go:generate golex -o=openmetricslex.l.go openmetricslex.l

package textparse

import (
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
)

type openMetricsLexer struct {
	b     []byte
	i     int
	start int
	err   error
	state int
}

// buf returns the buffer of the current token.
func (l *openMetricsLexer) buf() []byte {
	return l.b[l.start:l.i]
}

// next advances the openMetricsLexer to the next character.
func (l *openMetricsLexer) next() byte {
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

func (l *openMetricsLexer) Error(es string) {
	l.err = errors.New(es)
}

// OpenMetricsParser parses samples from a byte slice of samples in the official
// OpenMetrics text exposition format.
// This is based on the working draft https://docs.google.com/document/u/1/d/1KwV0mAXwwbvvifBvDKH_LU1YjyXE_wxCkHNoCGq1GX0/edit
type OpenMetricsParser struct {
	l       *openMetricsLexer
	builder labels.ScratchBuilder
	series  []byte
	text    []byte
	mtype   model.MetricType
	val     float64
	ts      int64
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

	// lines ending with _created are skipped by default as they are
	// parsed by the CreatedTimestamp method. The skipCT flag is set to
	// false when the CreatedTimestamp method is called.
	skipCT bool
}
type openMetricsParserOptions struct {
	// SkipCT skips the parsing of _created lines.
	SkipCT bool
}

type OpenMetricsOption func(*openMetricsParserOptions)

func WithOMParserCTSeriesSkipped(skipCT bool) OpenMetricsOption {
	return func(o *openMetricsParserOptions) {
		o.SkipCT = skipCT
	}
}

// NewOpenMetricsParser returns a new parser of the byte slice.
func NewOpenMetricsParser(b []byte, st *labels.SymbolTable) Parser {
	return &OpenMetricsParser{
		l:       &openMetricsLexer{b: b},
		builder: labels.NewScratchBuilderWithSymbolTable(st, 16),
		skipCT:  false,
	}
}

// NewOpenMetricsParserWithOpts returns a new parser of the byte slice with options.
func NewOpenMetricsParserWithOpts(b []byte, st *labels.SymbolTable, opts ...OpenMetricsOption) Parser {
	parser := &OpenMetricsParser{
		l:       &openMetricsLexer{b: b},
		builder: labels.NewScratchBuilderWithSymbolTable(st, 16),
	}
	DefaultOpenMetricsParserOptions := openMetricsParserOptions{
		SkipCT: true,
	}
	for _, opt := range opts {
		opt(&DefaultOpenMetricsParserOptions)
	}
	parser.skipCT = DefaultOpenMetricsParserOptions.SkipCT

	return parser
}

// Series returns the bytes of the series, the timestamp if set, and the value
// of the current sample.
func (p *OpenMetricsParser) Series() ([]byte, *int64, float64) {
	if p.hasTS {
		ts := p.ts
		return p.series, &ts, p.val
	}
	return p.series, nil, p.val
}

// Histogram returns (nil, nil, nil, nil) for now because OpenMetrics does not
// support sparse histograms yet.
func (p *OpenMetricsParser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	return nil, nil, nil, nil
}

// Help returns the metric name and help text in the current entry.
// Must only be called after Next returned a help entry.
// The returned byte slices become invalid after the next call to Next.
func (p *OpenMetricsParser) Help() ([]byte, []byte) {
	m := p.l.b[p.offsets[0]:p.offsets[1]]

	// Replacer causes allocations. Replace only when necessary.
	if strings.IndexByte(yoloString(p.text), byte('\\')) >= 0 {
		// OpenMetrics always uses the Prometheus format label value escaping.
		return m, []byte(lvalReplacer.Replace(string(p.text)))
	}
	return m, p.text
}

// Type returns the metric name and type in the current entry.
// Must only be called after Next returned a type entry.
// The returned byte slices become invalid after the next call to Next.
func (p *OpenMetricsParser) Type() ([]byte, model.MetricType) {
	return p.l.b[p.offsets[0]:p.offsets[1]], p.mtype
}

// Unit returns the metric name and unit in the current entry.
// Must only be called after Next returned a unit entry.
// The returned byte slices become invalid after the next call to Next.
func (p *OpenMetricsParser) Unit() ([]byte, []byte) {
	return p.l.b[p.offsets[0]:p.offsets[1]], p.text
}

// Comment returns the text of the current comment.
// Must only be called after Next returned a comment entry.
// The returned byte slice becomes invalid after the next call to Next.
func (p *OpenMetricsParser) Comment() []byte {
	return p.text
}

// Metric writes the labels of the current sample into the passed labels.
// It returns the string from which the metric was parsed.
func (p *OpenMetricsParser) Metric(l *labels.Labels) string {
	// Copy the buffer to a string: this is only necessary for the return value.
	s := string(p.series)

	p.builder.Reset()
	metricName := unreplace(s[p.offsets[0]-p.start : p.offsets[1]-p.start])
	p.builder.Add(labels.MetricName, metricName)

	for i := 2; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		label := unreplace(s[a:b])
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start
		value := unreplace(s[c:d])

		p.builder.Add(label, value)
	}

	p.builder.Sort()
	*l = p.builder.Labels()

	return s
}

// Exemplar writes the exemplar of the current sample into the passed exemplar.
// It returns whether an exemplar exists. As OpenMetrics only ever has one
// exemplar per sample, every call after the first (for the same sample) will
// always return false.
func (p *OpenMetricsParser) Exemplar(e *exemplar.Exemplar) bool {
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
// NOTE(Maniktherana): Might use additional CPU/mem resources due to deep copy of parser required for peeking given 1.0 OM specification on _created series.
func (p *OpenMetricsParser) CreatedTimestamp() *int64 {
	if !typeRequiresCT(p.mtype) {
		// Not a CT supported metric type, fast path.
		return nil
	}

	var (
		currLset                labels.Labels
		buf                     []byte
		peekWithoutNameLsetHash uint64
	)
	p.Metric(&currLset)
	currWithoutNameLsetHash, buf := currLset.HashWithoutLabels(buf, labels.MetricName, "le", "quantile")
	// Search for the _created line for the currName using ephemeral parser until
	// we see EOF or new metric family. We have to do it as we don't know where (and if)
	// that CT line is.
	// TODO(bwplotka): Make sure OM 1.1/2.0 pass CT via metadata or exemplar-like to avoid this.
	peek := deepCopy(p)
	for {
		eType, err := peek.Next()
		if err != nil {
			// This means p will give error too later on, so def no CT line found.
			// This might result in partial scrape with wrong/missing CT, but only
			// spec improvement would help.
			// TODO(bwplotka): Make sure OM 1.1/2.0 pass CT via metadata or exemplar-like to avoid this.
			return nil
		}
		if eType != EntrySeries {
			// Assume we hit different family, no CT line found.
			return nil
		}
		// We are sure this series is the same metric family as in currLset
		// because otherwise we would have EntryType first which we ruled out before.
		var peekedLset labels.Labels
		peek.Metric(&peekedLset)
		peekedName := peekedLset.Get(model.MetricNameLabel)
		if !strings.HasSuffix(peekedName, "_created") {
			// Not a CT line, search more.
			continue
		}

		// We got a CT line here, but let's search if CT line is actually for our series, edge case.
		peekWithoutNameLsetHash, _ = peekedLset.HashWithoutLabels(buf, labels.MetricName, "le", "quantile")
		if peekWithoutNameLsetHash != currWithoutNameLsetHash {
			// CT line for a different series, for our series no CT.
			return nil
		}
		ct := int64(peek.val)
		return &ct
	}
}

// typeRequiresCT returns true if the metric type requires a _created timestamp.
func typeRequiresCT(t model.MetricType) bool {
	switch t {
	case model.MetricTypeCounter, model.MetricTypeSummary, model.MetricTypeHistogram:
		return true
	default:
		return false
	}
}

// deepCopy creates a copy of a parser without re-using the slices' original memory addresses.
func deepCopy(p *OpenMetricsParser) OpenMetricsParser {
	newB := make([]byte, len(p.l.b))
	copy(newB, p.l.b)

	newLexer := &openMetricsLexer{
		b:     newB,
		i:     p.l.i,
		start: p.l.start,
		err:   p.l.err,
		state: p.l.state,
	}

	newParser := OpenMetricsParser{
		l:       newLexer,
		builder: p.builder,
		mtype:   p.mtype,
		val:     p.val,
		skipCT:  false,
	}
	return newParser
}

// nextToken returns the next token from the openMetricsLexer.
func (p *OpenMetricsParser) nextToken() token {
	tok := p.l.Lex()
	return tok
}

func (p *OpenMetricsParser) parseError(exp string, got token) error {
	e := p.l.i + 1
	if len(p.l.b) < e {
		e = len(p.l.b)
	}
	return fmt.Errorf("%s, got %q (%q) while parsing: %q", exp, p.l.b[p.l.start:e], got, p.l.b[p.start:e])
}

// Next advances the parser to the next sample.
// It returns (EntryInvalid, io.EOF) if no samples were read.
func (p *OpenMetricsParser) Next() (Entry, error) {
	var err error

	p.start = p.l.i
	p.offsets = p.offsets[:0]
	p.eOffsets = p.eOffsets[:0]
	p.exemplar = p.exemplar[:0]
	p.exemplarVal = 0
	p.hasExemplarTs = false

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
			m := yoloString(p.l.b[p.offsets[0]:p.offsets[1]])
			u := yoloString(p.text)
			if len(u) > 0 {
				if !strings.HasSuffix(m, u) || len(m) < len(u)+1 || p.l.b[p.offsets[1]-len(u)-1] != '_' {
					return EntryInvalid, fmt.Errorf("unit %q not a suffix of metric %q", u, m)
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
		return p.parseMetricSuffix(p.nextToken())
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
		return p.parseMetricSuffix(t2)

	default:
		err = p.parseError("expected a valid start token", t)
	}
	return EntryInvalid, err
}

func (p *OpenMetricsParser) parseComment() error {
	var err error
	// Parse the labels.
	p.eOffsets, err = p.parseLVals(p.eOffsets, true)
	if err != nil {
		return err
	}
	p.exemplar = p.l.b[p.start:p.l.i]

	// Get the value.
	p.exemplarVal, err = p.getFloatValue(p.nextToken(), "exemplar labels")
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

func (p *OpenMetricsParser) parseLVals(offsets []int, isExemplar bool) ([]int, error) {
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

// parseMetricSuffix parses the end of the line after the metric name and
// labels. It starts parsing with the provided token.
func (p *OpenMetricsParser) parseMetricSuffix(t token) (Entry, error) {
	if p.offsets[0] == -1 {
		return EntryInvalid, fmt.Errorf("metric name not set while parsing: %q", p.l.b[p.start:p.l.i])
	}

	var err error
	p.val, err = p.getFloatValue(t, "metric")
	if err != nil {
		return EntryInvalid, err
	}

	p.hasTS = false
	switch t2 := p.nextToken(); t2 {
	case tEOF:
		return EntryInvalid, errors.New("data does not end with # EOF")
	case tLinebreak:
		break
	case tComment:
		if err := p.parseComment(); err != nil {
			return EntryInvalid, err
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

	if p.skipCT {
		var newLbs labels.Labels
		p.Metric(&newLbs)
		name := newLbs.Get(model.MetricNameLabel)
		switch p.mtype {
		case model.MetricTypeCounter, model.MetricTypeSummary, model.MetricTypeHistogram:
			if strings.HasSuffix(name, "_created") {
				return p.Next()
			}
		default:
			break
		}
	}

	return EntrySeries, nil
}

func (p *OpenMetricsParser) getFloatValue(t token, after string) (float64, error) {
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
