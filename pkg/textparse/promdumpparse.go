// Copyright 2020 The Prometheus Authors
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
//go:generate golex -o=promdumplex.l.go promdumplex.l

package textparse

import (
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/value"
)

type promDumpLexer struct {
	b     []byte
	i     int
	start int
	err   error
	state int
}

// buf returns the buffer of the current token.
func (l *promDumpLexer) buf() []byte {
	return l.b[l.start:l.i]
}

// next advances the promDumpLexer to the next character.
func (l *promDumpLexer) next() byte {
	l.i++
	if l.i >= len(l.b) {
		l.err = io.EOF
		return byte(tEOF)
	}
	// Lex struggles with null bytes. If we are in a label value or help string, where
	// they are allowed, consume them here immediately.
	for l.b[l.i] == 0 && (l.state == sLValue) {
		l.i++
	}
	return l.b[l.i]
}

func (l *promDumpLexer) Error(es string) {
	l.err = errors.New(es)
}

// PromDumpParser parses samples from a byte slice of samples in the format
// of "promtool tsdb dump".
type PromDumpParser struct {
	l       *promDumpLexer
	series  []byte
	val     float64
	ts      int64
	start   int
	offsets []int
}

// NewPromDumpParser returns a new parser of the byte slice.
func NewPromDumpParser(b []byte) Parser {
	return &PromDumpParser{l: &promDumpLexer{b: append(b, '\n')}}
}

// Series returns the bytes of the series, the timestamp, and the value
// of the current sample.
func (p *PromDumpParser) Series() ([]byte, *int64, float64) {
	metric := p.series[(p.offsets[1] - p.start):(p.offsets[2] - p.start)]
	return metric, &p.ts, p.val
}

// Help returns the metric name and help text in the current entry.
// Must only be called after Next returned a help entry.
// The returned byte slices become invalid after the next call to Next.
func (p *PromDumpParser) Help() ([]byte, []byte) {
	// The dump format does not include help
	return nil, nil
}

// Type returns the metric name and type in the current entry.
// Must only be called after Next returned a type entry.
// The returned byte slices become invalid after the next call to Next.
func (p *PromDumpParser) Type() ([]byte, MetricType) {
	// The dump format does not include type
	return nil, MetricTypeUnknown
}

// Unit returns the metric name and unit in the current entry.
// Must only be called after Next returned a unit entry.
// The returned byte slices become invalid after the next call to Next.
func (p *PromDumpParser) Unit() ([]byte, []byte) {
	// The dump format does not have units.
	return nil, nil
}

// Comment returns the text of the current comment.
// Must only be called after Next returned a comment entry.
// The returned byte slice becomes invalid after the next call to Next.
func (p *PromDumpParser) Comment() []byte {
	// The dump format does not have comments.
	return nil
}

// Metric writes the labels of the current sample into the passed labels.
// It returns the string from which the metric was parsed.
func (p *PromDumpParser) Metric(l *labels.Labels) string {
	// Allocate the full immutable string immediately, so we just
	// have to create references on it below.
	s := string(p.series)

	*l = append(*l, labels.Label{
		Name:  labels.MetricName,
		Value: s[(p.offsets[1] - p.start):(p.offsets[2] - p.start)],
	})

	for i := 3; i < len(p.offsets); i += 4 {
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

	// Sort labels to maintain the sorted labels invariant.
	sort.Sort(*l)

	return s
}

// Exemplar writes the exemplar of the current sample into the passed
// exemplar. It returns if an exemplar exists.
func (p *PromDumpParser) Exemplar(e *exemplar.Exemplar) bool {
	// The dump format does not include exemplars
	return false
}

// Next advances the parser to the next sample. It returns false if no
// more samples were read or an error occurred.
func (p *PromDumpParser) Next() (Entry, error) {
	var err error

	p.start = p.l.i
	p.offsets = p.offsets[:0]

	switch t := p.nextToken(); t {
	case tEOF:
		return EntryInvalid, io.EOF
	case tLinebreak:
		// Allow full blank lines.
		return p.Next()

	case tBraceOpen:
		p.offsets = append(p.offsets, p.l.i)
		p.series = p.l.b[p.start:p.l.i]

		t2 := p.nextToken()
		if t2 != tMNameLabel {
			return EntryInvalid, parseError("expected __name__ label after open brace", t2)
		}

		t2 = p.nextToken()
		if t2 != tEqual {
			return EntryInvalid, parseError("expected equal sign after __name__ label", t2)
		}

		t2 = p.nextToken()
		if t2 != tMName {
			return EntryInvalid, parseError("expected valid metric name after __name__ label", t2)
		}
		// The promDumpLexer ensures the value string is quoted. Strip first
		// and last character.
		p.offsets = append(p.offsets, p.l.start+1, p.l.i-1)

		if err := p.parseLVals(); err != nil {
			return EntryInvalid, err
		}

		p.series = p.l.b[p.start:p.l.i]
		t2 = p.nextToken()

		if t2 != tValue {
			return EntryInvalid, parseError("expected value after metric", t2)
		}
		if p.val, err = parseFloat(yoloString(p.l.buf())); err != nil {
			return EntryInvalid, err
		}

		// Ensure canonical NaN value.
		if math.IsNaN(p.val) {
			p.val = math.Float64frombits(value.NormalNaN)
		}

		t2 = p.nextToken()
		if t2 != tTimestamp {
			return EntryInvalid, parseError("expected timestamp after metric value", t2)
		}
		if p.ts, err = strconv.ParseInt(yoloString(p.l.buf()), 10, 64); err != nil {
			return EntryInvalid, err
		}

		t2 = p.nextToken()
		if t2 != tLinebreak {
			return EntryInvalid, parseError("expected next entry after timestamp", t2)
		}
		return EntrySeries, nil

	default:
		err = errors.Errorf("%q is not a valid start token", t)
	}
	return EntryInvalid, err
}

// nextToken returns the next token from the promDumpLexer. It skips over tabs
// and spaces.
func (p *PromDumpParser) nextToken() token {
	for {
		if tok := p.l.Lex(); tok != tWhitespace {
			return tok
		}
	}
}

func (p *PromDumpParser) parseLVals() error {
	for {
		token := p.nextToken()

		if token != tComma && token != tBraceClose {
			return parseError("expected comma or closing brace after label value", token)
		}
		switch token {
		case tBraceClose:
			return nil
		}

		token = p.nextToken()
		if token != tLName {
			return parseError("expected label name", token)
		}
		// Add label names offsets
		p.offsets = append(p.offsets, p.l.start, p.l.i)

		token = p.nextToken()
		if token != tEqual {
			return parseError("expected equal", token)
		}
		token = p.nextToken()
		if token != tLValue {
			return parseError("expected label value", token)
		}
		if !utf8.Valid(p.l.buf()) {
			return errors.Errorf("invalid UTF-8 label value")
		}

		// Add label value offsets
		// The promDumpLexer ensures the value string is quoted. Strip first
		// and last character.
		p.offsets = append(p.offsets, p.l.start+1, p.l.i-1)
	}
}
