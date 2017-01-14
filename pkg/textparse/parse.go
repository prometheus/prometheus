//go:generate golex -o=lex.l.go lex.l
package textparse

import (
	"errors"
	"io"
	"reflect"
	"unsafe"

	"k8s.io/client-go/pkg/labels"
)

type lexer struct {
	b      []byte
	i      int
	vstart int

	mstart, mend int
	err          error
	val          float64
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

type Parser struct {
	l   *lexer
	err error
	val float64
}

func New(b []byte) *Parser {
	return &Parser{l: &lexer{b: b}}
}

func (p *Parser) Next() bool {
	switch p.l.Lex() {
	case 0, -1:
		return false
	case 1:
		return true
	}
	panic("unexpected")
}

func (p *Parser) At() ([]byte, *int64, float64) {
	return p.l.b[p.l.mstart:p.l.mend], nil, p.l.val
}

func (p *Parser) Err() error {
	if p.err != nil {
		return p.err
	}
	if p.l.err == io.EOF {
		return nil
	}
	return p.l.err
}

func (p *Parser) Metric() labels.Labels {
	return nil
}

func yoloString(b []byte) string {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&b))

	h := reflect.StringHeader{
		Data: sh.Data,
		Len:  sh.Len,
	}
	return *((*string)(unsafe.Pointer(&h)))
}
