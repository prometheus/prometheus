// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0.

package index

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

const (
	pagedTableMagic      = "PPOT0001"
	pagedTableHeaderSize = 28
)

type (
	pagedEntry struct {
		value  string
		offset uint64
	}
	pageDescriptor struct {
		offset, size, lastOffset uint64
		count, lastLen           uint32
	}
	postingsTableBuilder struct {
		file                   *FileWriter
		start                  uint64
		pos                    uint64
		limit                  int
		target                 int
		entries                []pagedEntry
		valueBytes             int
		name, prefix, previous string
		started                bool
		pages                  []pageDescriptor
		groups                 uint32
		directory              encoding.Encbuf
	}
)

func newPostingsTableBuilder(f *FileWriter) (*postingsTableBuilder, error) {
	b := &postingsTableBuilder{file: f, start: f.pos, pos: f.pos, limit: 128, target: 64 << 10}
	return b, b.write(make([]byte, pagedTableHeaderSize))
}

func (b *postingsTableBuilder) write(p []byte) error {
	err := b.file.Write(p)
	b.pos = b.file.pos
	return err
}

func (b *postingsTableBuilder) add(name, value string, offset uint64) error {
	if uint64(len(value)) > math.MaxUint32-1024 {
		return errors.New("value exceeds page limit")
	}
	if b.started && (name < b.name || name == b.name && value <= b.previous) {
		return errors.New("keys must be strictly sorted")
	}
	if !b.started || name != b.name {
		if b.started {
			if err := b.finishGroup(); err != nil {
				return err
			}
		}
		b.started = true
		b.name = name
		b.prefix = value[:min(len(value), 64<<10)]
	} else {
		n := min(len(b.prefix), len(value))
		i := 0
		for i < n && b.prefix[i] == value[i] {
			i++
		}
		b.prefix = b.prefix[:i]
	}
	if len(b.entries) > 0 && (len(b.entries) >= b.limit || uint64(b.valueBytes)+uint64(len(value))+uint64(12*(len(b.entries)+1)) > uint64(b.target)) {
		if err := b.flush(); err != nil {
			return err
		}
	}
	b.entries = append(b.entries, pagedEntry{value, offset})
	b.valueBytes += len(value)
	b.previous = value
	return nil
}

func (b *postingsTableBuilder) flush() error {
	if len(b.entries) == 0 {
		return nil
	}
	base, high := b.entries[0].offset, b.entries[0].offset
	for _, e := range b.entries {
		base = min(base, e.offset)
		high = max(high, e.offset)
	}
	width := 1
	for width < 8 && high-base > uint64(1)<<(width*8)-1 {
		width *= 2
	}
	n := len(b.entries)
	header := 20 + 4*(n+1) + width*n
	size := uint64(header) + uint64(b.valueBytes) + 4
	if size > math.MaxUint32 || size > uint64(math.MaxInt) {
		return encoding.ErrInvalidSize
	}
	payload := make([]byte, int(size))
	binary.BigEndian.PutUint32(payload, uint32(n))
	binary.BigEndian.PutUint32(payload[4:], uint32(b.valueBytes))
	binary.BigEndian.PutUint64(payload[8:], base)
	payload[16] = byte(width)
	at := 0
	for i, e := range b.entries {
		binary.BigEndian.PutUint32(payload[20+4*i:], uint32(at))
		delta := e.offset - base
		start := 20 + 4*(n+1) + i*width
		for j := range width {
			payload[start+j] = byte(delta >> ((width - 1 - j) * 8))
		}
		at += copy(payload[header+at:], e.value)
	}
	binary.BigEndian.PutUint32(payload[20+4*n:], uint32(at))
	binary.BigEndian.PutUint32(payload[len(payload)-4:], crc32.Checksum(payload[:len(payload)-4], castagnoliTable))
	last := b.entries[n-1].value
	d := pageDescriptor{b.pos, uint64(len(payload)), b.pos + uint64(header+at-len(last)), uint32(n), uint32(len(last))}
	if err := b.write(payload); err != nil {
		return err
	}
	b.pages = append(b.pages, d)
	clear(b.entries)
	b.entries = b.entries[:0]
	b.valueBytes = 0
	return nil
}

func (b *postingsTableBuilder) finishGroup() error {
	if err := b.flush(); err != nil {
		return err
	}
	prefix := b.prefix
	if len(b.pages) == 1 {
		prefix = ""
	}
	if err := b.file.Flush(); err != nil {
		return err
	}
	b.directory.PutUvarintStr(b.name)
	b.directory.PutUvarintStr(prefix)
	b.directory.PutBE32(uint32(len(b.pages)))
	var suffix []byte
	for _, p := range b.pages {
		b.directory.PutBE64(p.offset)
		b.directory.PutBE32(uint32(p.size))
		b.directory.PutBE32(p.count)
		if len(b.pages) > 1 {
			suffix = slices.Grow(suffix[:0], int(p.lastLen)-len(prefix))[:int(p.lastLen)-len(prefix)]
			if _, err := b.file.f.ReadAt(suffix, int64(p.lastOffset)+int64(len(prefix))); err != nil {
				return err
			}
			b.directory.PutUvarintBytes(suffix)
		} else {
			b.directory.PutUvarintStr("")
		}
	}
	b.groups++
	b.pages = b.pages[:0]
	b.previous = ""
	return nil
}

func (b *postingsTableBuilder) finish() error {
	if b.started {
		if err := b.finishGroup(); err != nil {
			return err
		}
	}
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], b.groups)
	root := append(count[:], b.directory.B...)
	root = binary.BigEndian.AppendUint32(root, crc32.Checksum(root, castagnoliTable))
	off := b.pos
	if err := b.write(root); err != nil {
		return err
	}
	header := append([]byte{}, pagedTableMagic...)
	header = binary.BigEndian.AppendUint64(header, off)
	header = binary.BigEndian.AppendUint64(header, uint64(len(root)))
	header = binary.BigEndian.AppendUint32(header, crc32.Checksum(header, castagnoliTable))
	return b.file.WriteAt(header, b.start)
}

// The directory is read eagerly; immutable value pages are validated on first use.
// Returned strings borrow the reader's ByteSlice and share its lifetime.
type pagedPostingsTable struct {
	bs                   ByteSlice
	start, postingsStart uint64
	groups               map[string]*postingsGroup
}

type postingsGroup struct {
	prefix string
	count  int
	pages  []*postingsPage
}

type postingsPage struct {
	offset               uint64
	size                 uint32
	count                int
	end                  string
	once                 sync.Once
	err                  error
	starts, deltas, data []byte
	base                 uint64
	width                int
}

func checkedRange(bs ByteSlice, off, size uint64) ([]byte, error) {
	if off > uint64(bs.Len()) || size > uint64(bs.Len())-off {
		return nil, encoding.ErrInvalidSize
	}
	return bs.Range(int(off), int(off+size)), nil
}

func readPagedPostingsTable(bs ByteSlice, start, end, postingsStart uint64) (*pagedPostingsTable, error) {
	h, err := checkedRange(bs, start, pagedTableHeaderSize)
	if err != nil {
		return nil, err
	}
	if string(h[:8]) != pagedTableMagic || crc32.Checksum(h[:24], castagnoliTable) != binary.BigEndian.Uint32(h[24:]) {
		return nil, errors.New("invalid paged postings header")
	}
	off, size := binary.BigEndian.Uint64(h[8:]), binary.BigEndian.Uint64(h[16:])
	if off < start+pagedTableHeaderSize || off > end || size < 8 || size > end-off {
		return nil, encoding.ErrInvalidSize
	}
	root, err := checkedRange(bs, off, size)
	if err != nil {
		return nil, err
	}
	if crc32.Checksum(root[:len(root)-4], castagnoliTable) != binary.BigEndian.Uint32(root[len(root)-4:]) {
		return nil, encoding.ErrInvalidChecksum
	}
	d := encoding.Decbuf{B: root[:len(root)-4]}
	n := d.Be32()
	if uint64(n) > uint64(d.Len())/23 {
		return nil, encoding.ErrInvalidSize
	}
	r := &pagedPostingsTable{bs: bs, start: start, postingsStart: postingsStart, groups: make(map[string]*postingsGroup)}
	previous := ""
	nextPage := start + pagedTableHeaderSize
	var total uint64
	for i := range n {
		name, prefix := d.UvarintStr(), d.UvarintStr()
		np := d.Be32()
		if d.Err() != nil || len(prefix) > 64<<10 || np == 0 || uint64(np) > uint64(d.Len())/17 || i > 0 && name <= previous {
			return nil, errors.New("invalid postings group")
		}
		previous = name
		g := &postingsGroup{prefix: prefix, pages: make([]*postingsPage, 0, np)}
		lastEnd := ""
		for j := range np {
			po, ps, count := d.Be64(), d.Be32(), d.Be32()
			fence := d.UvarintStr()
			if d.Err() != nil || po != nextPage || po > off || uint64(ps) > off-po || ps < 33 || count == 0 || count > 128 || j > 0 && fence <= lastEnd {
				return nil, errors.New("invalid postings page descriptor")
			}
			if uint64(g.count)+uint64(count) > uint64(math.MaxInt) {
				return nil, encoding.ErrInvalidSize
			}
			g.count += int(count)
			total += uint64(count)
			if total > math.MaxUint32 {
				return nil, errors.New("too many postings entries")
			}
			g.pages = append(g.pages, &postingsPage{offset: po, size: ps, count: int(count), end: fence})
			nextPage = po + uint64(ps)
			lastEnd = fence
		}
		if np == 1 && (prefix != "" || lastEnd != "") {
			return nil, errors.New("invalid single-page routing")
		}
		r.groups[name] = g
	}
	if d.Err() != nil || d.Len() != 0 || nextPage != off {
		return nil, errors.New("invalid postings directory")
	}
	return r, nil
}

func (r *pagedPostingsTable) load(g *postingsGroup, page int) (*postingsPage, error) {
	p := g.pages[page]
	p.once.Do(func() {
		b, err := checkedRange(r.bs, p.offset, uint64(p.size))
		if err != nil {
			p.err = err
			return
		}
		if crc32.Checksum(b[:len(b)-4], castagnoliTable) != binary.BigEndian.Uint32(b[len(b)-4:]) {
			p.err = encoding.ErrInvalidChecksum
			return
		}
		n, nb := uint64(binary.BigEndian.Uint32(b)), uint64(binary.BigEndian.Uint32(b[4:]))
		width := uint64(b[16])
		if n != uint64(p.count) || width != 1 && width != 2 && width != 4 && width != 8 || b[17] != 0 || b[18] != 0 || b[19] != 0 {
			p.err = errors.New("invalid postings page header")
			return
		}
		end := 20 + 4*(n+1) + width*n
		if end+nb+4 != uint64(len(b)) {
			p.err = encoding.ErrInvalidSize
			return
		}
		p.base = binary.BigEndian.Uint64(b[8:])
		p.width = int(width)
		p.starts = b[20 : 20+4*(n+1)]
		p.deltas = b[20+4*(n+1) : end]
		p.data = b[end : end+nb]
		prev := uint32(0)
		for i := 0; i <= p.count; i++ {
			next := binary.BigEndian.Uint32(p.starts[4*i:])
			if uint64(next) > nb || next < prev || i == 0 && next != 0 || i == p.count && uint64(next) != nb {
				p.err = errors.New("invalid postings value offsets")
				return
			}
			prev = next
		}
		for i := range p.count {
			value := p.value(i)
			if i > 0 && value <= p.value(i-1) || !strings.HasPrefix(value, g.prefix) {
				p.err = errors.New("unsorted postings values")
				return
			}
			delta := p.delta(i)
			if delta > math.MaxUint64-p.base || p.base+delta < r.postingsStart || p.base+delta >= r.start {
				p.err = errors.New("postings offset outside postings section")
				return
			}
		}
		if len(g.pages) > 1 && (p.value(p.count - 1)[len(g.prefix):] != p.end || page > 0 && p.value(0)[len(g.prefix):] <= g.pages[page-1].end) {
			p.err = errors.New("postings page does not match routing")
			return
		}
	})
	return p, p.err
}

func (p *postingsPage) value(i int) string {
	a, b := binary.BigEndian.Uint32(p.starts[4*i:]), binary.BigEndian.Uint32(p.starts[4*(i+1):])
	return yoloString(p.data[a:b])
}

func (p *postingsPage) delta(i int) uint64 {
	b := p.deltas[i*p.width:]
	switch p.width {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(binary.BigEndian.Uint16(b))
	case 4:
		return uint64(binary.BigEndian.Uint32(b))
	default:
		return binary.BigEndian.Uint64(b)
	}
}

func (r *pagedPostingsTable) lookup(g *postingsGroup, value string) (uint64, bool, error) {
	page := 0
	if len(g.pages) > 1 {
		if !strings.HasPrefix(value, g.prefix) {
			return 0, false, nil
		}
		suffix := value[len(g.prefix):]
		page = sort.Search(len(g.pages), func(i int) bool { return g.pages[i].end >= suffix })
		if page == len(g.pages) {
			return 0, false, nil
		}
	}
	p, err := r.load(g, page)
	if err != nil {
		return 0, false, err
	}
	i := sort.Search(p.count, func(i int) bool { return p.value(i) >= value })
	if i == p.count || p.value(i) != value {
		return 0, false, nil
	}
	return p.base + p.delta(i), true, nil
}

func (r *pagedPostingsTable) traverse(ctx context.Context, name string, f func(string, uint64, int) (bool, error)) error {
	g := r.groups[name]
	if g == nil {
		return ctx.Err()
	}
	for pi := range g.pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		p, err := r.load(g, pi)
		if err != nil {
			return err
		}
		for i := range p.count {
			if err := ctx.Err(); err != nil {
				return err
			}
			pos := p.offset + 20 + uint64(4*i) - r.start
			ok, err := f(p.value(i), p.base+p.delta(i), int(pos))
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
	}
	return nil
}

func (w *Writer) writePagedPostingsOffsetTable() error {
	if w.cntPO > math.MaxUint32 {
		return errors.New("too many postings entries")
	}
	if err := w.fPO.Flush(); err != nil {
		return err
	}
	f, err := fileutil.OpenMmapFile(w.fPO.name)
	if err != nil {
		return err
	}
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	b, err := newPostingsTableBuilder(w.f)
	if err != nil {
		return err
	}
	d := encoding.NewDecbufRaw(realByteSlice(f.Bytes()), int(w.fPO.pos))
	for range w.cntPO {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		if d.Uvarint() != 2 {
			return errors.New("invalid temporary postings key")
		}
		name, value := yoloString(d.UvarintBytes()), yoloString(d.UvarintBytes())
		offset := d.Uvarint64()
		if d.Err() != nil {
			return d.Err()
		}
		if offset > math.MaxUint64-w.postingsStart {
			return encoding.ErrInvalidSize
		}
		if err := b.add(name, value, offset+w.postingsStart); err != nil {
			return err
		}
	}
	if d.Err() != nil || d.Len() != 0 {
		return errors.New("invalid temporary postings table")
	}
	if err := b.finish(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	f = nil
	if err := w.fPO.Close(); err != nil {
		return err
	}
	if err := w.fPO.Remove(); err != nil {
		return err
	}
	w.fPO = nil
	return nil
}

// SymbolDecbuf keeps the legacy symbol payload and count, widening only its length.
func symbolDecbuf(bs ByteSlice, version, off int) encoding.Decbuf {
	if version != FormatV4 {
		return encoding.NewDecbufAt(bs, off, castagnoliTable)
	}
	if off < 0 {
		return encoding.Decbuf{E: encoding.ErrInvalidSize}
	}
	header, err := checkedRange(bs, uint64(off), 8)
	if err != nil {
		return encoding.Decbuf{E: err}
	}
	size := binary.BigEndian.Uint64(header)
	if size < 4 || size > uint64(bs.Len())-uint64(off)-8 || uint64(bs.Len())-uint64(off)-8-size < 4 {
		return encoding.Decbuf{E: encoding.ErrInvalidSize}
	}
	b, err := checkedRange(bs, uint64(off)+8, size+4)
	if err != nil {
		return encoding.Decbuf{E: err}
	}
	if crc32.Checksum(b[:len(b)-4], castagnoliTable) != binary.BigEndian.Uint32(b[len(b)-4:]) {
		return encoding.Decbuf{E: encoding.ErrInvalidChecksum}
	}
	return encoding.Decbuf{B: b[:len(b)-4]}
}

func (r *Reader) pagedLabelValues(ctx context.Context, name string, hints *storage.LabelHints) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g := r.paged.groups[name]
	if g == nil {
		return nil, nil
	}
	count := g.count
	if hints != nil && hints.Limit > 0 {
		count = min(count, hints.Limit)
	}
	values := make([]string, 0, count)
	for pi := range g.pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p, err := r.paged.load(g, pi)
		if err != nil {
			return nil, err
		}
		for i := 0; i < p.count && len(values) < count; i++ {
			values = append(values, p.value(i))
		}
		if len(values) == count {
			break
		}
	}
	return values, nil
}

func (r *Reader) decodePagedPostings(off uint64) (Postings, error) {
	h, err := checkedRange(r.b, off, 4)
	if err != nil {
		return nil, err
	}
	size := uint64(binary.BigEndian.Uint32(h))
	if off > r.toc.PostingsTable || r.toc.PostingsTable-off < 8 || size > r.toc.PostingsTable-off-8 {
		return nil, encoding.ErrInvalidSize
	}
	_, p, err := r.dec.DecodePostings(encoding.NewDecbufAt(r.b, int(off), castagnoliTable))
	return p, err
}

func (r *Reader) pagedPostings(ctx context.Context, name string, values ...string) (Postings, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g := r.paged.groups[name]
	if g == nil || len(values) == 0 {
		return EmptyPostings(), nil
	}
	if len(values) == 1 {
		off, found, err := r.paged.lookup(g, values[0])
		if err != nil {
			return nil, err
		}
		if !found {
			return EmptyPostings(), nil
		}
		return r.decodePagedPostings(off)
	}
	slices.Sort(values)
	result := make([]Postings, 0, len(values))
	for _, value := range values {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		off, found, err := r.paged.lookup(g, value)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		p, err := r.decodePagedPostings(off)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return Merge(ctx, result...), nil
}

func (r *Reader) pagedPostingsMatching(ctx context.Context, name string, match func(string) bool) Postings {
	var result []Postings
	err := r.paged.traverse(ctx, name, func(value string, offset uint64, _ int) (bool, error) {
		if match != nil && !match(value) {
			return true, nil
		}
		p, err := r.decodePagedPostings(offset)
		if err != nil {
			return false, err
		}
		result = append(result, p)
		return true, nil
	})
	if err != nil {
		return ErrPostings(err)
	}
	return Merge(ctx, result...)
}
