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

package index

import (
	"bufio"
	"encoding/binary"
	"hash"
	"hash/crc32"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/encoding"
	tsdb_errors "github.com/prometheus/tsdb/errors"
	"github.com/prometheus/tsdb/fileutil"
	"github.com/prometheus/tsdb/labels"
)

const (
	// MagicIndex 4 bytes at the head of an index file.
	MagicIndex = 0xBAAAD700
	// HeaderLen represents number of bytes reserved of index for header.
	HeaderLen = 5

	// FormatV1 represents 1 version of index.
	FormatV1 = 1
	// FormatV2 represents 2 version of index.
	FormatV2 = 2

	labelNameSeperator = "\xff"

	indexFilename = "index"
)

type indexWriterSeries struct {
	labels labels.Labels
	chunks []chunks.Meta // series file offset of chunks
	offset uint32        // index file offset of series reference
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

// The table gets initialized with sync.Once but may still cause a race
// with any other use of the crc32 package anywhere. Thus we initialize it
// before.
var castagnoliTable *crc32.Table

func init() {
	castagnoliTable = crc32.MakeTable(crc32.Castagnoli)
}

// newCRC32 initializes a CRC32 hash with a preconfigured polynomial, so the
// polynomial may be easily changed in one location at a later time, if necessary.
func newCRC32() hash.Hash32 {
	return crc32.New(castagnoliTable)
}

// Writer implements the IndexWriter interface for the standard
// serialization format.
type Writer struct {
	f    *os.File
	fbuf *bufio.Writer
	pos  uint64

	toc   TOC
	stage indexWriterStage

	// Reusable memory.
	buf1    encoding.Encbuf
	buf2    encoding.Encbuf
	uint32s []uint32

	symbols       map[string]uint32 // symbol offsets
	seriesOffsets map[uint64]uint64 // offsets of series
	labelIndexes  []hashEntry       // label index offsets
	postings      []hashEntry       // postings lists offsets

	// Hold last series to validate that clients insert new series in order.
	lastSeries labels.Labels

	crc32 hash.Hash

	Version int
}

// TOC represents index Table Of Content that states where each section of index starts.
type TOC struct {
	Symbols           uint64
	Series            uint64
	LabelIndices      uint64
	LabelIndicesTable uint64
	Postings          uint64
	PostingsTable     uint64
}

// NewTOCFromByteSlice return parsed TOC from given index byte slice.
func NewTOCFromByteSlice(bs ByteSlice) (*TOC, error) {
	if bs.Len() < indexTOCLen {
		return nil, encoding.ErrInvalidSize
	}
	b := bs.Range(bs.Len()-indexTOCLen, bs.Len())

	expCRC := binary.BigEndian.Uint32(b[len(b)-4:])
	d := encoding.Decbuf{B: b[:len(b)-4]}

	if d.Crc32(castagnoliTable) != expCRC {
		return nil, errors.Wrap(encoding.ErrInvalidChecksum, "read TOC")
	}

	if err := d.Err(); err != nil {
		return nil, err
	}

	return &TOC{
		Symbols:           d.Be64(),
		Series:            d.Be64(),
		LabelIndices:      d.Be64(),
		LabelIndicesTable: d.Be64(),
		Postings:          d.Be64(),
		PostingsTable:     d.Be64(),
	}, nil
}

// NewWriter returns a new Writer to the given filename. It serializes data in format version 2.
func NewWriter(fn string) (*Writer, error) {
	dir := filepath.Dir(fn)

	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}
	defer df.Close() // Close for platform windows.

	if err := os.RemoveAll(fn); err != nil {
		return nil, errors.Wrap(err, "remove any existing index at path")
	}

	f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}
	if err := df.Sync(); err != nil {
		return nil, errors.Wrap(err, "sync dir")
	}

	iw := &Writer{
		f:     f,
		fbuf:  bufio.NewWriterSize(f, 1<<22),
		pos:   0,
		stage: idxStageNone,

		// Reusable memory.
		buf1:    encoding.Encbuf{B: make([]byte, 0, 1<<22)},
		buf2:    encoding.Encbuf{B: make([]byte, 0, 1<<22)},
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

func (w *Writer) write(bufs ...[]byte) error {
	for _, b := range bufs {
		n, err := w.fbuf.Write(b)
		w.pos += uint64(n)
		if err != nil {
			return err
		}
		// For now the index file must not grow beyond 64GiB. Some of the fixed-sized
		// offset references in v1 are only 4 bytes large.
		// Once we move to compressed/varint representations in those areas, this limitation
		// can be lifted.
		if w.pos > 16*math.MaxUint32 {
			return errors.Errorf("exceeding max size of 64GiB")
		}
	}
	return nil
}

// addPadding adds zero byte padding until the file size is a multiple size.
func (w *Writer) addPadding(size int) error {
	p := w.pos % uint64(size)
	if p == 0 {
		return nil
	}
	p = uint64(size) - p
	return errors.Wrap(w.write(make([]byte, p)), "add padding")
}

// ensureStage handles transitions between write stages and ensures that IndexWriter
// methods are called in an order valid for the implementation.
func (w *Writer) ensureStage(s indexWriterStage) error {
	if w.stage == s {
		return nil
	}
	if w.stage > s {
		return errors.Errorf("invalid stage %q, currently at %q", s, w.stage)
	}

	// Mark start of sections in table of contents.
	switch s {
	case idxStageSymbols:
		w.toc.Symbols = w.pos
	case idxStageSeries:
		w.toc.Series = w.pos

	case idxStageLabelIndex:
		w.toc.LabelIndices = w.pos

	case idxStagePostings:
		w.toc.Postings = w.pos

	case idxStageDone:
		w.toc.LabelIndicesTable = w.pos
		if err := w.writeOffsetTable(w.labelIndexes); err != nil {
			return err
		}
		w.toc.PostingsTable = w.pos
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

func (w *Writer) writeMeta() error {
	w.buf1.Reset()
	w.buf1.PutBE32(MagicIndex)
	w.buf1.PutByte(FormatV2)

	return w.write(w.buf1.Get())
}

// AddSeries adds the series one at a time along with its chunks.
func (w *Writer) AddSeries(ref uint64, lset labels.Labels, chunks ...chunks.Meta) error {
	if err := w.ensureStage(idxStageSeries); err != nil {
		return err
	}
	if labels.Compare(lset, w.lastSeries) <= 0 {
		return errors.Errorf("out-of-order series added with label set %q", lset)
	}

	if _, ok := w.seriesOffsets[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	// We add padding to 16 bytes to increase the addressable space we get through 4 byte
	// series references.
	if err := w.addPadding(16); err != nil {
		return errors.Errorf("failed to write padding bytes: %v", err)
	}

	if w.pos%16 != 0 {
		return errors.Errorf("series write not 16-byte aligned at %d", w.pos)
	}
	w.seriesOffsets[ref] = w.pos / 16

	w.buf2.Reset()
	w.buf2.PutUvarint(len(lset))

	for _, l := range lset {
		// here we have an index for the symbol file if v2, otherwise it's an offset
		index, ok := w.symbols[l.Name]
		if !ok {
			return errors.Errorf("symbol entry for %q does not exist", l.Name)
		}
		w.buf2.PutUvarint32(index)

		index, ok = w.symbols[l.Value]
		if !ok {
			return errors.Errorf("symbol entry for %q does not exist", l.Value)
		}
		w.buf2.PutUvarint32(index)
	}

	w.buf2.PutUvarint(len(chunks))

	if len(chunks) > 0 {
		c := chunks[0]
		w.buf2.PutVarint64(c.MinTime)
		w.buf2.PutUvarint64(uint64(c.MaxTime - c.MinTime))
		w.buf2.PutUvarint64(c.Ref)
		t0 := c.MaxTime
		ref0 := int64(c.Ref)

		for _, c := range chunks[1:] {
			w.buf2.PutUvarint64(uint64(c.MinTime - t0))
			w.buf2.PutUvarint64(uint64(c.MaxTime - c.MinTime))
			t0 = c.MaxTime

			w.buf2.PutVarint64(int64(c.Ref) - ref0)
			ref0 = int64(c.Ref)
		}
	}

	w.buf1.Reset()
	w.buf1.PutUvarint(w.buf2.Len())

	w.buf2.PutHash(w.crc32)

	if err := w.write(w.buf1.Get(), w.buf2.Get()); err != nil {
		return errors.Wrap(err, "write series data")
	}

	w.lastSeries = append(w.lastSeries[:0], lset...)

	return nil
}

func (w *Writer) AddSymbols(sym map[string]struct{}) error {
	if err := w.ensureStage(idxStageSymbols); err != nil {
		return err
	}
	// Generate sorted list of strings we will store as reference table.
	symbols := make([]string, 0, len(sym))

	for s := range sym {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	w.buf1.Reset()
	w.buf2.Reset()

	w.buf2.PutBE32int(len(symbols))

	w.symbols = make(map[string]uint32, len(symbols))

	for index, s := range symbols {
		w.symbols[s] = uint32(index)
		w.buf2.PutUvarintStr(s)
	}

	w.buf1.PutBE32int(w.buf2.Len())
	w.buf2.PutHash(w.crc32)

	err := w.write(w.buf1.Get(), w.buf2.Get())
	return errors.Wrap(err, "write symbols")
}

func (w *Writer) WriteLabelIndex(names []string, values []string) error {
	if len(values)%len(names) != 0 {
		return errors.Errorf("invalid value list length %d for %d names", len(values), len(names))
	}
	if err := w.ensureStage(idxStageLabelIndex); err != nil {
		return errors.Wrap(err, "ensure stage")
	}

	valt, err := NewStringTuples(values, len(names))
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

	w.buf2.Reset()
	w.buf2.PutBE32int(len(names))
	w.buf2.PutBE32int(valt.Len())

	// here we have an index for the symbol file if v2, otherwise it's an offset
	for _, v := range valt.entries {
		index, ok := w.symbols[v]
		if !ok {
			return errors.Errorf("symbol entry for %q does not exist", v)
		}
		w.buf2.PutBE32(index)
	}

	w.buf1.Reset()
	w.buf1.PutBE32int(w.buf2.Len())

	w.buf2.PutHash(w.crc32)

	err = w.write(w.buf1.Get(), w.buf2.Get())
	return errors.Wrap(err, "write label index")
}

// writeOffsetTable writes a sequence of readable hash entries.
func (w *Writer) writeOffsetTable(entries []hashEntry) error {
	w.buf2.Reset()
	w.buf2.PutBE32int(len(entries))

	for _, e := range entries {
		w.buf2.PutUvarint(len(e.keys))
		for _, k := range e.keys {
			w.buf2.PutUvarintStr(k)
		}
		w.buf2.PutUvarint64(e.offset)
	}

	w.buf1.Reset()
	w.buf1.PutBE32int(w.buf2.Len())
	w.buf2.PutHash(w.crc32)

	return w.write(w.buf1.Get(), w.buf2.Get())
}

const indexTOCLen = 6*8 + 4

func (w *Writer) writeTOC() error {
	w.buf1.Reset()

	w.buf1.PutBE64(w.toc.Symbols)
	w.buf1.PutBE64(w.toc.Series)
	w.buf1.PutBE64(w.toc.LabelIndices)
	w.buf1.PutBE64(w.toc.LabelIndicesTable)
	w.buf1.PutBE64(w.toc.Postings)
	w.buf1.PutBE64(w.toc.PostingsTable)

	w.buf1.PutHash(w.crc32)

	return w.write(w.buf1.Get())
}

func (w *Writer) WritePostings(name, value string, it Postings) error {
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

	w.buf2.Reset()
	w.buf2.PutBE32int(len(refs))

	for _, r := range refs {
		w.buf2.PutBE32(r)
	}
	w.uint32s = refs

	w.buf1.Reset()
	w.buf1.PutBE32int(w.buf2.Len())

	w.buf2.PutHash(w.crc32)

	err := w.write(w.buf1.Get(), w.buf2.Get())
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

func (w *Writer) Close() error {
	if err := w.ensureStage(idxStageDone); err != nil {
		return err
	}
	if err := w.fbuf.Flush(); err != nil {
		return err
	}
	if err := w.f.Sync(); err != nil {
		return err
	}
	return w.f.Close()
}

// StringTuples provides access to a sorted list of string tuples.
type StringTuples interface {
	// Total number of tuples in the list.
	Len() int
	// At returns the tuple at position i.
	At(i int) ([]string, error)
}

type Reader struct {
	b ByteSlice

	// Close that releases the underlying resources of the byte slice.
	c io.Closer

	// Cached hashmaps of section offsets.
	labels map[string]uint64
	// LabelName to LabelValue to offset map.
	postings map[string]map[string]uint64
	// Cache of read symbols. Strings that are returned when reading from the
	// block are always backed by true strings held in here rather than
	// strings that are backed by byte slices from the mmap'd index file. This
	// prevents memory faults when applications work with read symbols after
	// the block has been unmapped. The older format has sparse indexes so a map
	// must be used, but the new format is not so we can use a slice.
	symbolsV1        map[uint32]string
	symbolsV2        []string
	symbolsTableSize uint64

	dec *Decoder

	version int
}

// ByteSlice abstracts a byte slice.
type ByteSlice interface {
	Len() int
	Range(start, end int) []byte
}

type realByteSlice []byte

func (b realByteSlice) Len() int {
	return len(b)
}

func (b realByteSlice) Range(start, end int) []byte {
	return b[start:end]
}

func (b realByteSlice) Sub(start, end int) ByteSlice {
	return b[start:end]
}

// NewReader returns a new index reader on the given byte slice. It automatically
// handles different format versions.
func NewReader(b ByteSlice) (*Reader, error) {
	return newReader(b, ioutil.NopCloser(nil))
}

// NewFileReader returns a new index reader against the given index file.
func NewFileReader(path string) (*Reader, error) {
	f, err := fileutil.OpenMmapFile(path)
	if err != nil {
		return nil, err
	}
	r, err := newReader(realByteSlice(f.Bytes()), f)
	if err != nil {
		var merr tsdb_errors.MultiError
		merr.Add(err)
		merr.Add(f.Close())
		return nil, merr
	}

	return r, nil
}

func newReader(b ByteSlice, c io.Closer) (*Reader, error) {
	r := &Reader{
		b:        b,
		c:        c,
		labels:   map[string]uint64{},
		postings: map[string]map[string]uint64{},
	}

	// Verify header.
	if r.b.Len() < HeaderLen {
		return nil, errors.Wrap(encoding.ErrInvalidSize, "index header")
	}
	if m := binary.BigEndian.Uint32(r.b.Range(0, 4)); m != MagicIndex {
		return nil, errors.Errorf("invalid magic number %x", m)
	}
	r.version = int(r.b.Range(4, 5)[0])

	if r.version != FormatV1 && r.version != FormatV2 {
		return nil, errors.Errorf("unknown index file version %d", r.version)
	}

	toc, err := NewTOCFromByteSlice(b)
	if err != nil {
		return nil, errors.Wrap(err, "read TOC")
	}

	r.symbolsV2, r.symbolsV1, err = ReadSymbols(r.b, r.version, int(toc.Symbols))
	if err != nil {
		return nil, errors.Wrap(err, "read symbols")
	}

	// Use the strings already allocated by symbols, rather than
	// re-allocating them again below.
	// Additionally, calculate symbolsTableSize.
	allocatedSymbols := make(map[string]string, len(r.symbolsV1)+len(r.symbolsV2))
	for _, s := range r.symbolsV1 {
		r.symbolsTableSize += uint64(len(s) + 8)
		allocatedSymbols[s] = s
	}
	for _, s := range r.symbolsV2 {
		r.symbolsTableSize += uint64(len(s) + 8)
		allocatedSymbols[s] = s
	}

	if err := ReadOffsetTable(r.b, toc.LabelIndicesTable, func(key []string, off uint64) error {
		if len(key) != 1 {
			return errors.Errorf("unexpected key length for label indices table %d", len(key))
		}

		r.labels[allocatedSymbols[key[0]]] = off
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "read label index table")
	}

	r.postings[""] = map[string]uint64{}
	if err := ReadOffsetTable(r.b, toc.PostingsTable, func(key []string, off uint64) error {
		if len(key) != 2 {
			return errors.Errorf("unexpected key length for posting table %d", len(key))
		}
		if _, ok := r.postings[key[0]]; !ok {
			r.postings[allocatedSymbols[key[0]]] = map[string]uint64{}
		}
		r.postings[key[0]][allocatedSymbols[key[1]]] = off
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "read postings table")
	}

	r.dec = &Decoder{LookupSymbol: r.lookupSymbol}

	return r, nil
}

// Version returns the file format version of the underlying index.
func (r *Reader) Version() int {
	return r.version
}

// Range marks a byte range.
type Range struct {
	Start, End int64
}

// PostingsRanges returns a new map of byte range in the underlying index file
// for all postings lists.
func (r *Reader) PostingsRanges() (map[labels.Label]Range, error) {
	m := map[labels.Label]Range{}

	for k, e := range r.postings {
		for v, start := range e {
			d := encoding.NewDecbufAt(r.b, int(start), castagnoliTable)
			if d.Err() != nil {
				return nil, d.Err()
			}
			m[labels.Label{Name: k, Value: v}] = Range{
				Start: int64(start) + 4,
				End:   int64(start) + 4 + int64(d.Len()),
			}
		}
	}
	return m, nil
}

// ReadSymbols reads the symbol table fully into memory and allocates proper strings for them.
// Strings backed by the mmap'd memory would cause memory faults if applications keep using them
// after the reader is closed.
func ReadSymbols(bs ByteSlice, version int, off int) ([]string, map[uint32]string, error) {
	if off == 0 {
		return nil, nil, nil
	}
	d := encoding.NewDecbufAt(bs, off, castagnoliTable)

	var (
		origLen     = d.Len()
		cnt         = d.Be32int()
		basePos     = uint32(off) + 4
		nextPos     = basePos + uint32(origLen-d.Len())
		symbolSlice []string
		symbols     = map[uint32]string{}
	)
	if version == FormatV2 {
		symbolSlice = make([]string, 0, cnt)
	}

	for d.Err() == nil && d.Len() > 0 && cnt > 0 {
		s := d.UvarintStr()

		if version == FormatV2 {
			symbolSlice = append(symbolSlice, s)
		} else {
			symbols[nextPos] = s
			nextPos = basePos + uint32(origLen-d.Len())
		}
		cnt--
	}
	return symbolSlice, symbols, errors.Wrap(d.Err(), "read symbols")
}

// ReadOffsetTable reads an offset table and at the given position calls f for each
// found entry. If f returns an error it stops decoding and returns the received error.
func ReadOffsetTable(bs ByteSlice, off uint64, f func([]string, uint64) error) error {
	d := encoding.NewDecbufAt(bs, int(off), castagnoliTable)
	cnt := d.Be32()

	for d.Err() == nil && d.Len() > 0 && cnt > 0 {
		keyCount := d.Uvarint()
		keys := make([]string, 0, keyCount)

		for i := 0; i < keyCount; i++ {
			keys = append(keys, d.UvarintStr())
		}
		o := d.Uvarint64()
		if d.Err() != nil {
			break
		}
		if err := f(keys, o); err != nil {
			return err
		}
		cnt--
	}
	return d.Err()
}

// Close the reader and its underlying resources.
func (r *Reader) Close() error {
	return r.c.Close()
}

func (r *Reader) lookupSymbol(o uint32) (string, error) {
	if int(o) < len(r.symbolsV2) {
		return r.symbolsV2[o], nil
	}
	s, ok := r.symbolsV1[o]
	if !ok {
		return "", errors.Errorf("unknown symbol offset %d", o)
	}
	return s, nil
}

// Symbols returns a set of symbols that exist within the index.
func (r *Reader) Symbols() (map[string]struct{}, error) {
	res := make(map[string]struct{}, len(r.symbolsV1)+len(r.symbolsV2))

	for _, s := range r.symbolsV1 {
		res[s] = struct{}{}
	}
	for _, s := range r.symbolsV2 {
		res[s] = struct{}{}
	}
	return res, nil
}

// SymbolTableSize returns the symbol table size in bytes.
func (r *Reader) SymbolTableSize() uint64 {
	return r.symbolsTableSize
}

// LabelValues returns value tuples that exist for the given label name tuples.
func (r *Reader) LabelValues(names ...string) (StringTuples, error) {

	key := strings.Join(names, labelNameSeperator)
	off, ok := r.labels[key]
	if !ok {
		// XXX(fabxc): hot fix. Should return a partial data error and handle cases
		// where the entire block has no data gracefully.
		return emptyStringTuples{}, nil
		//return nil, fmt.Errorf("label index doesn't exist")
	}

	d := encoding.NewDecbufAt(r.b, int(off), castagnoliTable)

	nc := d.Be32int()
	d.Be32() // consume unused value entry count.

	if d.Err() != nil {
		return nil, errors.Wrap(d.Err(), "read label value index")
	}
	st := &serializedStringTuples{
		idsCount: nc,
		idsBytes: d.Get(),
		lookup:   r.lookupSymbol,
	}
	return st, nil
}

type emptyStringTuples struct{}

func (emptyStringTuples) At(i int) ([]string, error) { return nil, nil }
func (emptyStringTuples) Len() int                   { return 0 }

// LabelIndices returns a slice of label names for which labels or label tuples value indices exist.
// NOTE: This is deprecated. Use `LabelNames()` instead.
func (r *Reader) LabelIndices() ([][]string, error) {
	var res [][]string
	for s := range r.labels {
		res = append(res, strings.Split(s, labelNameSeperator))
	}
	return res, nil
}

// Series reads the series with the given ID and writes its labels and chunks into lbls and chks.
func (r *Reader) Series(id uint64, lbls *labels.Labels, chks *[]chunks.Meta) error {
	offset := id
	// In version 2 series IDs are no longer exact references but series are 16-byte padded
	// and the ID is the multiple of 16 of the actual position.
	if r.version == FormatV2 {
		offset = id * 16
	}
	d := encoding.NewDecbufUvarintAt(r.b, int(offset), castagnoliTable)
	if d.Err() != nil {
		return d.Err()
	}
	return errors.Wrap(r.dec.Series(d.Get(), lbls, chks), "read series")
}

// Postings returns a postings list for the given label pair.
func (r *Reader) Postings(name, value string) (Postings, error) {
	e, ok := r.postings[name]
	if !ok {
		return EmptyPostings(), nil
	}
	off, ok := e[value]
	if !ok {
		return EmptyPostings(), nil
	}
	d := encoding.NewDecbufAt(r.b, int(off), castagnoliTable)
	if d.Err() != nil {
		return nil, errors.Wrap(d.Err(), "get postings entry")
	}
	_, p, err := r.dec.Postings(d.Get())
	if err != nil {
		return nil, errors.Wrap(err, "decode postings")
	}
	return p, nil
}

// SortedPostings returns the given postings list reordered so that the backing series
// are sorted.
func (r *Reader) SortedPostings(p Postings) Postings {
	return p
}

// Size returns the size of an index file.
func (r *Reader) Size() int64 {
	return int64(r.b.Len())
}

// LabelNames returns all the unique label names present in the index.
func (r *Reader) LabelNames() ([]string, error) {
	labelNamesMap := make(map[string]struct{}, len(r.labels))
	for key := range r.labels {
		// 'key' contains the label names concatenated with the
		// delimiter 'labelNameSeperator'.
		names := strings.Split(key, labelNameSeperator)
		for _, name := range names {
			if name == allPostingsKey.Name {
				// This is not from any metric.
				// It is basically an empty label name.
				continue
			}
			labelNamesMap[name] = struct{}{}
		}
	}
	labelNames := make([]string, 0, len(labelNamesMap))
	for name := range labelNamesMap {
		labelNames = append(labelNames, name)
	}
	sort.Strings(labelNames)
	return labelNames, nil
}

type stringTuples struct {
	length  int      // tuple length
	entries []string // flattened tuple entries
}

func NewStringTuples(entries []string, length int) (*stringTuples, error) {
	if len(entries)%length != 0 {
		return nil, errors.Wrap(encoding.ErrInvalidSize, "string tuple list")
	}
	return &stringTuples{entries: entries, length: length}, nil
}

func (t *stringTuples) Len() int                   { return len(t.entries) / t.length }
func (t *stringTuples) At(i int) ([]string, error) { return t.entries[i : i+t.length], nil }

func (t *stringTuples) Swap(i, j int) {
	c := make([]string, t.length)
	copy(c, t.entries[i:i+t.length])

	for k := 0; k < t.length; k++ {
		t.entries[i+k] = t.entries[j+k]
		t.entries[j+k] = c[k]
	}
}

func (t *stringTuples) Less(i, j int) bool {
	for k := 0; k < t.length; k++ {
		d := strings.Compare(t.entries[i+k], t.entries[j+k])

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
	idsCount int
	idsBytes []byte // bytes containing the ids pointing to the string in the lookup table.
	lookup   func(uint32) (string, error)
}

func (t *serializedStringTuples) Len() int {
	return len(t.idsBytes) / (4 * t.idsCount)
}

func (t *serializedStringTuples) At(i int) ([]string, error) {
	if len(t.idsBytes) < (i+t.idsCount)*4 {
		return nil, encoding.ErrInvalidSize
	}
	res := make([]string, 0, t.idsCount)

	for k := 0; k < t.idsCount; k++ {
		offset := binary.BigEndian.Uint32(t.idsBytes[(i+k)*4:])

		s, err := t.lookup(offset)
		if err != nil {
			return nil, errors.Wrap(err, "symbol lookup")
		}
		res = append(res, s)
	}

	return res, nil
}

// Decoder provides decoding methods for the v1 and v2 index file format.
//
// It currently does not contain decoding methods for all entry types but can be extended
// by them if there's demand.
type Decoder struct {
	LookupSymbol func(uint32) (string, error)
}

// Postings returns a postings list for b and its number of elements.
func (dec *Decoder) Postings(b []byte) (int, Postings, error) {
	d := encoding.Decbuf{B: b}
	n := d.Be32int()
	l := d.Get()
	return n, newBigEndianPostings(l), d.Err()
}

// Series decodes a series entry from the given byte slice into lset and chks.
func (dec *Decoder) Series(b []byte, lbls *labels.Labels, chks *[]chunks.Meta) error {
	*lbls = (*lbls)[:0]
	*chks = (*chks)[:0]

	d := encoding.Decbuf{B: b}

	k := d.Uvarint()

	for i := 0; i < k; i++ {
		lno := uint32(d.Uvarint())
		lvo := uint32(d.Uvarint())

		if d.Err() != nil {
			return errors.Wrap(d.Err(), "read series label offsets")
		}

		ln, err := dec.LookupSymbol(lno)
		if err != nil {
			return errors.Wrap(err, "lookup label name")
		}
		lv, err := dec.LookupSymbol(lvo)
		if err != nil {
			return errors.Wrap(err, "lookup label value")
		}

		*lbls = append(*lbls, labels.Label{Name: ln, Value: lv})
	}

	// Read the chunks meta data.
	k = d.Uvarint()

	if k == 0 {
		return nil
	}

	t0 := d.Varint64()
	maxt := int64(d.Uvarint64()) + t0
	ref0 := int64(d.Uvarint64())

	*chks = append(*chks, chunks.Meta{
		Ref:     uint64(ref0),
		MinTime: t0,
		MaxTime: maxt,
	})
	t0 = maxt

	for i := 1; i < k; i++ {
		mint := int64(d.Uvarint64()) + t0
		maxt := int64(d.Uvarint64()) + mint

		ref0 += d.Varint64()
		t0 = maxt

		if d.Err() != nil {
			return errors.Wrapf(d.Err(), "read meta for chunk %d", i)
		}

		*chks = append(*chks, chunks.Meta{
			Ref:     uint64(ref0),
			MinTime: mint,
			MaxTime: maxt,
		})
	}
	return d.Err()
}
