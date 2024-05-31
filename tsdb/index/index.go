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
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"unsafe"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
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

	indexFilename = "index"

	seriesByteAlign = 16

	// checkContextEveryNIterations is used in some tight loops to check if the context is done.
	checkContextEveryNIterations = 128
)

type indexWriterSeries struct {
	labels labels.Labels
	chunks []chunks.Meta // series file offset of chunks
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

type symbolCacheEntry struct {
	index          uint32
	lastValueIndex uint32
	lastValue      string
}

type PostingsEncoder func(*encoding.Encbuf, []uint32) error

// Writer implements the IndexWriter interface for the standard
// serialization format.
type Writer struct {
	ctx context.Context

	// For the main index file.
	f *FileWriter

	// Temporary file for postings.
	fP *FileWriter
	// Temporary file for posting offsets table.
	fPO   *FileWriter
	cntPO uint64

	toc           TOC
	stage         indexWriterStage
	postingsStart uint64 // Due to padding, can differ from TOC entry.

	// Reusable memory.
	buf1 encoding.Encbuf
	buf2 encoding.Encbuf

	numSymbols  int
	symbols     *Symbols
	symbolFile  *fileutil.MmapFile
	lastSymbol  string
	symbolCache map[string]symbolCacheEntry

	labelIndexes []labelIndexHashEntry // Label index offsets.
	labelNames   map[string]uint64     // Label names, and their usage.

	// Hold last series to validate that clients insert new series in order.
	lastSeries    labels.Labels
	lastSeriesRef storage.SeriesRef

	// Hold last added chunk reference to make sure that chunks are ordered properly.
	lastChunkRef chunks.ChunkRef

	crc32 hash.Hash

	Version int

	postingsEncoder PostingsEncoder
}

// TOC represents the index Table Of Contents that states where each section of the index starts.
type TOC struct {
	Symbols           uint64
	Series            uint64
	LabelIndices      uint64
	LabelIndicesTable uint64
	Postings          uint64
	PostingsTable     uint64
}

// NewTOCFromByteSlice returns a parsed TOC from the given index byte slice.
func NewTOCFromByteSlice(bs ByteSlice) (*TOC, error) {
	if bs.Len() < indexTOCLen {
		return nil, encoding.ErrInvalidSize
	}
	b := bs.Range(bs.Len()-indexTOCLen, bs.Len())

	expCRC := binary.BigEndian.Uint32(b[len(b)-4:])
	d := encoding.Decbuf{B: b[:len(b)-4]}

	if d.Crc32(castagnoliTable) != expCRC {
		return nil, fmt.Errorf("read TOC: %w", encoding.ErrInvalidChecksum)
	}

	toc := &TOC{
		Symbols:           d.Be64(),
		Series:            d.Be64(),
		LabelIndices:      d.Be64(),
		LabelIndicesTable: d.Be64(),
		Postings:          d.Be64(),
		PostingsTable:     d.Be64(),
	}
	return toc, d.Err()
}

// NewWriter returns a new Writer to the given filename. It serializes data in format version 2.
// It uses the given encoder to encode each postings list.
func NewWriterWithEncoder(ctx context.Context, fn string, encoder PostingsEncoder) (*Writer, error) {
	dir := filepath.Dir(fn)

	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}
	defer df.Close() // Close for platform windows.

	if err := os.RemoveAll(fn); err != nil {
		return nil, fmt.Errorf("remove any existing index at path: %w", err)
	}

	// Main index file we are building.
	f, err := NewFileWriter(fn)
	if err != nil {
		return nil, err
	}
	// Temporary file for postings.
	fP, err := NewFileWriter(fn + "_tmp_p")
	if err != nil {
		return nil, err
	}
	// Temporary file for posting offset table.
	fPO, err := NewFileWriter(fn + "_tmp_po")
	if err != nil {
		return nil, err
	}
	if err := df.Sync(); err != nil {
		return nil, fmt.Errorf("sync dir: %w", err)
	}

	iw := &Writer{
		ctx:   ctx,
		f:     f,
		fP:    fP,
		fPO:   fPO,
		stage: idxStageNone,

		// Reusable memory.
		buf1: encoding.Encbuf{B: make([]byte, 0, 1<<22)},
		buf2: encoding.Encbuf{B: make([]byte, 0, 1<<22)},

		symbolCache:     make(map[string]symbolCacheEntry, 1<<8),
		labelNames:      make(map[string]uint64, 1<<8),
		crc32:           newCRC32(),
		postingsEncoder: encoder,
	}
	if err := iw.writeMeta(); err != nil {
		return nil, err
	}
	return iw, nil
}

// NewWriter creates a new index writer using the default encoder. See
// NewWriterWithEncoder.
func NewWriter(ctx context.Context, fn string) (*Writer, error) {
	return NewWriterWithEncoder(ctx, fn, EncodePostingsRaw)
}

func (w *Writer) write(bufs ...[]byte) error {
	return w.f.Write(bufs...)
}

func (w *Writer) writeAt(buf []byte, pos uint64) error {
	return w.f.WriteAt(buf, pos)
}

func (w *Writer) addPadding(size int) error {
	return w.f.AddPadding(size)
}

type FileWriter struct {
	f    *os.File
	fbuf *bufio.Writer
	pos  uint64
	name string
}

func NewFileWriter(name string) (*FileWriter, error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return nil, err
	}
	return &FileWriter{
		f:    f,
		fbuf: bufio.NewWriterSize(f, 1<<22),
		pos:  0,
		name: name,
	}, nil
}

func (fw *FileWriter) Pos() uint64 {
	return fw.pos
}

func (fw *FileWriter) Write(bufs ...[]byte) error {
	for _, b := range bufs {
		n, err := fw.fbuf.Write(b)
		fw.pos += uint64(n)
		if err != nil {
			return err
		}
		// For now the index file must not grow beyond 64GiB. Some of the fixed-sized
		// offset references in v1 are only 4 bytes large.
		// Once we move to compressed/varint representations in those areas, this limitation
		// can be lifted.
		if fw.pos > 16*math.MaxUint32 {
			return fmt.Errorf("%q exceeding max size of 64GiB", fw.name)
		}
	}
	return nil
}

func (fw *FileWriter) Flush() error {
	return fw.fbuf.Flush()
}

func (fw *FileWriter) WriteAt(buf []byte, pos uint64) error {
	if err := fw.Flush(); err != nil {
		return err
	}
	_, err := fw.f.WriteAt(buf, int64(pos))
	return err
}

// AddPadding adds zero byte padding until the file size is a multiple size.
func (fw *FileWriter) AddPadding(size int) error {
	p := fw.pos % uint64(size)
	if p == 0 {
		return nil
	}
	p = uint64(size) - p

	if err := fw.Write(make([]byte, p)); err != nil {
		return fmt.Errorf("add padding: %w", err)
	}
	return nil
}

func (fw *FileWriter) Close() error {
	if err := fw.Flush(); err != nil {
		return err
	}
	if err := fw.f.Sync(); err != nil {
		return err
	}
	return fw.f.Close()
}

func (fw *FileWriter) Remove() error {
	return os.Remove(fw.name)
}

// ensureStage handles transitions between write stages and ensures that IndexWriter
// methods are called in an order valid for the implementation.
func (w *Writer) ensureStage(s indexWriterStage) error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	default:
	}

	if w.stage == s {
		return nil
	}
	if w.stage < s-1 {
		// A stage has been skipped.
		if err := w.ensureStage(s - 1); err != nil {
			return err
		}
	}
	if w.stage > s {
		return fmt.Errorf("invalid stage %q, currently at %q", s, w.stage)
	}

	// Mark start of sections in table of contents.
	switch s {
	case idxStageSymbols:
		w.toc.Symbols = w.f.pos
		if err := w.startSymbols(); err != nil {
			return err
		}
	case idxStageSeries:
		if err := w.finishSymbols(); err != nil {
			return err
		}
		w.toc.Series = w.f.pos

	case idxStageDone:
		w.toc.LabelIndices = w.f.pos
		// LabelIndices generation depends on the posting offset
		// table produced at this stage.
		if err := w.writePostingsToTmpFiles(); err != nil {
			return err
		}
		if err := w.writeLabelIndices(); err != nil {
			return err
		}

		w.toc.Postings = w.f.pos
		if err := w.writePostings(); err != nil {
			return err
		}

		w.toc.LabelIndicesTable = w.f.pos
		if err := w.writeLabelIndexesOffsetTable(); err != nil {
			return err
		}

		w.toc.PostingsTable = w.f.pos
		if err := w.writePostingsOffsetTable(); err != nil {
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
func (w *Writer) AddSeries(ref storage.SeriesRef, lset labels.Labels, chunks ...chunks.Meta) error {
	if err := w.ensureStage(idxStageSeries); err != nil {
		return err
	}
	if labels.Compare(lset, w.lastSeries) <= 0 {
		return fmt.Errorf("out-of-order series added with label set %q", lset)
	}

	if ref < w.lastSeriesRef && !w.lastSeries.IsEmpty() {
		return fmt.Errorf("series with reference greater than %d already added", ref)
	}

	lastChunkRef := w.lastChunkRef
	lastMaxT := int64(0)
	for ix, c := range chunks {
		if c.Ref < lastChunkRef {
			return fmt.Errorf("unsorted chunk reference: %d, previous: %d", c.Ref, lastChunkRef)
		}
		lastChunkRef = c.Ref

		if ix > 0 && c.MinTime <= lastMaxT {
			return fmt.Errorf("chunk minT %d is not higher than previous chunk maxT %d", c.MinTime, lastMaxT)
		}
		if c.MaxTime < c.MinTime {
			return fmt.Errorf("chunk maxT %d is less than minT %d", c.MaxTime, c.MinTime)
		}
		lastMaxT = c.MaxTime
	}

	// We add padding to 16 bytes to increase the addressable space we get through 4 byte
	// series references.
	if err := w.addPadding(seriesByteAlign); err != nil {
		return fmt.Errorf("failed to write padding bytes: %w", err)
	}

	if w.f.pos%seriesByteAlign != 0 {
		return fmt.Errorf("series write not 16-byte aligned at %d", w.f.pos)
	}

	w.buf2.Reset()
	w.buf2.PutUvarint(lset.Len())

	if err := lset.Validate(func(l labels.Label) error {
		var err error
		cacheEntry, ok := w.symbolCache[l.Name]
		nameIndex := cacheEntry.index
		if !ok {
			nameIndex, err = w.symbols.ReverseLookup(l.Name)
			if err != nil {
				return fmt.Errorf("symbol entry for %q does not exist, %w", l.Name, err)
			}
		}
		w.labelNames[l.Name]++
		w.buf2.PutUvarint32(nameIndex)

		valueIndex := cacheEntry.lastValueIndex
		if !ok || cacheEntry.lastValue != l.Value {
			valueIndex, err = w.symbols.ReverseLookup(l.Value)
			if err != nil {
				return fmt.Errorf("symbol entry for %q does not exist, %w", l.Value, err)
			}
			w.symbolCache[l.Name] = symbolCacheEntry{
				index:          nameIndex,
				lastValueIndex: valueIndex,
				lastValue:      l.Value,
			}
		}
		w.buf2.PutUvarint32(valueIndex)
		return nil
	}); err != nil {
		return err
	}

	w.buf2.PutUvarint(len(chunks))

	if len(chunks) > 0 {
		c := chunks[0]
		w.buf2.PutVarint64(c.MinTime)
		w.buf2.PutUvarint64(uint64(c.MaxTime - c.MinTime))
		w.buf2.PutUvarint64(uint64(c.Ref))
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
		return fmt.Errorf("write series data: %w", err)
	}

	w.lastSeries.CopyFrom(lset)
	w.lastSeriesRef = ref
	w.lastChunkRef = lastChunkRef

	return nil
}

func (w *Writer) startSymbols() error {
	// We are at w.toc.Symbols.
	// Leave 4 bytes of space for the length, and another 4 for the number of symbols
	// which will both be calculated later.
	return w.write([]byte("alenblen"))
}

func (w *Writer) AddSymbol(sym string) error {
	if err := w.ensureStage(idxStageSymbols); err != nil {
		return err
	}
	if w.numSymbols != 0 && sym <= w.lastSymbol {
		return fmt.Errorf("symbol %q out-of-order", sym)
	}
	w.lastSymbol = sym
	w.numSymbols++
	w.buf1.Reset()
	w.buf1.PutUvarintStr(sym)
	return w.write(w.buf1.Get())
}

func (w *Writer) finishSymbols() error {
	symbolTableSize := w.f.pos - w.toc.Symbols - 4
	// The symbol table's <len> part is 4 bytes. So the total symbol table size must be less than or equal to 2^32-1
	if symbolTableSize > math.MaxUint32 {
		return fmt.Errorf("symbol table size exceeds %d bytes: %d", uint32(math.MaxUint32), symbolTableSize)
	}

	// Write out the length and symbol count.
	w.buf1.Reset()
	w.buf1.PutBE32int(int(symbolTableSize))
	w.buf1.PutBE32int(w.numSymbols)
	if err := w.writeAt(w.buf1.Get(), w.toc.Symbols); err != nil {
		return err
	}

	hashPos := w.f.pos
	// Leave space for the hash. We can only calculate it
	// now that the number of symbols is known, so mmap and do it from there.
	if err := w.write([]byte("hash")); err != nil {
		return err
	}
	if err := w.f.Flush(); err != nil {
		return err
	}

	sf, err := fileutil.OpenMmapFile(w.f.name)
	if err != nil {
		return err
	}
	w.symbolFile = sf
	hash := crc32.Checksum(w.symbolFile.Bytes()[w.toc.Symbols+4:hashPos], castagnoliTable)
	w.buf1.Reset()
	w.buf1.PutBE32(hash)
	if err := w.writeAt(w.buf1.Get(), hashPos); err != nil {
		return err
	}

	// Load in the symbol table efficiently for the rest of the index writing.
	w.symbols, err = NewSymbols(realByteSlice(w.symbolFile.Bytes()), FormatV2, int(w.toc.Symbols))
	if err != nil {
		return fmt.Errorf("read symbols: %w", err)
	}
	return nil
}

func (w *Writer) writeLabelIndices() error {
	if err := w.fPO.Flush(); err != nil {
		return err
	}

	// Find all the label values in the tmp posting offset table.
	f, err := fileutil.OpenMmapFile(w.fPO.name)
	if err != nil {
		return err
	}
	defer f.Close()

	d := encoding.NewDecbufRaw(realByteSlice(f.Bytes()), int(w.fPO.pos))
	cnt := w.cntPO
	current := []byte{}
	values := []uint32{}
	for d.Err() == nil && cnt > 0 {
		cnt--
		d.Uvarint()                           // Keycount.
		name := d.UvarintBytes()              // Label name.
		value := yoloString(d.UvarintBytes()) // Label value.
		d.Uvarint64()                         // Offset.
		if len(name) == 0 {
			continue // All index is ignored.
		}

		if !bytes.Equal(name, current) && len(values) > 0 {
			// We've reached a new label name.
			if err := w.writeLabelIndex(string(current), values); err != nil {
				return err
			}
			values = values[:0]
		}
		current = name
		sid, err := w.symbols.ReverseLookup(value)
		if err != nil {
			return err
		}
		values = append(values, sid)
	}
	if d.Err() != nil {
		return d.Err()
	}

	// Handle the last label.
	if len(values) > 0 {
		if err := w.writeLabelIndex(string(current), values); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) writeLabelIndex(name string, values []uint32) error {
	// Align beginning to 4 bytes for more efficient index list scans.
	if err := w.addPadding(4); err != nil {
		return err
	}

	w.labelIndexes = append(w.labelIndexes, labelIndexHashEntry{
		keys:   []string{name},
		offset: w.f.pos,
	})

	startPos := w.f.pos
	// Leave 4 bytes of space for the length, which will be calculated later.
	if err := w.write([]byte("alen")); err != nil {
		return err
	}
	w.crc32.Reset()

	w.buf1.Reset()
	w.buf1.PutBE32int(1) // Number of names.
	w.buf1.PutBE32int(len(values))
	w.buf1.WriteToHash(w.crc32)
	if err := w.write(w.buf1.Get()); err != nil {
		return err
	}

	for _, v := range values {
		w.buf1.Reset()
		w.buf1.PutBE32(v)
		w.buf1.WriteToHash(w.crc32)
		if err := w.write(w.buf1.Get()); err != nil {
			return err
		}
	}

	// Write out the length.
	w.buf1.Reset()
	l := w.f.pos - startPos - 4
	if l > math.MaxUint32 {
		return fmt.Errorf("label index size exceeds 4 bytes: %d", l)
	}
	w.buf1.PutBE32int(int(l))
	if err := w.writeAt(w.buf1.Get(), startPos); err != nil {
		return err
	}

	w.buf1.Reset()
	w.buf1.PutHashSum(w.crc32)
	return w.write(w.buf1.Get())
}

// writeLabelIndexesOffsetTable writes the label indices offset table.
func (w *Writer) writeLabelIndexesOffsetTable() error {
	startPos := w.f.pos
	// Leave 4 bytes of space for the length, which will be calculated later.
	if err := w.write([]byte("alen")); err != nil {
		return err
	}
	w.crc32.Reset()

	w.buf1.Reset()
	w.buf1.PutBE32int(len(w.labelIndexes))
	w.buf1.WriteToHash(w.crc32)
	if err := w.write(w.buf1.Get()); err != nil {
		return err
	}

	for _, e := range w.labelIndexes {
		w.buf1.Reset()
		w.buf1.PutUvarint(len(e.keys))
		for _, k := range e.keys {
			w.buf1.PutUvarintStr(k)
		}
		w.buf1.PutUvarint64(e.offset)
		w.buf1.WriteToHash(w.crc32)
		if err := w.write(w.buf1.Get()); err != nil {
			return err
		}
	}

	// Write out the length.
	err := w.writeLengthAndHash(startPos)
	if err != nil {
		return fmt.Errorf("label indexes offset table length/crc32 write error: %w", err)
	}
	return nil
}

// writePostingsOffsetTable writes the postings offset table.
func (w *Writer) writePostingsOffsetTable() error {
	// Ensure everything is in the temporary file.
	if err := w.fPO.Flush(); err != nil {
		return err
	}

	startPos := w.f.pos
	// Leave 4 bytes of space for the length, which will be calculated later.
	if err := w.write([]byte("alen")); err != nil {
		return err
	}

	// Copy over the tmp posting offset table, however we need to
	// adjust the offsets.
	adjustment := w.postingsStart

	w.buf1.Reset()
	w.crc32.Reset()
	w.buf1.PutBE32int(int(w.cntPO)) // Count.
	w.buf1.WriteToHash(w.crc32)
	if err := w.write(w.buf1.Get()); err != nil {
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
	d := encoding.NewDecbufRaw(realByteSlice(f.Bytes()), int(w.fPO.pos))
	cnt := w.cntPO
	for d.Err() == nil && cnt > 0 {
		w.buf1.Reset()
		w.buf1.PutUvarint(d.Uvarint())                     // Keycount.
		w.buf1.PutUvarintStr(yoloString(d.UvarintBytes())) // Label name.
		w.buf1.PutUvarintStr(yoloString(d.UvarintBytes())) // Label value.
		w.buf1.PutUvarint64(d.Uvarint64() + adjustment)    // Offset.
		w.buf1.WriteToHash(w.crc32)
		if err := w.write(w.buf1.Get()); err != nil {
			return err
		}
		cnt--
	}
	if d.Err() != nil {
		return d.Err()
	}

	// Cleanup temporary file.
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

	err = w.writeLengthAndHash(startPos)
	if err != nil {
		return fmt.Errorf("postings offset table length/crc32 write error: %w", err)
	}
	return nil
}

func (w *Writer) writeLengthAndHash(startPos uint64) error {
	w.buf1.Reset()
	l := w.f.pos - startPos - 4
	if l > math.MaxUint32 {
		return fmt.Errorf("length size exceeds 4 bytes: %d", l)
	}
	w.buf1.PutBE32int(int(l))
	if err := w.writeAt(w.buf1.Get(), startPos); err != nil {
		return fmt.Errorf("write length from buffer error: %w", err)
	}

	// Write out the hash.
	w.buf1.Reset()
	w.buf1.PutHashSum(w.crc32)
	if err := w.write(w.buf1.Get()); err != nil {
		return fmt.Errorf("write buffer's crc32 error: %w", err)
	}
	return nil
}

const indexTOCLen = 6*8 + crc32.Size

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

func (w *Writer) writePostingsToTmpFiles() error {
	names := make([]string, 0, len(w.labelNames))
	for n := range w.labelNames {
		names = append(names, n)
	}
	slices.Sort(names)

	if err := w.f.Flush(); err != nil {
		return err
	}
	f, err := fileutil.OpenMmapFile(w.f.name)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write out the special all posting.
	offsets := []uint32{}
	d := encoding.NewDecbufRaw(realByteSlice(f.Bytes()), int(w.toc.LabelIndices))
	d.Skip(int(w.toc.Series))
	for d.Len() > 0 {
		d.ConsumePadding()
		startPos := w.toc.LabelIndices - uint64(d.Len())
		if startPos%seriesByteAlign != 0 {
			return fmt.Errorf("series not 16-byte aligned at %d", startPos)
		}
		offsets = append(offsets, uint32(startPos/seriesByteAlign))
		// Skip to next series.
		x := d.Uvarint()
		d.Skip(x + crc32.Size)
		if err := d.Err(); err != nil {
			return err
		}
	}
	if err := w.writePosting("", "", offsets); err != nil {
		return err
	}
	maxPostings := uint64(len(offsets)) // No label name can have more postings than this.

	for len(names) > 0 {
		batchNames := []string{}
		var c uint64
		// Try to bunch up label names into one loop, but avoid
		// using more memory than a single label name can.
		for len(names) > 0 {
			if w.labelNames[names[0]]+c > maxPostings {
				if c > 0 {
					break
				}
				return fmt.Errorf("corruption detected when writing postings to index: label %q has %d uses, but maxPostings is %d", names[0], w.labelNames[names[0]], maxPostings)
			}
			batchNames = append(batchNames, names[0])
			c += w.labelNames[names[0]]
			names = names[1:]
		}

		nameSymbols := map[uint32]string{}
		for _, name := range batchNames {
			sid, err := w.symbols.ReverseLookup(name)
			if err != nil {
				return err
			}
			nameSymbols[sid] = name
		}
		// Label name -> label value -> positions.
		postings := map[uint32]map[uint32][]uint32{}

		d := encoding.NewDecbufRaw(realByteSlice(f.Bytes()), int(w.toc.LabelIndices))
		d.Skip(int(w.toc.Series))
		for d.Len() > 0 {
			d.ConsumePadding()
			startPos := w.toc.LabelIndices - uint64(d.Len())
			l := d.Uvarint() // Length of this series in bytes.
			startLen := d.Len()

			// See if label names we want are in the series.
			numLabels := d.Uvarint()
			for i := 0; i < numLabels; i++ {
				lno := uint32(d.Uvarint())
				lvo := uint32(d.Uvarint())

				if _, ok := nameSymbols[lno]; ok {
					if _, ok := postings[lno]; !ok {
						postings[lno] = map[uint32][]uint32{}
					}
					postings[lno][lvo] = append(postings[lno][lvo], uint32(startPos/seriesByteAlign))
				}
			}
			// Skip to next series.
			d.Skip(l - (startLen - d.Len()) + crc32.Size)
			if err := d.Err(); err != nil {
				return err
			}
		}

		for _, name := range batchNames {
			// Write out postings for this label name.
			sid, err := w.symbols.ReverseLookup(name)
			if err != nil {
				return err
			}
			values := make([]uint32, 0, len(postings[sid]))
			for v := range postings[sid] {
				values = append(values, v)
			}
			// Symbol numbers are in order, so the strings will also be in order.
			slices.Sort(values)
			for _, v := range values {
				value, err := w.symbols.Lookup(v)
				if err != nil {
					return err
				}
				if err := w.writePosting(name, value, postings[sid][v]); err != nil {
					return err
				}
			}
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		default:
		}
	}
	return nil
}

// EncodePostingsRaw uses the "basic" postings list encoding format with no compression:
// <BE uint32 len X><BE uint32 0><BE uint32 1>...<BE uint32 X-1>.
func EncodePostingsRaw(e *encoding.Encbuf, offs []uint32) error {
	e.PutBE32int(len(offs))

	for _, off := range offs {
		if off > (1<<32)-1 {
			return fmt.Errorf("series offset %d exceeds 4 bytes", off)
		}
		e.PutBE32(off)
	}
	return nil
}

func (w *Writer) writePosting(name, value string, offs []uint32) error {
	// Align beginning to 4 bytes for more efficient postings list scans.
	if err := w.fP.AddPadding(4); err != nil {
		return err
	}

	// Write out postings offset table to temporary file as we go.
	w.buf1.Reset()
	w.buf1.PutUvarint(2)
	w.buf1.PutUvarintStr(name)
	w.buf1.PutUvarintStr(value)
	w.buf1.PutUvarint64(w.fP.pos) // This is relative to the postings tmp file, not the final index file.
	if err := w.fPO.Write(w.buf1.Get()); err != nil {
		return err
	}
	w.cntPO++

	w.buf1.Reset()
	if err := w.postingsEncoder(&w.buf1, offs); err != nil {
		return err
	}

	w.buf2.Reset()
	l := w.buf1.Len()
	// We convert to uint to make code compile on 32-bit systems, as math.MaxUint32 doesn't fit into int there.
	if uint(l) > math.MaxUint32 {
		return fmt.Errorf("posting size exceeds 4 bytes: %d", l)
	}
	w.buf2.PutBE32int(l)
	w.buf1.PutHash(w.crc32)
	return w.fP.Write(w.buf2.Get(), w.buf1.Get())
}

func (w *Writer) writePostings() error {
	// There's padding in the tmp file, make sure it actually works.
	if err := w.f.AddPadding(4); err != nil {
		return err
	}
	w.postingsStart = w.f.pos

	// Copy temporary file into main index.
	if err := w.fP.Flush(); err != nil {
		return err
	}
	if _, err := w.fP.f.Seek(0, 0); err != nil {
		return err
	}
	// Don't need to calculate a checksum, so can copy directly.
	n, err := io.CopyBuffer(w.f.fbuf, w.fP.f, make([]byte, 1<<20))
	if err != nil {
		return err
	}
	if uint64(n) != w.fP.pos {
		return fmt.Errorf("wrote %d bytes to posting temporary file, but only read back %d", w.fP.pos, n)
	}
	w.f.pos += uint64(n)

	if err := w.fP.Close(); err != nil {
		return err
	}
	if err := w.fP.Remove(); err != nil {
		return err
	}
	w.fP = nil
	return nil
}

type labelIndexHashEntry struct {
	keys   []string
	offset uint64
}

func (w *Writer) Close() error {
	// Even if this fails, we need to close all the files.
	ensureErr := w.ensureStage(idxStageDone)

	if w.symbolFile != nil {
		if err := w.symbolFile.Close(); err != nil {
			return err
		}
	}
	if w.fP != nil {
		if err := w.fP.Close(); err != nil {
			return err
		}
	}
	if w.fPO != nil {
		if err := w.fPO.Close(); err != nil {
			return err
		}
	}
	if err := w.f.Close(); err != nil {
		return err
	}
	return ensureErr
}

// StringIter iterates over a sorted list of strings.
type StringIter interface {
	// Next advances the iterator and returns true if another value was found.
	Next() bool

	// At returns the value at the current iterator position.
	At() string

	// Err returns the last error of the iterator.
	Err() error
}

type Reader struct {
	b   ByteSlice
	toc *TOC

	// Close that releases the underlying resources of the byte slice.
	c io.Closer

	// Map of LabelName to a list of some LabelValues's position in the offset table.
	// The first and last values for each name are always present.
	postings map[string][]postingOffset
	// For the v1 format, labelname -> labelvalue -> offset.
	postingsV1 map[string]map[string]uint64

	symbols     *Symbols
	nameSymbols map[uint32]string // Cache of the label name symbol lookups,
	// as there are not many and they are half of all lookups.
	st *labels.SymbolTable // TODO: see if we can merge this with nameSymbols.

	dec *Decoder

	version int
}

type postingOffset struct {
	value string
	off   int
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
	return newReader(b, io.NopCloser(nil))
}

// NewFileReader returns a new index reader against the given index file.
func NewFileReader(path string) (*Reader, error) {
	f, err := fileutil.OpenMmapFile(path)
	if err != nil {
		return nil, err
	}
	r, err := newReader(realByteSlice(f.Bytes()), f)
	if err != nil {
		return nil, tsdb_errors.NewMulti(
			err,
			f.Close(),
		).Err()
	}

	return r, nil
}

func newReader(b ByteSlice, c io.Closer) (*Reader, error) {
	r := &Reader{
		b:        b,
		c:        c,
		postings: map[string][]postingOffset{},
		st:       labels.NewSymbolTable(),
	}

	// Verify header.
	if r.b.Len() < HeaderLen {
		return nil, fmt.Errorf("index header: %w", encoding.ErrInvalidSize)
	}
	if m := binary.BigEndian.Uint32(r.b.Range(0, 4)); m != MagicIndex {
		return nil, fmt.Errorf("invalid magic number %x", m)
	}
	r.version = int(r.b.Range(4, 5)[0])

	if r.version != FormatV1 && r.version != FormatV2 {
		return nil, fmt.Errorf("unknown index file version %d", r.version)
	}

	var err error
	r.toc, err = NewTOCFromByteSlice(b)
	if err != nil {
		return nil, fmt.Errorf("read TOC: %w", err)
	}

	r.symbols, err = NewSymbols(r.b, r.version, int(r.toc.Symbols))
	if err != nil {
		return nil, fmt.Errorf("read symbols: %w", err)
	}

	if r.version == FormatV1 {
		// Earlier V1 formats don't have a sorted postings offset table, so
		// load the whole offset table into memory.
		r.postingsV1 = map[string]map[string]uint64{}
		if err := ReadPostingsOffsetTable(r.b, r.toc.PostingsTable, func(name, value []byte, off uint64, _ int) error {
			if _, ok := r.postingsV1[string(name)]; !ok {
				r.postingsV1[string(name)] = map[string]uint64{}
				r.postings[string(name)] = nil // Used to get a list of labelnames in places.
			}
			r.postingsV1[string(name)][string(value)] = off
			return nil
		}); err != nil {
			return nil, fmt.Errorf("read postings table: %w", err)
		}
	} else {
		var lastName, lastValue []byte
		lastOff := 0
		valueCount := 0
		// For the postings offset table we keep every label name but only every nth
		// label value (plus the first and last one), to save memory.
		if err := ReadPostingsOffsetTable(r.b, r.toc.PostingsTable, func(name, value []byte, _ uint64, off int) error {
			if _, ok := r.postings[string(name)]; !ok {
				// Next label name.
				r.postings[string(name)] = []postingOffset{}
				if lastName != nil {
					// Always include last value for each label name.
					r.postings[string(lastName)] = append(r.postings[string(lastName)], postingOffset{value: string(lastValue), off: lastOff})
				}
				valueCount = 0
			}
			if valueCount%symbolFactor == 0 {
				r.postings[string(name)] = append(r.postings[string(name)], postingOffset{value: string(value), off: off})
				lastName, lastValue = nil, nil
			} else {
				lastName, lastValue = name, value
				lastOff = off
			}
			valueCount++
			return nil
		}); err != nil {
			return nil, fmt.Errorf("read postings table: %w", err)
		}
		if lastName != nil {
			r.postings[string(lastName)] = append(r.postings[string(lastName)], postingOffset{value: string(lastValue), off: lastOff})
		}
		// Trim any extra space in the slices.
		for k, v := range r.postings {
			l := make([]postingOffset, len(v))
			copy(l, v)
			r.postings[k] = l
		}
	}

	r.nameSymbols = make(map[uint32]string, len(r.postings))
	for k := range r.postings {
		if k == "" {
			continue
		}
		off, err := r.symbols.ReverseLookup(k)
		if err != nil {
			return nil, fmt.Errorf("reverse symbol lookup: %w", err)
		}
		r.nameSymbols[off] = k
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
	if err := ReadPostingsOffsetTable(r.b, r.toc.PostingsTable, func(name, value []byte, off uint64, _ int) error {
		d := encoding.NewDecbufAt(r.b, int(off), castagnoliTable)
		if d.Err() != nil {
			return d.Err()
		}
		m[labels.Label{Name: string(name), Value: string(value)}] = Range{
			Start: int64(off) + 4,
			End:   int64(off) + 4 + int64(d.Len()),
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read postings table: %w", err)
	}
	return m, nil
}

type Symbols struct {
	bs      ByteSlice
	version int
	off     int

	offsets []int
	seen    int
}

const symbolFactor = 32

// NewSymbols returns a Symbols object for symbol lookups.
func NewSymbols(bs ByteSlice, version, off int) (*Symbols, error) {
	s := &Symbols{
		bs:      bs,
		version: version,
		off:     off,
	}
	d := encoding.NewDecbufAt(bs, off, castagnoliTable)
	var (
		origLen = d.Len()
		cnt     = d.Be32int()
		basePos = off + 4
	)
	s.offsets = make([]int, 0, 1+cnt/symbolFactor)
	for d.Err() == nil && s.seen < cnt {
		if s.seen%symbolFactor == 0 {
			s.offsets = append(s.offsets, basePos+origLen-d.Len())
		}
		d.UvarintBytes() // The symbol.
		s.seen++
	}
	if d.Err() != nil {
		return nil, d.Err()
	}
	return s, nil
}

func (s Symbols) Lookup(o uint32) (string, error) {
	d := encoding.Decbuf{
		B: s.bs.Range(0, s.bs.Len()),
	}

	if s.version == FormatV2 {
		if int(o) >= s.seen {
			return "", fmt.Errorf("unknown symbol offset %d", o)
		}
		d.Skip(s.offsets[int(o/symbolFactor)])
		// Walk until we find the one we want.
		for i := o - (o / symbolFactor * symbolFactor); i > 0; i-- {
			d.UvarintBytes()
		}
	} else {
		d.Skip(int(o))
	}
	sym := d.UvarintStr()
	if d.Err() != nil {
		return "", d.Err()
	}
	return sym, nil
}

func (s Symbols) ReverseLookup(sym string) (uint32, error) {
	if len(s.offsets) == 0 {
		return 0, fmt.Errorf("unknown symbol %q - no symbols", sym)
	}
	i := sort.Search(len(s.offsets), func(i int) bool {
		// Any decoding errors here will be lost, however
		// we already read through all of this at startup.
		d := encoding.Decbuf{
			B: s.bs.Range(0, s.bs.Len()),
		}
		d.Skip(s.offsets[i])
		return yoloString(d.UvarintBytes()) > sym
	})
	d := encoding.Decbuf{
		B: s.bs.Range(0, s.bs.Len()),
	}
	if i > 0 {
		i--
	}
	d.Skip(s.offsets[i])
	res := i * symbolFactor
	var lastLen int
	var lastSymbol string
	for d.Err() == nil && res <= s.seen {
		lastLen = d.Len()
		lastSymbol = yoloString(d.UvarintBytes())
		if lastSymbol >= sym {
			break
		}
		res++
	}
	if d.Err() != nil {
		return 0, d.Err()
	}
	if lastSymbol != sym {
		return 0, fmt.Errorf("unknown symbol %q", sym)
	}
	if s.version == FormatV2 {
		return uint32(res), nil
	}
	return uint32(s.bs.Len() - lastLen), nil
}

func (s Symbols) Size() int {
	return len(s.offsets) * 8
}

func (s Symbols) Iter() StringIter {
	d := encoding.NewDecbufAt(s.bs, s.off, castagnoliTable)
	cnt := d.Be32int()
	return &symbolsIter{
		d:   d,
		cnt: cnt,
	}
}

// symbolsIter implements StringIter.
type symbolsIter struct {
	d   encoding.Decbuf
	cnt int
	cur string
	err error
}

func (s *symbolsIter) Next() bool {
	if s.cnt == 0 || s.err != nil {
		return false
	}
	s.cur = yoloString(s.d.UvarintBytes())
	s.cnt--
	if s.d.Err() != nil {
		s.err = s.d.Err()
		return false
	}
	return true
}

func (s symbolsIter) At() string { return s.cur }
func (s symbolsIter) Err() error { return s.err }

// ReadPostingsOffsetTable reads the postings offset table and at the given position calls f for each
// found entry.
// The name and value parameters passed to f reuse the backing memory of the underlying byte slice,
// so they shouldn't be persisted without previously copying them.
// If f returns an error it stops decoding and returns the received error.
func ReadPostingsOffsetTable(bs ByteSlice, off uint64, f func(name, value []byte, postingsOffset uint64, labelOffset int) error) error {
	d := encoding.NewDecbufAt(bs, int(off), castagnoliTable)
	startLen := d.Len()
	cnt := d.Be32()

	for d.Err() == nil && d.Len() > 0 && cnt > 0 {
		offsetPos := startLen - d.Len()

		if keyCount := d.Uvarint(); keyCount != 2 {
			return fmt.Errorf("unexpected number of keys for postings offset table %d", keyCount)
		}
		name := d.UvarintBytes()
		value := d.UvarintBytes()
		o := d.Uvarint64()
		if d.Err() != nil {
			break
		}
		if err := f(name, value, o, offsetPos); err != nil {
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

func (r *Reader) lookupSymbol(ctx context.Context, o uint32) (string, error) {
	if s, ok := r.nameSymbols[o]; ok {
		return s, nil
	}
	return r.symbols.Lookup(o)
}

// Symbols returns an iterator over the symbols that exist within the index.
func (r *Reader) Symbols() StringIter {
	return r.symbols.Iter()
}

// SymbolTableSize returns the symbol table size in bytes.
func (r *Reader) SymbolTableSize() uint64 {
	return uint64(r.symbols.Size())
}

// SortedLabelValues returns value tuples that exist for the given label name.
// It is not safe to use the return value beyond the lifetime of the byte slice
// passed into the Reader.
func (r *Reader) SortedLabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	values, err := r.LabelValues(ctx, name, matchers...)
	if err == nil && r.version == FormatV1 {
		slices.Sort(values)
	}
	return values, err
}

// LabelValues returns value tuples that exist for the given label name.
// It is not safe to use the return value beyond the lifetime of the byte slice
// passed into the Reader.
// TODO(replay): Support filtering by matchers.
func (r *Reader) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	if len(matchers) > 0 {
		return nil, fmt.Errorf("matchers parameter is not implemented: %+v", matchers)
	}

	if r.version == FormatV1 {
		e, ok := r.postingsV1[name]
		if !ok {
			return nil, nil
		}
		values := make([]string, 0, len(e))
		for k := range e {
			values = append(values, k)
		}
		return values, nil
	}
	e, ok := r.postings[name]
	if !ok {
		return nil, nil
	}
	if len(e) == 0 {
		return nil, nil
	}

	values := make([]string, 0, len(e)*symbolFactor)
	lastVal := e[len(e)-1].value
	err := r.traversePostingOffsets(ctx, e[0].off, func(val string, _ uint64) (bool, error) {
		values = append(values, val)
		return val != lastVal, nil
	})
	return values, err
}

// LabelNamesFor returns all the label names for the series referred to by IDs.
// The names returned are sorted.
func (r *Reader) LabelNamesFor(ctx context.Context, ids ...storage.SeriesRef) ([]string, error) {
	// Gather offsetsMap the name offsetsMap in the symbol table first
	offsetsMap := make(map[uint32]struct{})
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		offset := id
		// In version 2 series IDs are no longer exact references but series are 16-byte padded
		// and the ID is the multiple of 16 of the actual position.
		if r.version == FormatV2 {
			offset = id * seriesByteAlign
		}

		d := encoding.NewDecbufUvarintAt(r.b, int(offset), castagnoliTable)
		buf := d.Get()
		if d.Err() != nil {
			return nil, fmt.Errorf("get buffer for series: %w", d.Err())
		}

		offsets, err := r.dec.LabelNamesOffsetsFor(buf)
		if err != nil {
			return nil, fmt.Errorf("get label name offsets: %w", err)
		}
		for _, off := range offsets {
			offsetsMap[off] = struct{}{}
		}
	}

	// Lookup the unique symbols.
	names := make([]string, 0, len(offsetsMap))
	for off := range offsetsMap {
		name, err := r.lookupSymbol(ctx, off)
		if err != nil {
			return nil, fmt.Errorf("lookup symbol in LabelNamesFor: %w", err)
		}
		names = append(names, name)
	}

	slices.Sort(names)

	return names, nil
}

// LabelValueFor returns label value for the given label name in the series referred to by ID.
func (r *Reader) LabelValueFor(ctx context.Context, id storage.SeriesRef, label string) (string, error) {
	offset := id
	// In version 2 series IDs are no longer exact references but series are 16-byte padded
	// and the ID is the multiple of 16 of the actual position.
	if r.version == FormatV2 {
		offset = id * seriesByteAlign
	}
	d := encoding.NewDecbufUvarintAt(r.b, int(offset), castagnoliTable)
	buf := d.Get()
	if d.Err() != nil {
		return "", fmt.Errorf("label values for: %w", d.Err())
	}

	value, err := r.dec.LabelValueFor(ctx, buf, label)
	if err != nil {
		return "", storage.ErrNotFound
	}

	if value == "" {
		return "", storage.ErrNotFound
	}

	return value, nil
}

// Series reads the series with the given ID and writes its labels and chunks into builder and chks.
func (r *Reader) Series(id storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	offset := id
	// In version 2 series IDs are no longer exact references but series are 16-byte padded
	// and the ID is the multiple of 16 of the actual position.
	if r.version == FormatV2 {
		offset = id * seriesByteAlign
	}
	d := encoding.NewDecbufUvarintAt(r.b, int(offset), castagnoliTable)
	if d.Err() != nil {
		return d.Err()
	}
	builder.SetSymbolTable(r.st)
	builder.Reset()
	err := r.dec.Series(d.Get(), builder, chks)
	if err != nil {
		return fmt.Errorf("read series: %w", err)
	}
	return nil
}

// traversePostingOffsets traverses r's posting offsets table, starting at off, and calls cb with every label value and postings offset.
// If cb returns false (or an error), the traversing is interrupted.
func (r *Reader) traversePostingOffsets(ctx context.Context, off int, cb func(string, uint64) (bool, error)) error {
	// Don't Crc32 the entire postings offset table, this is very slow
	// so hope any issues were caught at startup.
	d := encoding.NewDecbufAt(r.b, int(r.toc.PostingsTable), nil)
	d.Skip(off)
	skip := 0
	ctxErr := ctx.Err()
	for d.Err() == nil && ctxErr == nil {
		if skip == 0 {
			// These are always the same number of bytes,
			// and it's faster to skip than to parse.
			skip = d.Len()
			d.Uvarint()      // Keycount.
			d.UvarintBytes() // Label name.
			skip -= d.Len()
		} else {
			d.Skip(skip)
		}
		v := yoloString(d.UvarintBytes()) // Label value.
		postingsOff := d.Uvarint64()      // Offset.
		if ok, err := cb(v, postingsOff); err != nil {
			return err
		} else if !ok {
			break
		}
		ctxErr = ctx.Err()
	}
	if d.Err() != nil {
		return fmt.Errorf("get postings offset entry: %w", d.Err())
	}
	if ctxErr != nil {
		return fmt.Errorf("get postings offset entry: %w", ctxErr)
	}
	return nil
}

func (r *Reader) Postings(ctx context.Context, name string, values ...string) (Postings, error) {
	if r.version == FormatV1 {
		e, ok := r.postingsV1[name]
		if !ok {
			return EmptyPostings(), nil
		}
		res := make([]Postings, 0, len(values))
		for _, v := range values {
			postingsOff, ok := e[v]
			if !ok {
				continue
			}
			// Read from the postings table.
			d := encoding.NewDecbufAt(r.b, int(postingsOff), castagnoliTable)
			_, p, err := r.dec.Postings(d.Get())
			if err != nil {
				return nil, fmt.Errorf("decode postings: %w", err)
			}
			res = append(res, p)
		}
		return Merge(ctx, res...), nil
	}

	e, ok := r.postings[name]
	if !ok {
		return EmptyPostings(), nil
	}

	if len(values) == 0 {
		return EmptyPostings(), nil
	}

	slices.Sort(values) // Values must be in order so we can step through the table on disk.
	res := make([]Postings, 0, len(values))
	valueIndex := 0
	for valueIndex < len(values) && values[valueIndex] < e[0].value {
		// Discard values before the start.
		valueIndex++
	}
	for valueIndex < len(values) {
		value := values[valueIndex]

		i := sort.Search(len(e), func(i int) bool { return e[i].value >= value })
		if i == len(e) {
			// We're past the end.
			break
		}
		if i > 0 && e[i].value != value {
			// Need to look from previous entry.
			i--
		}

		if err := r.traversePostingOffsets(ctx, e[i].off, func(val string, postingsOff uint64) (bool, error) {
			for val >= value {
				if val == value {
					// Read from the postings table.
					d2 := encoding.NewDecbufAt(r.b, int(postingsOff), castagnoliTable)
					_, p, err := r.dec.Postings(d2.Get())
					if err != nil {
						return false, fmt.Errorf("decode postings: %w", err)
					}
					res = append(res, p)
				}
				valueIndex++
				if valueIndex == len(values) {
					break
				}
				value = values[valueIndex]
			}
			if i+1 == len(e) || value >= e[i+1].value || valueIndex == len(values) {
				// Need to go to a later postings offset entry, if there is one.
				return false, nil
			}
			return true, nil
		}); err != nil {
			return nil, err
		}
	}

	return Merge(ctx, res...), nil
}

func (r *Reader) PostingsForLabelMatching(ctx context.Context, name string, match func(string) bool) Postings {
	if r.version == FormatV1 {
		return r.postingsForLabelMatchingV1(ctx, name, match)
	}

	e := r.postings[name]
	if len(e) == 0 {
		return EmptyPostings()
	}

	lastVal := e[len(e)-1].value
	var its []Postings
	if err := r.traversePostingOffsets(ctx, e[0].off, func(val string, postingsOff uint64) (bool, error) {
		if match(val) {
			// We want this postings iterator since the value is a match
			postingsDec := encoding.NewDecbufAt(r.b, int(postingsOff), castagnoliTable)
			_, p, err := r.dec.PostingsFromDecbuf(postingsDec)
			if err != nil {
				return false, fmt.Errorf("decode postings: %w", err)
			}
			its = append(its, p)
		}
		return val != lastVal, nil
	}); err != nil {
		return ErrPostings(err)
	}

	return Merge(ctx, its...)
}

func (r *Reader) postingsForLabelMatchingV1(ctx context.Context, name string, match func(string) bool) Postings {
	e := r.postingsV1[name]
	if len(e) == 0 {
		return EmptyPostings()
	}

	var its []Postings
	count := 1
	for val, offset := range e {
		if count%checkContextEveryNIterations == 0 && ctx.Err() != nil {
			return ErrPostings(ctx.Err())
		}
		count++
		if !match(val) {
			continue
		}

		// Read from the postings table.
		d := encoding.NewDecbufAt(r.b, int(offset), castagnoliTable)
		_, p, err := r.dec.PostingsFromDecbuf(d)
		if err != nil {
			return ErrPostings(fmt.Errorf("decode postings: %w", err))
		}

		its = append(its, p)
	}

	return Merge(ctx, its...)
}

// SortedPostings returns the given postings list reordered so that the backing series
// are sorted.
func (r *Reader) SortedPostings(p Postings) Postings {
	return p
}

// ShardedPostings returns a postings list filtered by the provided shardIndex out of shardCount.
func (r *Reader) ShardedPostings(p Postings, shardIndex, shardCount uint64) Postings {
	var (
		out     = make([]storage.SeriesRef, 0, 128)
		bufLbls = labels.ScratchBuilder{}
	)

	for p.Next() {
		id := p.At()

		// Get the series labels (no chunks).
		err := r.Series(id, &bufLbls, nil)
		if err != nil {
			return ErrPostings(fmt.Errorf("series %d not found", id))
		}

		// Check if the series belong to the shard.
		if labels.StableHash(bufLbls.Labels())%shardCount != shardIndex {
			continue
		}

		out = append(out, id)
	}

	return NewListPostings(out)
}

// Size returns the size of an index file.
func (r *Reader) Size() int64 {
	return int64(r.b.Len())
}

// LabelNames returns all the unique label names present in the index.
// TODO(twilkie) implement support for matchers.
func (r *Reader) LabelNames(_ context.Context, matchers ...*labels.Matcher) ([]string, error) {
	if len(matchers) > 0 {
		return nil, fmt.Errorf("matchers parameter is not implemented: %+v", matchers)
	}

	labelNames := make([]string, 0, len(r.postings))
	for name := range r.postings {
		if name == allPostingsKey.Name {
			// This is not from any metric.
			continue
		}
		labelNames = append(labelNames, name)
	}
	slices.Sort(labelNames)
	return labelNames, nil
}

// NewStringListIter returns a StringIter for the given sorted list of strings.
func NewStringListIter(s []string) StringIter {
	return &stringListIter{l: s}
}

// stringListIter implements StringIter.
type stringListIter struct {
	l   []string
	cur string
}

func (s *stringListIter) Next() bool {
	if len(s.l) == 0 {
		return false
	}
	s.cur = s.l[0]
	s.l = s.l[1:]
	return true
}
func (s stringListIter) At() string { return s.cur }
func (s stringListIter) Err() error { return nil }

// Decoder provides decoding methods for the v1 and v2 index file format.
//
// It currently does not contain decoding methods for all entry types but can be extended
// by them if there's demand.
type Decoder struct {
	LookupSymbol func(context.Context, uint32) (string, error)
}

// Postings returns a postings list for b and its number of elements.
func (dec *Decoder) Postings(b []byte) (int, Postings, error) {
	d := encoding.Decbuf{B: b}
	return dec.PostingsFromDecbuf(d)
}

// PostingsFromDecbuf returns a postings list for d and its number of elements.
func (dec *Decoder) PostingsFromDecbuf(d encoding.Decbuf) (int, Postings, error) {
	n := d.Be32int()
	l := d.Get()
	if d.Err() != nil {
		return 0, nil, d.Err()
	}
	if len(l) != 4*n {
		return 0, nil, fmt.Errorf("unexpected postings length, should be %d bytes for %d postings, got %d bytes", 4*n, n, len(l))
	}
	return n, newBigEndianPostings(l), nil
}

// LabelNamesOffsetsFor decodes the offsets of the name symbols for a given series.
// They are returned in the same order they're stored, which should be sorted lexicographically.
func (dec *Decoder) LabelNamesOffsetsFor(b []byte) ([]uint32, error) {
	d := encoding.Decbuf{B: b}
	k := d.Uvarint()

	offsets := make([]uint32, k)
	for i := 0; i < k; i++ {
		offsets[i] = uint32(d.Uvarint())
		_ = d.Uvarint() // skip the label value

		if d.Err() != nil {
			return nil, fmt.Errorf("read series label offsets: %w", d.Err())
		}
	}

	return offsets, d.Err()
}

// LabelValueFor decodes a label for a given series.
func (dec *Decoder) LabelValueFor(ctx context.Context, b []byte, label string) (string, error) {
	d := encoding.Decbuf{B: b}
	k := d.Uvarint()

	for i := 0; i < k; i++ {
		lno := uint32(d.Uvarint())
		lvo := uint32(d.Uvarint())

		if d.Err() != nil {
			return "", fmt.Errorf("read series label offsets: %w", d.Err())
		}

		ln, err := dec.LookupSymbol(ctx, lno)
		if err != nil {
			return "", fmt.Errorf("lookup label name: %w", err)
		}

		if ln == label {
			lv, err := dec.LookupSymbol(ctx, lvo)
			if err != nil {
				return "", fmt.Errorf("lookup label value: %w", err)
			}

			return lv, nil
		}
	}

	return "", d.Err()
}

// Series decodes a series entry from the given byte slice into builder and chks.
// Previous contents of builder can be overwritten - make sure you copy before retaining.
// Skips reading chunks metadata if chks is nil.
func (dec *Decoder) Series(b []byte, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	builder.Reset()
	if chks != nil {
		*chks = (*chks)[:0]
	}

	d := encoding.Decbuf{B: b}

	k := d.Uvarint()

	for i := 0; i < k; i++ {
		lno := uint32(d.Uvarint())
		lvo := uint32(d.Uvarint())

		if d.Err() != nil {
			return fmt.Errorf("read series label offsets: %w", d.Err())
		}

		ln, err := dec.LookupSymbol(context.TODO(), lno)
		if err != nil {
			return fmt.Errorf("lookup label name: %w", err)
		}
		lv, err := dec.LookupSymbol(context.TODO(), lvo)
		if err != nil {
			return fmt.Errorf("lookup label value: %w", err)
		}

		builder.Add(ln, lv)
	}

	// Skip reading chunks metadata if chks is nil.
	if chks == nil {
		return d.Err()
	}

	// Read the chunks meta data.
	k = d.Uvarint()

	if k == 0 {
		return d.Err()
	}

	t0 := d.Varint64()
	maxt := int64(d.Uvarint64()) + t0
	ref0 := int64(d.Uvarint64())

	*chks = append(*chks, chunks.Meta{
		Ref:     chunks.ChunkRef(ref0),
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
			return fmt.Errorf("read meta for chunk %d: %w", i, d.Err())
		}

		*chks = append(*chks, chunks.Meta{
			Ref:     chunks.ChunkRef(ref0),
			MinTime: mint,
			MaxTime: maxt,
		})
	}
	return d.Err()
}

func yoloString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}
