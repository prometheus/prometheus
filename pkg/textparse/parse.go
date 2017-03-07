//go:generate go get github.com/cznic/golex
//go:generate golex -o=lex.l.go lex.l

// Package textparse contains an efficient parser for the Prometheus text format.
package textparse

import (
	"errors"
	"io"
	"sort"
	"unsafe"

	"github.com/prometheus/prometheus/pkg/labels"
)

type lexer struct {
	b      []byte
	i      int
	vstart int

	err          error
	val          float64
	offsets      []int
	mstart, mend int
}

const eof = 0

func (l *lexer) next() byte {
	l.i++
	if l.i >= len(l.b) {
		l.err = io.EOF
		return eof
	}
	c := l.b[l.i]
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
	case 0, -1:
		return false
	case 1:
		return true
	}
	panic("unexpected")
}

// At returns the bytes of the metric, the timestamp if set, and the value
// of the current sample.
func (p *Parser) At() ([]byte, *int64, float64) {
	return p.l.b[p.l.mstart:p.l.mend], nil, p.l.val
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
func (p *Parser) Metric(l *labels.Labels) {
	// Allocate the full immutable string immediately, so we just
	// have to create references on it below.
	s := string(p.l.b[p.l.mstart:p.l.mend])

	*l = append(*l, labels.Label{
		Name:  labels.MetricName,
		Value: s[:p.l.offsets[0]-p.l.mstart],
	})

	for i := 1; i < len(p.l.offsets); i += 3 {
		a := p.l.offsets[i] - p.l.mstart
		b := p.l.offsets[i+1] - p.l.mstart
		c := p.l.offsets[i+2] - p.l.mstart

		*l = append(*l, labels.Label{Name: s[a:b], Value: s[b+2 : c]})
	}

	sort.Sort((*l)[1:])
}

func yoloString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}
