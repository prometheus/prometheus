package index

import (
	"encoding/binary"
	"errors"
	"io"
)

const pageSize = 2048

var errPageFull = errors.New("page full")

type pageCursor interface {
	Iterator
	append(v DocID) error
}

type page interface {
	cursor() pageCursor
	init(v DocID) error
	data() []byte
}

type pageDelta struct {
	b []byte
}

type pageType uint8

const (
	pageTypeDelta pageType = iota
)

func newPageDelta(data []byte) *pageDelta {
	return &pageDelta{b: data}
}

func (p *pageDelta) init(v DocID) error {
	// Write first value.
	binary.PutUvarint(p.b, uint64(v))
	return nil
}

func (p *pageDelta) cursor() pageCursor {
	return &pageDeltaCursor{data: p.b}
}

func (p *pageDelta) data() []byte {
	return p.b
}

type pageDeltaCursor struct {
	data []byte
	pos  int
	cur  DocID
}

func (p *pageDeltaCursor) append(id DocID) error {
	// Run to the end.
	_, err := p.Next()
	for ; err == nil; _, err = p.Next() {
		// Consume.
	}
	if err != io.EOF {
		return err
	}
	if len(p.data)-p.pos < binary.MaxVarintLen64 {
		return errPageFull
	}
	if p.cur >= id {
		return errOutOfOrder
	}
	p.pos += binary.PutUvarint(p.data[p.pos:], uint64(id-p.cur))
	p.cur = id
	return nil
}

func (p *pageDeltaCursor) Close() error {
	return nil
}

func (p *pageDeltaCursor) Seek(min DocID) (v DocID, err error) {
	if min < p.cur {
		p.pos = 0
	}
	for v, err = p.Next(); err == nil && v < min; v, err = p.Next() {
		// Consume.
	}
	return p.cur, err
}

func (p *pageDeltaCursor) Next() (DocID, error) {
	var n int
	var dv uint64
	if p.pos == 0 {
		dv, n = binary.Uvarint(p.data)
		p.cur = DocID(dv)
	} else {
		dv, n = binary.Uvarint(p.data[p.pos:])
		if n <= 0 || dv == 0 {
			return 0, io.EOF
		}
		p.cur += DocID(dv)
	}
	p.pos += n

	return p.cur, nil
}
