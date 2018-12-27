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
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/value"
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

func (l *openMetricsLexer) cur() byte {
	return l.b[l.i]
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
	series  []byte
	text    []byte
	mtype   MetricType
	val     float64
	ts      int64
	hasTS   bool
	start   int
	offsets []int
}

// New returns a new parser of the byte slice.
func NewOpenMetricsParser(b []byte) Parser {
	return &OpenMetricsParser{l: &openMetricsLexer{b: b}}
}

// Series returns the bytes of the series, the timestamp if set, and the value
// of the current sample.
func (p *OpenMetricsParser) Series() ([]byte, *int64, float64) {
	if p.hasTS {
		return p.series, &p.ts, p.val
	}
	return p.series, nil, p.val
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
func (p *OpenMetricsParser) Type() ([]byte, MetricType) {
	return p.l.b[p.offsets[0]:p.offsets[1]], p.mtype
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
	// Allocate the full immutable string immediately, so we just
	// have to create references on it below.
	s := string(p.series)

	*l = append(*l, labels.Label{
		Name:  labels.MetricName,
		Value: s[:p.offsets[0]-p.start],
	})

	for i := 1; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start

		// Replacer causes allocations. Replace only when necessary.
		if strings.IndexByte(s[c:d], byte('\\')) >= 0 {
			*l = append(*l, labels.Label{Name: s[a:b], Value: lvalReplacer.Replace(s[c:d])})
			continue
		}
		*l = append(*l, labels.Label{Name: s[a:b], Value: s[c:d]})
	}

	// Sort labels. We can skip the first entry since the metric name is
	// already at the right place.
	sort.Sort((*l)[1:])

	return s
}

// nextToken returns the next token from the openMetricsLexer.
func (p *OpenMetricsParser) nextToken() token {
	tok := p.l.Lex()
	return tok
}

// Next advances the parser to the next sample. It returns false if no
// more samples were read or an error occurred.
func (p *OpenMetricsParser) Next() (Entry, error) {
	var err error

	p.start = p.l.i
	p.offsets = p.offsets[:0]

	switch t := p.nextToken(); t {
	case tEofWord:
		if t := p.nextToken(); t != tEOF {
			return EntryInvalid, fmt.Errorf("unexpected data after # EOF")
		}
		return EntryInvalid, io.EOF
	case tEOF:
		return EntryInvalid, parseError("unexpected end of data", t)
	case tHelp, tType, tUnit:
		switch t := p.nextToken(); t {
		case tMName:
			p.offsets = append(p.offsets, p.l.start, p.l.i)
		default:
			return EntryInvalid, parseError("expected metric name after HELP", t)
		}
		switch t := p.nextToken(); t {
		case tText:
			if len(p.l.buf()) > 1 {
				p.text = p.l.buf()[1 : len(p.l.buf())-1]
			} else {
				p.text = []byte{}
			}
		default:
			return EntryInvalid, parseError("expected text in HELP", t)
		}
		switch t {
		case tType:
			switch s := yoloString(p.text); s {
			case "counter":
				p.mtype = MetricTypeCounter
			case "gauge":
				p.mtype = MetricTypeGauge
			case "histogram":
				p.mtype = MetricTypeHistogram
			case "gaugehistogram":
				p.mtype = MetricTypeGaugeHistogram
			case "summary":
				p.mtype = MetricTypeSummary
			case "info":
				p.mtype = MetricTypeInfo
			case "stateset":
				p.mtype = MetricTypeStateset
			case "unknown":
				p.mtype = MetricTypeUnknown
			default:
				return EntryInvalid, fmt.Errorf("invalid metric type %q", s)
			}
		case tHelp:
			if !utf8.Valid(p.text) {
				return EntryInvalid, fmt.Errorf("help text is not a valid utf8 string")
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
					return EntryInvalid, fmt.Errorf("unit not a suffix of metric %q", m)
				}
			}
			return EntryUnit, nil
		}

	case tMName:
		p.offsets = append(p.offsets, p.l.i)
		p.series = p.l.b[p.start:p.l.i]

		t2 := p.nextToken()
		if t2 == tBraceOpen {
			if err := p.parseLVals(); err != nil {
				return EntryInvalid, err
			}
			p.series = p.l.b[p.start:p.l.i]
			t2 = p.nextToken()
		}
		if t2 != tValue {
			return EntryInvalid, parseError("expected value after metric", t)
		}
		if p.val, err = strconv.ParseFloat(yoloString(p.l.buf()[1:]), 64); err != nil {
			return EntryInvalid, err
		}
		// Ensure canonical NaN value.
		if math.IsNaN(p.val) {
			p.val = math.Float64frombits(value.NormalNaN)
		}
		p.hasTS = false
		switch p.nextToken() {
		case tLinebreak:
			break
		case tTimestamp:
			p.hasTS = true
			var ts float64
			// A float is enough to hold what we need for millisecond resolution.
			if ts, err = strconv.ParseFloat(yoloString(p.l.buf()[1:]), 64); err != nil {
				return EntryInvalid, err
			}
			p.ts = int64(ts * 1000)
			if t2 := p.nextToken(); t2 != tLinebreak {
				return EntryInvalid, parseError("expected next entry after timestamp", t)
			}
		default:
			return EntryInvalid, parseError("expected timestamp or new record", t)
		}
		return EntrySeries, nil

	default:
		err = fmt.Errorf("%q %q is not a valid start token", t, string(p.l.cur()))
	}
	return EntryInvalid, err
}

func (p *OpenMetricsParser) parseLVals() error {
	first := true
	for {
		t := p.nextToken()
		switch t {
		case tBraceClose:
			return nil
		case tComma:
			if first {
				return parseError("expected label name or left brace", t)
			}
			t = p.nextToken()
			if t != tLName {
				return parseError("expected label name", t)
			}
		case tLName:
			if !first {
				return parseError("expected comma", t)
			}
		default:
			if first {
				return parseError("expected label name or left brace", t)
			}
			return parseError("expected comma or left brace", t)

		}
		first = false
		// t is now a label name.

		p.offsets = append(p.offsets, p.l.start, p.l.i)

		if t := p.nextToken(); t != tEqual {
			return parseError("expected equal", t)
		}
		if t := p.nextToken(); t != tLValue {
			return parseError("expected label value", t)
		}
		if !utf8.Valid(p.l.buf()) {
			return fmt.Errorf("invalid UTF-8 label value")
		}

		// The openMetricsLexer ensures the value string is quoted. Strip first
		// and last character.
		p.offsets = append(p.offsets, p.l.start+1, p.l.i-1)

	}
}
