// Copyright 2017 The Prometheus Authors
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

//go:generate go get github.com/cznic/golex
//go:generate golex -o=lex.l.go lex.l

// Package textparse contains an efficient parser for the Prometheus text format.
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
	"unsafe"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/value"
)

type lexer struct {
	b     []byte
	i     int
	start int
	err   error
	state int
}

type token int

const (
	tInvalid   token = -1
	tEOF       token = 0
	tLinebreak token = iota
	tWhitespace
	tHelp
	tType
	tText
	tComment
	tBlank
	tMName
	tBraceOpen
	tBraceClose
	tLName
	tLValue
	tComma
	tEqual
	tTimestamp
	tValue
)

func (t token) String() string {
	switch t {
	case tInvalid:
		return "INVALID"
	case tEOF:
		return "EOF"
	case tLinebreak:
		return "LINEBREAK"
	case tWhitespace:
		return "WHITESPACE"
	case tHelp:
		return "HELP"
	case tType:
		return "TYPE"
	case tText:
		return "TEXT"
	case tComment:
		return "COMMENT"
	case tBlank:
		return "BLANK"
	case tMName:
		return "MNAME"
	case tBraceOpen:
		return "BOPEN"
	case tBraceClose:
		return "BCLOSE"
	case tLName:
		return "LNAME"
	case tLValue:
		return "LVALUE"
	case tEqual:
		return "EQUAL"
	case tComma:
		return "COMMA"
	case tTimestamp:
		return "TIMESTAMP"
	case tValue:
		return "VALUE"
	}
	return fmt.Sprintf("<invalid: %d>", t)
}

// buf returns the buffer of the current token.
func (l *lexer) buf() []byte {
	return l.b[l.start:l.i]
}

func (l *lexer) cur() byte {
	return l.b[l.i]
}

// next advances the lexer to the next character.
func (l *lexer) next() byte {
	l.i++
	if l.i >= len(l.b) {
		l.err = io.EOF
		return byte(tEOF)
	}
	// Lex struggles with null bytes. If we are in a label value or help string, where
	// they are allowed, consume them here immediately.
	for l.b[l.i] == 0 && (l.state == sLValue || l.state == sMeta2 || l.state == sComment) {
		l.i++
	}
	return l.b[l.i]
}

func (l *lexer) Error(es string) {
	l.err = errors.New(es)
}

// Parser parses samples from a byte slice of samples in the official
// Prometheus text exposition format.
type Parser struct {
	l       *lexer
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
func New(b []byte) *Parser {
	return &Parser{l: &lexer{b: append(b, '\n')}}
}

// Series returns the bytes of the series, the timestamp if set, and the value
// of the current sample.
func (p *Parser) Series() ([]byte, *int64, float64) {
	if p.hasTS {
		return p.series, &p.ts, p.val
	}
	return p.series, nil, p.val
}

// Help returns the metric name and help text in the current entry.
// Must only be called after Next returned a help entry.
// The returned byte slices become invalid after the next call to Next.
func (p *Parser) Help() ([]byte, []byte) {
	m := p.l.b[p.offsets[0]:p.offsets[1]]

	// Replacer causes allocations. Replace only when necessary.
	if strings.IndexByte(yoloString(p.text), byte('\\')) >= 0 {
		return m, []byte(helpReplacer.Replace(string(p.text)))
	}
	return m, p.text
}

// Type returns the metric name and type in the current entry.
// Must only be called after Next returned a type entry.
// The returned byte slices become invalid after the next call to Next.
func (p *Parser) Type() ([]byte, MetricType) {
	return p.l.b[p.offsets[0]:p.offsets[1]], p.mtype
}

// Comment returns the text of the current comment.
// Must only be called after Next returned a comment entry.
// The returned byte slice becomes invalid after the next call to Next.
func (p *Parser) Comment() []byte {
	return p.text
}

// Metric writes the labels of the current sample into the passed labels.
// It returns the string from which the metric was parsed.
func (p *Parser) Metric(l *labels.Labels) string {
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

// nextToken returns the next token from the lexer. It skips over tabs
// and spaces.
func (p *Parser) nextToken() token {
	for {
		if tok := p.l.Lex(); tok != tWhitespace {
			return tok
		}
	}
}

// Entry represents the type of a parsed entry.
type Entry int

const (
	EntryInvalid Entry = -1
	EntryType    Entry = 0
	EntryHelp    Entry = 1
	EntrySeries  Entry = 2
	EntryComment Entry = 3
)

// MetricType represents metric type values.
type MetricType string

const (
	MetricTypeCounter   = "counter"
	MetricTypeGauge     = "gauge"
	MetricTypeHistogram = "histogram"
	MetricTypeSummary   = "summary"
	MetricTypeUntyped   = "untyped"
)

func parseError(exp string, got token) error {
	return fmt.Errorf("%s, got %q", exp, got)
}

// Next advances the parser to the next sample. It returns false if no
// more samples were read or an error occurred.
func (p *Parser) Next() (Entry, error) {
	var err error

	p.start = p.l.i
	p.offsets = p.offsets[:0]

	switch t := p.nextToken(); t {
	case tEOF:
		return EntryInvalid, io.EOF
	case tLinebreak:
		// Allow full blank lines.
		return p.Next()

	case tHelp, tType:
		switch t := p.nextToken(); t {
		case tMName:
			p.offsets = append(p.offsets, p.l.start, p.l.i)
		default:
			return EntryInvalid, parseError("expected metric name after HELP", t)
		}
		switch t := p.nextToken(); t {
		case tText:
			if len(p.l.buf()) > 1 {
				p.text = p.l.buf()[1:]
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
			case "summary":
				p.mtype = MetricTypeSummary
			case "untyped":
				p.mtype = MetricTypeUntyped
			default:
				return EntryInvalid, fmt.Errorf("invalid metric type %q", s)
			}
		case tHelp:
			if !utf8.Valid(p.text) {
				return EntryInvalid, fmt.Errorf("help text is not a valid utf8 string")
			}
		}
		if t := p.nextToken(); t != tLinebreak {
			return EntryInvalid, parseError("linebreak expected after metadata", t)
		}
		switch t {
		case tHelp:
			return EntryHelp, nil
		case tType:
			return EntryType, nil
		}
	case tComment:
		p.text = p.l.buf()
		if t := p.nextToken(); t != tLinebreak {
			return EntryInvalid, parseError("linebreak expected after comment", t)
		}
		return EntryComment, nil

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
		if p.val, err = strconv.ParseFloat(yoloString(p.l.buf()), 64); err != nil {
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
			if p.ts, err = strconv.ParseInt(yoloString(p.l.buf()), 10, 64); err != nil {
				return EntryInvalid, err
			}
			if t2 := p.nextToken(); t2 != tLinebreak {
				return EntryInvalid, parseError("expected next entry after timestamp", t)
			}
		default:
			return EntryInvalid, parseError("expected timestamp or new record", t)
		}
		return EntrySeries, nil

	default:
		err = fmt.Errorf("%q is not a valid start token", t)
	}
	return EntryInvalid, err
}

func (p *Parser) parseLVals() error {
	t := p.nextToken()
	for {
		switch t {
		case tBraceClose:
			return nil
		case tLName:
		default:
			return parseError("expected label name", t)
		}
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

		// The lexer ensures the value string is quoted. Strip first
		// and last character.
		p.offsets = append(p.offsets, p.l.start+1, p.l.i-1)

		// Free trailing commas are allowed.
		if t = p.nextToken(); t == tComma {
			t = p.nextToken()
		}
	}
}

var lvalReplacer = strings.NewReplacer(
	`\"`, "\"",
	`\\`, "\\",
	`\n`, "\n",
)

var helpReplacer = strings.NewReplacer(
	`\\`, "\\",
	`\n`, "\n",
)

func yoloString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}
