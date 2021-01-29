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
	"io"
	"sort"
	"strings"
	"unsafe"

	"github.com/prometheus/prometheus/pkg/labels"
)

type lexer struct {
	b      []byte
	i      int
	vstart int
	tstart int

	err          error
	val          float64
	ts           *int64
	offsets      []int
	mstart, mend int
	nextMstart   int

	state int
}

const eof = 0

func (l *lexer) next() byte {
	l.i++
	if l.i >= len(l.b) {
		l.err = io.EOF
		return eof
	}
	c := l.b[l.i]

	// Consume null byte when encountered in label-value.
	if c == eof && (l.state == lstateLValueIn || l.state == lstateLValue) {
		return l.next()
	}
	return c
}

func (l *lexer) Error(es string) {
	l.err = errors.New(es)
}

// Parser parses samples from a byte slice of samples in the official
// Prometheus text exposition format.
type Parser struct {
	l   *lexer
	err error
	val float64
}

// New returns a new parser of the byte slice.
func New(b []byte) *Parser {
	return &Parser{l: &lexer{b: b}}
}

// Next advances the parser to the next sample. It returns false if no
// more samples were read or an error occurred.
func (p *Parser) Next() bool {
	switch p.l.Lex() {
	case -1, eof:
		return false
	case 1:
		return true
	}
	panic("unexpected")
}

// At returns the bytes of the metric, the timestamp if set, and the value
// of the current sample.
func (p *Parser) At() ([]byte, *int64, float64) {
	return p.l.b[p.l.mstart:p.l.mend], p.l.ts, p.l.val
}

// Err returns the current error.
func (p *Parser) Err() error {
	if p.err != nil {
		return p.err
	}
	if p.l.err == io.EOF {
		return nil
	}
	return p.l.err
}

// Metric writes the labels of the current sample into the passed labels.
// It returns the string from which the metric was parsed.
func (p *Parser) Metric(l *labels.Labels) string {
	// Allocate the full immutable string immediately, so we just
	// have to create references on it below.
	s := string(p.l.b[p.l.mstart:p.l.mend])

	*l = append(*l, labels.Label{
		Name:  labels.MetricName,
		Value: s[:p.l.offsets[0]-p.l.mstart],
	})

	for i := 1; i < len(p.l.offsets); i += 4 {
		a := p.l.offsets[i] - p.l.mstart
		b := p.l.offsets[i+1] - p.l.mstart
		c := p.l.offsets[i+2] - p.l.mstart
		d := p.l.offsets[i+3] - p.l.mstart

		// Replacer causes allocations. Replace only when necessary.
		if strings.IndexByte(s[c:d], byte('\\')) >= 0 {
			*l = append(*l, labels.Label{Name: s[a:b], Value: replacer.Replace(s[c:d])})
			continue
		}

		*l = append(*l, labels.Label{Name: s[a:b], Value: s[c:d]})
	}

	sort.Sort((*l)[1:])

	return s
}

var replacer = strings.NewReplacer(
	`\"`, `"`,
	`\\`, `\`,
	`\n`, `
`,
	`\t`, `	`,
)

func yoloString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}
