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
	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
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
	mname   []byte
	val     float64
	h       *histogram.Histogram
	ts      int64
	hasTS   bool
	start   int
	offsets []int

	eOffsets      []int
	exemplar      []byte
	exemplarVal   float64
	exemplarTs    int64
	hasExemplarTs bool
}

// NewOpenMetricsParser returns a new parser of the byte slice.
func NewOpenMetricsParser(b []byte) Parser {
	return &OpenMetricsParser{l: &openMetricsLexer{b: b}}
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

// Histogram returns the bytes of the series, the timestamp if set, and the parsed histogram. Currently float histograms are not supported in the text format.
func (p *OpenMetricsParser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	if p.hasTS {
		ts := p.ts
		return p.series, &ts, p.h, nil
	}
	return p.series, nil, p.h, nil
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
	return p.mname, p.mtype
}

// Unit returns the metric name and unit in the current entry.
// Must only be called after Next returned a unit entry.
// The returned byte slices become invalid after the next call to Next.
func (p *OpenMetricsParser) Unit() ([]byte, []byte) {
	// The Prometheus format does not have units.
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
	p.builder.Add(labels.MetricName, s[:p.offsets[0]-p.start])

	for i := 1; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start

		value := s[c:d]
		// Replacer causes allocations. Replace only when necessary.
		if strings.IndexByte(s[c:d], byte('\\')) >= 0 {
			value = lvalReplacer.Replace(value)
		}
		p.builder.Add(s[a:b], value)
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

// CreatedTimestamp returns nil as it's not implemented yet.
// TODO(bwplotka): https://github.com/prometheus/prometheus/issues/12980
func (p *OpenMetricsParser) CreatedTimestamp() *int64 {
	return nil
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

// Next advances the parser to the next sample. It returns false if no
// more samples were read or an error occurred.
func (p *OpenMetricsParser) Next() (Entry, error) {
	var err error

	p.start = p.l.i
	p.offsets = p.offsets[:0]
	p.eOffsets = p.eOffsets[:0]
	p.exemplar = p.exemplar[:0]
	p.h = nil
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
			p.offsets = append(p.offsets, p.l.start, p.l.i)
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
			p.mname = p.l.b[p.offsets[0]:p.offsets[1]]
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

	case tMName:
		p.offsets = append(p.offsets, p.l.i)
		name := p.l.b[p.start:p.l.i]
		p.series = name

		t2 := p.nextToken()
		if t2 == tBraceOpen {
			p.offsets, err = p.parseLVals(p.offsets)
			if err != nil {
				return EntryInvalid, err
			}
			p.series = p.l.b[p.start:p.l.i]
			t2 = p.nextToken()
		}
		// We are parsing a native histogram if the name of this series matches
		// the name from the type metadata.
		if (p.mtype == model.MetricTypeGaugeHistogram || p.mtype == model.MetricTypeHistogram) &&
			bytes.Equal(name, p.mname) {
			p.h, err = p.getHistogramValue(t2)
		} else {
			p.val, err = p.getFloatValue(t2, "metric")
		}
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
		default:
			return EntryInvalid, p.parseError("expected timestamp or # symbol", t2)
		}
		if p.h != nil {
			return EntryHistogram, nil
		}
		return EntrySeries, nil

	default:
		err = p.parseError("expected a valid start token", t)
	}
	return EntryInvalid, err
}

func (p *OpenMetricsParser) parseComment() error {
	var err error
	// Parse the labels.
	p.eOffsets, err = p.parseLVals(p.eOffsets)
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

func (p *OpenMetricsParser) parseLVals(offsets []int) ([]int, error) {
	first := true
	for {
		t := p.nextToken()
		switch t {
		case tBraceClose:
			return offsets, nil
		case tComma:
			if first {
				return nil, p.parseError("expected label name or left brace", t)
			}
			t = p.nextToken()
			if t != tLName {
				return nil, p.parseError("expected label name", t)
			}
		case tLName:
			if !first {
				return nil, p.parseError("expected comma", t)
			}
		default:
			if first {
				return nil, p.parseError("expected label name or left brace", t)
			}
			return nil, p.parseError("expected comma or left brace", t)

		}
		first = false
		// t is now a label name.

		offsets = append(offsets, p.l.start, p.l.i)

		if t := p.nextToken(); t != tEqual {
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
	}
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

func (p *OpenMetricsParser) getHistogramValue(t token) (*histogram.Histogram, error) {
	if t != tValue {
		return nil, p.parseError("expected value after metric", t)
	}

	h, err := parseHistogram(p.l.buf()[1:])
	if err != nil {
		return nil, err
	}

	ht := dto.MetricType_HISTOGRAM
	if p.mtype == model.MetricTypeGaugeHistogram {
		ht = dto.MetricType_GAUGE_HISTOGRAM
	}
	sh := convertHistogram(h, ht)
	return &sh, nil
}

func parseHistogram(val []byte) (*dto.Histogram, error) {
	r := bytes.NewReader(val)
	ch, _, err := r.ReadRune()
	if err != nil || ch != '{' {
		return nil, fmt.Errorf("expected histogram to start with '{': %w", err)
	}
	h := dto.Histogram{}
	for {
		key, err := readKey(r)
		if err != nil {
			break
		}
		switch key {
		case "schema":
			var v int32
			_, err := fmt.Fscanf(r, "%d", &v)
			if err != nil {
				return nil, err
			}
			h.Schema = v
		case "zero_threshold":
			var v float64
			_, err := fmt.Fscanf(r, "%f", &v)
			if err != nil {
				return nil, err
			}
			h.ZeroThreshold = v
		case "zero_count":
			var v uint64
			_, err := fmt.Fscanf(r, "%d", &v)
			if err != nil {
				return nil, err
			}
			h.ZeroCount = v
		case "sample_count":
			var v uint64
			_, err := fmt.Fscanf(r, "%d", &v)
			if err != nil {
				return nil, err
			}
			h.SampleCount = v
		case "sample_sum":
			var v float64
			_, err := fmt.Fscanf(r, "%f", &v)
			if err != nil {
				return nil, err
			}
			h.SampleSum = v
		case "positive_span":
			spans, err := parseSpans(r)
			if err != nil {
				return nil, err
			}
			h.PositiveSpan = spans
		case "negative_span":
			spans, err := parseSpans(r)
			if err != nil {
				return nil, err
			}
			h.NegativeSpan = spans
		case "positive_delta":
			deltas, err := parseDeltas(r)
			if err != nil {
				return nil, err
			}
			h.PositiveDelta = deltas
		case "negative_delta":
			deltas, err := parseDeltas(r)
			if err != nil {
				return nil, err
			}
			h.NegativeDelta = deltas
		default:
			return nil, fmt.Errorf("unknown key: '%s'", key)
		}
	}
	return &h, nil
}

func readKey(r *bytes.Reader) (string, error) {
	var b strings.Builder
	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			return "", err
		}

		if ch == ',' {
			continue
		}

		if !isKeyRune(ch) {
			if ch == '}' {
				return "", io.EOF
			}
			return b.String(), nil
		}
		_, err = b.WriteRune(ch)
		if err != nil {
			return "", err
		}
	}
}

func isKeyRune(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func parseSpans(r *bytes.Reader) ([]dto.BucketSpan, error) {
	ch, _, err := r.ReadRune()
	if err != nil {
		return nil, err
	}
	if ch != '[' {
		return nil, errors.New("expected spans to begin with '['")
	}
	spans := []dto.BucketSpan{}

	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			return nil, err
		}
		if ch == ',' {
			continue
		}
		if ch == ']' {
			return spans, nil
		}
		// Unread the first character of the number before parsing the bucket.
		if err = r.UnreadRune(); err != nil {
			return nil, err
		}
		var (
			offset int32
			length uint32
		)
		_, err = fmt.Fscanf(r, "%d:%d", &offset, &length)
		if err != nil {
			return nil, fmt.Errorf("could not parse bucket: %w", err)
		}
		spans = append(spans, dto.BucketSpan{
			Offset: offset,
			Length: length,
		})
	}
}

func parseDeltas(r *bytes.Reader) ([]int64, error) {
	ch, _, err := r.ReadRune()
	if err != nil {
		return nil, err
	}
	if ch != '[' {
		return nil, errors.New("expected deltas to begin with '['")
	}
	deltas := []int64{}

	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			return nil, err
		}
		if ch == ',' {
			continue
		}
		if ch == ']' {
			return deltas, nil
		}
		// Unread the first character of the number before parsing the value.
		if err = r.UnreadRune(); err != nil {
			return nil, err
		}
		var delta int64
		fmt.Fscanf(r, "%d", &delta)
		deltas = append(deltas, delta)
	}
}
