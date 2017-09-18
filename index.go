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

package tsdb

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"math"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"
)

const (
	// MagicIndex 4 bytes at the head of an index file.
	MagicIndex = 0xBAAAD700

	indexFormatV1 = 1
)

const indexFilename = "index"

const compactionPageBytes = minSectorSize * 64

type indexWriterSeries struct {
	labels labels.Labels
	chunks []ChunkMeta // series file offset of chunks
	offset uint32      // index file offset of series reference
}

type indexWriterSeriesSlice []*indexWriterSeries

func (s indexWriterSeriesSlice) Len() int      { return len(s) }
func (s indexWriterSeriesSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s indexWriterSeriesSlice) Less(i, j int) bool {
	return labels.Compare(s[i].labels, s[j].labels) < 0
}

type indexWriterStage uint8

const (
	idxStageNone indexWriterStage = iota
	idxStageSymbols
	idxStageSeries
	idxStageLabelIndex
	idxStagePostings
	idxStageDone
)

func (s indexWriterStage) String() string {
	switch s {
	case idxStageNone:
		return "none"
	case idxStageSymbols:
		return "symbols"
	case idxStageSeries:
		return "series"
	case idxStageLabelIndex:
		return "label index"
	case idxStagePostings:
		return "postings"
	case idxStageDone:
		return "done"
	}
	return "<unknown>"
}

// IndexWriter serializes the index for a block of series data.
// The methods must be called in the order they are specified in.
type IndexWriter interface {
	// AddSymbols registers all string symbols that are encountered in series
	// and other indices.
	AddSymbols(sym map[string]struct{}) error

	// AddSeries populates the index writer with a series and its offsets
	// of chunks that the index can reference.
	// Implementations may require series to be insert in increasing order by
	// their labels.
	// The reference numbers are used to resolve entries in postings lists that
	// are added later.
	AddSeries(ref uint64, l labels.Labels, chunks ...ChunkMeta) error

	// WriteLabelIndex serializes an index from label names to values.
	// The passed in values chained tuples of strings of the length of names.
	WriteLabelIndex(names []string, values []string) error

	// WritePostings writes a postings list for a single label pair.
	// The Postings here contain refs to the series that were added.
	WritePostings(name, value string, it Postings) error

	// Close writes any finalization and closes the resources associated with
	// the underlying writer.
	Close() error
}

// indexWriter implements the IndexWriter interface for the standard
// serialization format.
type indexWriter struct {
	f    *os.File
	fbuf *bufio.Writer
	pos  uint64

	toc   indexTOC
	stage indexWriterStage

	// Reusable memory.
	buf1    encbuf
	buf2    encbuf
	uint32s []uint32

	symbols       map[string]uint32 // symbol offsets
	seriesOffsets map[uint64]uint64 // offsets of series
	labelIndexes  []hashEntry       // label index offsets
	postings      []hashEntry       // postings lists offsets

	// Hold last series to validate that clients insert new series in order.
	lastSeries labels.Labels

	crc32 hash.Hash
}

type indexTOC struct {
	symbols           uint64
	series            uint64
	labelIndices      uint64
	labelIndicesTable uint64
	postings          uint64
	postingsTable     uint64
}

func newIndexWriter(dir string) (*indexWriter, error) {
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, indexFilename), os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}
	if err := fileutil.Fsync(df); err != nil {
		return nil, errors.Wrap(err, "sync dir")
	}

	iw := &indexWriter{
		f:     f,
		fbuf:  bufio.NewWriterSize(f, 1<<22),
		pos:   0,
		stage: idxStageNone,

		// Reusable memory.
		buf1:    encbuf{b: make([]byte, 0, 1<<22)},
		buf2:    encbuf{b: make([]byte, 0, 1<<22)},
		uint32s: make([]uint32, 0, 1<<15),

		// Caches.
		symbols:       make(map[string]uint32, 1<<13),
		seriesOffsets: make(map[uint64]uint64, 1<<16),
		crc32:         newCRC32(),
	}
	if err := iw.writeMeta(); err != nil {
		return nil, err
	}
	return iw, nil
}

func (w *indexWriter) write(bufs ...[]byte) error {
	for _, b := range bufs {
		n, err := w.fbuf.Write(b)
		w.pos += uint64(n)
		if err != nil {
			return err
		}
		// For now the index file must not grow beyond 4GiB. Some of the fixed-sized
		// offset references in v1 are only 4 bytes large.
		// Once we move to compressed/varint representations in those areas, this limitation
		// can be lifted.
		if w.pos > math.MaxUint32 {
			return errors.Errorf("exceeding max size of 4GiB")
		}
	}
	return nil
}

// addPadding adds zero byte padding until the file size is a multiple of n.
func (w *indexWriter) addPadding(n int) error {
	p := n - (int(w.pos) % n)
	if p == 0 {
		return nil
	}
	return errors.Wrap(w.write(make([]byte, p)), "add padding")
}

// ensureStage handles transitions between write stages and ensures that IndexWriter
// methods are called in an order valid for the implementation.
func (w *indexWriter) ensureStage(s indexWriterStage) error {
	if w.stage == s {
		return nil
	}
	if w.stage > s {
		return errors.Errorf("invalid stage %q, currently at %q", s, w.stage)
	}

	// Mark start of sections in table of contents.
	switch s {
	case idxStageSymbols:
		w.toc.symbols = w.pos
	case idxStageSeries:
		w.toc.series = w.pos

	case idxStageLabelIndex:
		w.toc.labelIndices = w.pos

	case idxStagePostings:
		w.toc.labelIndicesTable = w.pos
		if err := w.writeOffsetTable(w.labelIndexes); err != nil {
			return err
		}
		w.toc.postings = w.pos

	case idxStageDone:
		w.toc.postingsTable = w.pos
		if err := w.writeOffsetTable(w.postings); err != nil {
			return err
		}
		if err := w.writeTOC(); err != nil {
			return err
		}
	}

	w.stage = s
	return nil
}

func (w *indexWriter) writeMeta() error {
	w.buf1.reset()
	w.buf1.putBE32(MagicIndex)
	w.buf1.putByte(indexFormatV1)

	return w.write(w.buf1.get())
}

func (w *indexWriter) AddSeries(ref uint64, lset labels.Labels, chunks ...ChunkMeta) error {
	if err := w.ensureStage(idxStageSeries); err != nil {
		return err
	}
	if labels.Compare(lset, w.lastSeries) <= 0 {
		return errors.Errorf("out-of-order series added with label set %q", lset)
	}

	if _, ok := w.seriesOffsets[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	w.seriesOffsets[ref] = w.pos

	w.buf2.reset()
	w.buf2.putUvarint(len(lset))

	for _, l := range lset {
		offset, ok := w.symbols[l.Name]
		if !ok {
			return errors.Errorf("symbol entry for %q does not exist", l.Name)
		}
		w.buf2.putUvarint32(offset)

		offset, ok = w.symbols[l.Value]
		if !ok {
			return errors.Errorf("symbol entry for %q does not exist", l.Value)
		}
		w.buf2.putUvarint32(offset)
	}

	w.buf2.putUvarint(len(chunks))

	if len(chunks) > 0 {
		c := chunks[0]
		w.buf2.putVarint64(c.MinTime)
		w.buf2.putUvarint64(uint64(c.MaxTime - c.MinTime))
		w.buf2.putUvarint64(c.Ref)
		t0 := c.MaxTime
		ref0 := int64(c.Ref)

		for _, c := range chunks[1:] {
			w.buf2.putUvarint64(uint64(c.MinTime - t0))
			w.buf2.putUvarint64(uint64(c.MaxTime - c.MinTime))
			t0 = c.MaxTime

			w.buf2.putVarint64(int64(c.Ref) - ref0)
			ref0 = int64(c.Ref)
		}
	}

	w.buf1.reset()
	w.buf1.putUvarint(w.buf2.len())

	w.buf2.putHash(w.crc32)

	if err := w.write(w.buf1.get(), w.buf2.get()); err != nil {
		return errors.Wrap(err, "write series data")
	}

	w.lastSeries = append(w.lastSeries[:0], lset...)

	return nil
}

func (w *indexWriter) AddSymbols(sym map[string]struct{}) error {
	if err := w.ensureStage(idxStageSymbols); err != nil {
		return err
	}
	// Generate sorted list of strings we will store as reference table.
	symbols := make([]string, 0, len(sym))

	for s := range sym {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	const headerSize = 4

	w.buf1.reset()
	w.buf2.reset()

	w.buf2.putBE32int(len(symbols))

	w.symbols = make(map[string]uint32, len(symbols))

	for _, s := range symbols {
		w.symbols[s] = uint32(w.pos) + headerSize + uint32(w.buf2.len())
		w.buf2.putUvarintStr(s)
	}

	w.buf1.putBE32int(w.buf2.len())
	w.buf2.putHash(w.crc32)

	err := w.write(w.buf1.get(), w.buf2.get())
	return errors.Wrap(err, "write symbols")
}

func (w *indexWriter) WriteLabelIndex(names []string, values []string) error {
	if len(values)%len(names) != 0 {
		return errors.Errorf("invalid value list length %d for %d names", len(values), len(names))
	}
	if err := w.ensureStage(idxStageLabelIndex); err != nil {
		return errors.Wrap(err, "ensure stage")
	}

	valt, err := newStringTuples(values, len(names))
	if err != nil {
		return err
	}
	sort.Sort(valt)

	// Align beginning to 4 bytes for more efficient index list scans.
	if err := w.addPadding(4); err != nil {
		return err
	}

	w.labelIndexes = append(w.labelIndexes, hashEntry{
		keys:   names,
		offset: w.pos,
	})

	w.buf2.reset()
	w.buf2.putBE32int(len(names))
	w.buf2.putBE32int(valt.Len())

	for _, v := range valt.s {
		offset, ok := w.symbols[v]
		if !ok {
			return errors.Errorf("symbol entry for %q does not exist", v)
		}
		w.buf2.putBE32(offset)
	}

	w.buf1.reset()
	w.buf1.putBE32int(w.buf2.len())

	w.buf2.putHash(w.crc32)

	err = w.write(w.buf1.get(), w.buf2.get())
	return errors.Wrap(err, "write label index")
}

// writeOffsetTable writes a sequence of readable hash entries.
func (w *indexWriter) writeOffsetTable(entries []hashEntry) error {
	w.buf1.reset()
	w.buf1.putBE32int(len(entries))

	w.buf2.reset()

	for _, e := range entries {
		w.buf2.putUvarint(len(e.keys))
		for _, k := range e.keys {
			w.buf2.putUvarintStr(k)
		}
		w.buf2.putUvarint64(e.offset)
	}

	w.buf1.putBE32int(w.buf2.len())
	w.buf2.putHash(w.crc32)

	return w.write(w.buf1.get(), w.buf2.get())
}

const indexTOCLen = 6*8 + 4

func (w *indexWriter) writeTOC() error {
	w.buf1.reset()

	w.buf1.putBE64(w.toc.symbols)
	w.buf1.putBE64(w.toc.series)
	w.buf1.putBE64(w.toc.labelIndices)
	w.buf1.putBE64(w.toc.labelIndicesTable)
	w.buf1.putBE64(w.toc.postings)
	w.buf1.putBE64(w.toc.postingsTable)

	w.buf1.putHash(w.crc32)

	return w.write(w.buf1.get())
}

func (w *indexWriter) WritePostings(name, value string, it Postings) error {
	if err := w.ensureStage(idxStagePostings); err != nil {
		return errors.Wrap(err, "ensure stage")
	}

	// Align beginning to 4 bytes for more efficient postings list scans.
	if err := w.addPadding(4); err != nil {
		return err
	}

	w.postings = append(w.postings, hashEntry{
		keys:   []string{name, value},
		offset: w.pos,
	})

	// Order of the references in the postings list does not imply order
	// of the series references within the persisted block they are mapped to.
	// We have to sort the new references again.
	refs := w.uint32s[:0]

	for it.Next() {
		offset, ok := w.seriesOffsets[it.At()]
		if !ok {
			return errors.Errorf("%p series for reference %d not found", w, it.At())
		}
		if offset > (1<<32)-1 {
			return errors.Errorf("series offset %d exceeds 4 bytes", offset)
		}
		refs = append(refs, uint32(offset))
	}
	if err := it.Err(); err != nil {
		return err
	}
	sort.Sort(uint32slice(refs))

	w.buf2.reset()
	w.buf2.putBE32int(len(refs))

	for _, r := range refs {
		w.buf2.putBE32(r)
	}
	w.uint32s = refs

	w.buf1.reset()
	w.buf1.putBE32int(w.buf2.len())

	w.buf2.putHash(w.crc32)

	err := w.write(w.buf1.get(), w.buf2.get())
	return errors.Wrap(err, "write postings")
}

type uint32slice []uint32

func (s uint32slice) Len() int           { return len(s) }
func (s uint32slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s uint32slice) Less(i, j int) bool { return s[i] < s[j] }

type hashEntry struct {
	keys   []string
	offset uint64
}

func (w *indexWriter) Close() error {
	if err := w.ensureStage(idxStageDone); err != nil {
		return err
	}
	if err := w.fbuf.Flush(); err != nil {
		return err
	}
	if err := fileutil.Fsync(w.f); err != nil {
		return err
	}
	return w.f.Close()
}

// IndexReader provides reading access of serialized index data.
type IndexReader interface {
	// Symbols returns a set of string symbols that may occur in series' labels
	// and indices.
	Symbols() (map[string]struct{}, error)

	// LabelValues returns the possible label values
	LabelValues(names ...string) (StringTuples, error)

	// Postings returns the postings list iterator for the label pair.
	// The Postings here contain the offsets to the series inside the index.
	Postings(name, value string) (Postings, error)

	// SortedPostings returns a postings list that is reordered to be sorted
	// by the label set of the underlying series.
	SortedPostings(Postings) Postings

	// Series populates the given labels and chunk metas for the series identified
	// by the reference.
	Series(ref uint64, lset *labels.Labels, chks *[]ChunkMeta) error

	// LabelIndices returns the label pairs for which indices exist.
	LabelIndices() ([][]string, error)

	// Close released the underlying resources of the reader.
	Close() error
}

// StringTuples provides access to a sorted list of string tuples.
type StringTuples interface {
	// Total number of tuples in the list.
	Len() int
	// At returns the tuple at position i.
	At(i int) ([]string, error)
}

type indexReader struct {
	// The underlying byte slice holding the encoded series data.
	b   []byte
	toc indexTOC

	// Close that releases the underlying resources of the byte slice.
	c io.Closer

	// Cached hashmaps of section offsets.
	labels   map[string]uint32
	postings map[string]uint32
}

var (
	errInvalidSize = fmt.Errorf("invalid size")
	errInvalidFlag = fmt.Errorf("invalid flag")
)

// NewIndexReader returns a new IndexReader on the given directory.
func NewIndexReader(dir string) (IndexReader, error) { return newIndexReader(dir) }

// newIndexReader returns a new indexReader on the given directory.
func newIndexReader(dir string) (*indexReader, error) {
	f, err := openMmapFile(filepath.Join(dir, "index"))
	if err != nil {
		return nil, err
	}
	r := &indexReader{b: f.b, c: f}

	// Verify magic number.
	if len(f.b) < 4 {
		return nil, errors.Wrap(errInvalidSize, "index header")
	}
	if m := binary.BigEndian.Uint32(r.b[:4]); m != MagicIndex {
		return nil, errors.Errorf("invalid magic number %x", m)
	}

	if err := r.readTOC(); err != nil {
		return nil, errors.Wrap(err, "read TOC")
	}

	r.labels, err = r.readOffsetTable(r.toc.labelIndicesTable)
	if err != nil {
		return nil, errors.Wrap(err, "read label index table")
	}
	r.postings, err = r.readOffsetTable(r.toc.postingsTable)
	return r, errors.Wrap(err, "read postings table")
}

func (r *indexReader) readTOC() error {
	d := r.decbufAt(len(r.b) - indexTOCLen)

	r.toc.symbols = d.be64()
	r.toc.series = d.be64()
	r.toc.labelIndices = d.be64()
	r.toc.labelIndicesTable = d.be64()
	r.toc.postings = d.be64()
	r.toc.postingsTable = d.be64()

	// TODO(fabxc): validate checksum.

	return nil
}

func (r *indexReader) decbufAt(off int) decbuf {
	if len(r.b) < off {
		return decbuf{e: errInvalidSize}
	}
	return decbuf{b: r.b[off:]}
}

// readOffsetTable reads an offset table at the given position and returns a map
// with the key strings concatenated by the 0xff unicode non-character.
func (r *indexReader) readOffsetTable(off uint64) (map[string]uint32, error) {
	// A table might not have been written at all, in which case the position
	// is zeroed out.
	if off == 0 {
		return nil, nil
	}

	const sep = "\xff"

	var (
		d1  = r.decbufAt(int(off))
		cnt = d1.be32()
		d2  = d1.decbuf(d1.be32int())
	)

	res := make(map[string]uint32, 512)

	for d2.err() == nil && d2.len() > 0 && cnt > 0 {
		keyCount := int(d2.uvarint())
		keys := make([]string, 0, keyCount)

		for i := 0; i < keyCount; i++ {
			keys = append(keys, d2.uvarintTempStr())
		}
		res[strings.Join(keys, sep)] = uint32(d2.uvarint())

		cnt--
	}

	// TODO(fabxc): verify checksum from remainer of d1.
	return res, d2.err()
}

func (r *indexReader) Close() error {
	return r.c.Close()
}

func (r *indexReader) section(o uint32) (byte, []byte, error) {
	b := r.b[o:]

	if len(b) < 5 {
		return 0, nil, errors.Wrap(errInvalidSize, "read header")
	}

	flag := b[0]
	l := binary.BigEndian.Uint32(b[1:5])

	b = b[5:]

	// b must have the given length plus 4 bytes for the CRC32 checksum.
	if len(b) < int(l)+4 {
		return 0, nil, errors.Wrap(errInvalidSize, "section content")
	}
	return flag, b[:l], nil
}

func (r *indexReader) lookupSymbol(o uint32) (string, error) {
	d := r.decbufAt(int(o))

	s := d.uvarintTempStr()
	if d.err() != nil {
		return "", errors.Wrapf(d.err(), "read symbol at %d", o)
	}
	return s, nil
}

func (r *indexReader) Symbols() (map[string]struct{}, error) {
	d1 := r.decbufAt(int(r.toc.symbols))
	d2 := d1.decbuf(d1.be32int())

	count := d2.be32int()
	sym := make(map[string]struct{}, count)

	for ; count > 0; count-- {
		s := d2.uvarintTempStr()
		sym[s] = struct{}{}
	}

	return sym, d2.err()
}

func (r *indexReader) LabelValues(names ...string) (StringTuples, error) {
	const sep = "\xff"

	key := strings.Join(names, sep)
	off, ok := r.labels[key]
	if !ok {
		// XXX(fabxc): hot fix. Should return a partial data error and handle cases
		// where the entire block has no data gracefully.
		return emptyStringTuples{}, nil
		//return nil, fmt.Errorf("label index doesn't exist")
	}

	d1 := r.decbufAt(int(off))
	d2 := d1.decbuf(d1.be32int())

	nc := d2.be32int()
	d2.be32() // consume unused value entry count.

	if d2.err() != nil {
		return nil, errors.Wrap(d2.err(), "read label value index")
	}

	// TODO(fabxc): verify checksum in 4 remaining bytes of d1.

	st := &serializedStringTuples{
		l:      nc,
		b:      d2.get(),
		lookup: r.lookupSymbol,
	}
	return st, nil
}

type emptyStringTuples struct{}

func (emptyStringTuples) At(i int) ([]string, error) { return nil, nil }
func (emptyStringTuples) Len() int                   { return 0 }

func (r *indexReader) LabelIndices() ([][]string, error) {
	const sep = "\xff"

	res := [][]string{}

	for s := range r.labels {
		res = append(res, strings.Split(s, sep))
	}
	return res, nil
}

func (r *indexReader) Series(ref uint64, lbls *labels.Labels, chks *[]ChunkMeta) error {
	d1 := r.decbufAt(int(ref))
	d2 := d1.decbuf(int(d1.uvarint()))

	*lbls = (*lbls)[:0]
	*chks = (*chks)[:0]

	k := int(d2.uvarint())

	for i := 0; i < k; i++ {
		lno := uint32(d2.uvarint())
		lvo := uint32(d2.uvarint())

		if d2.err() != nil {
			return errors.Wrap(d2.err(), "read series label offsets")
		}

		ln, err := r.lookupSymbol(lno)
		if err != nil {
			return errors.Wrap(err, "lookup label name")
		}
		lv, err := r.lookupSymbol(lvo)
		if err != nil {
			return errors.Wrap(err, "lookup label value")
		}

		*lbls = append(*lbls, labels.Label{Name: ln, Value: lv})
	}

	// Read the chunks meta data.
	k = int(d2.uvarint())

	if k == 0 {
		return nil
	}

	t0 := d2.varint64()
	maxt := int64(d2.uvarint64()) + t0
	ref0 := int64(d2.uvarint64())

	*chks = append(*chks, ChunkMeta{
		Ref:     uint64(ref0),
		MinTime: t0,
		MaxTime: maxt,
	})
	t0 = maxt

	for i := 1; i < k; i++ {
		mint := int64(d2.uvarint64()) + t0
		maxt := int64(d2.uvarint64()) + mint

		ref0 += d2.varint64()
		t0 = maxt

		if d2.err() != nil {
			return errors.Wrapf(d2.err(), "read meta for chunk %d", i)
		}

		*chks = append(*chks, ChunkMeta{
			Ref:     uint64(ref0),
			MinTime: mint,
			MaxTime: maxt,
		})
	}

	// TODO(fabxc): verify CRC32.

	return nil
}

func (r *indexReader) Postings(name, value string) (Postings, error) {
	const sep = "\xff"
	key := strings.Join([]string{name, value}, sep)

	off, ok := r.postings[key]
	if !ok {
		return emptyPostings, nil
	}

	d1 := r.decbufAt(int(off))
	d2 := d1.decbuf(d1.be32int())

	d2.be32() // consume unused postings list length.

	if d2.err() != nil {
		return nil, errors.Wrap(d2.err(), "get postings bytes")
	}

	// TODO(fabxc): read checksum from 4 remainer bytes of d1 and verify.

	return newBigEndianPostings(d2.get()), nil
}

func (r *indexReader) SortedPostings(p Postings) Postings {
	return p
}

type stringTuples struct {
	l int      // tuple length
	s []string // flattened tuple entries
}

func newStringTuples(s []string, l int) (*stringTuples, error) {
	if len(s)%l != 0 {
		return nil, errors.Wrap(errInvalidSize, "string tuple list")
	}
	return &stringTuples{s: s, l: l}, nil
}

func (t *stringTuples) Len() int                   { return len(t.s) / t.l }
func (t *stringTuples) At(i int) ([]string, error) { return t.s[i : i+t.l], nil }

func (t *stringTuples) Swap(i, j int) {
	c := make([]string, t.l)
	copy(c, t.s[i:i+t.l])

	for k := 0; k < t.l; k++ {
		t.s[i+k] = t.s[j+k]
		t.s[j+k] = c[k]
	}
}

func (t *stringTuples) Less(i, j int) bool {
	for k := 0; k < t.l; k++ {
		d := strings.Compare(t.s[i+k], t.s[j+k])

		if d < 0 {
			return true
		}
		if d > 0 {
			return false
		}
	}
	return false
}

type serializedStringTuples struct {
	l      int
	b      []byte
	lookup func(uint32) (string, error)
}

func (t *serializedStringTuples) Len() int {
	return len(t.b) / (4 * t.l)
}

func (t *serializedStringTuples) At(i int) ([]string, error) {
	if len(t.b) < (i+t.l)*4 {
		return nil, errInvalidSize
	}
	res := make([]string, 0, t.l)

	for k := 0; k < t.l; k++ {
		offset := binary.BigEndian.Uint32(t.b[(i+k)*4:])

		s, err := t.lookup(offset)
		if err != nil {
			return nil, errors.Wrap(err, "symbol lookup")
		}
		res = append(res, s)
	}

	return res, nil
}
