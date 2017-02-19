package tsdb

import (
	"bufio"
	"encoding/binary"
	"hash"
	"hash/crc32"
	"io"
	"sort"
	"strings"

	"github.com/bradfitz/slice"
	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
	"github.com/pkg/errors"
)

const (
	// MagicSeries 4 bytes at the head of series file.
	MagicSeries = 0x85BD40DD

	// MagicIndex 4 bytes at the head of an index file.
	MagicIndex = 0xBAAAD700
)

const compactionPageBytes = minSectorSize * 64

// ChunkWriter serializes a time block of chunked series data.
type ChunkWriter interface {
	// WriteChunks writes several chunks. The data field of the ChunkMetas
	// must be populated.
	// After returning successfully, the Ref fields in the ChunkMetas
	// is set and can be used to retrieve the chunks from the written data.
	WriteChunks(chunks ...ChunkMeta) error

	// Size returns the size of the data written so far.
	Size() int64

	// Close writes any required finalization and closes the resources
	// associated with the underlying writer.
	Close() error
}

// chunkWriter implements the ChunkWriter interface for the standard
// serialization format.
type chunkWriter struct {
	ow    io.Writer
	w     *bufio.Writer
	n     int64
	c     int
	crc32 hash.Hash
}

func newChunkWriter(w io.Writer) *chunkWriter {
	return &chunkWriter{
		ow:    w,
		w:     bufio.NewWriterSize(w, 1*1024*1024),
		n:     0,
		crc32: crc32.New(crc32.MakeTable(crc32.Castagnoli)),
	}
}

func (w *chunkWriter) write(wr io.Writer, b []byte) error {
	n, err := wr.Write(b)
	w.n += int64(n)
	return err
}

func (w *chunkWriter) writeMeta() error {
	b := [8]byte{}

	binary.BigEndian.PutUint32(b[:4], MagicSeries)
	b[4] = flagStd

	return w.write(w.w, b[:])
}

func (w *chunkWriter) WriteChunks(chks ...ChunkMeta) error {
	// Initialize with meta data.
	if w.n == 0 {
		if err := w.writeMeta(); err != nil {
			return err
		}
	}

	w.crc32.Reset()
	wr := io.MultiWriter(w.crc32, w.w)

	// For normal reads we don't need the number of the chunk section but
	// it allows us to verify checksums without reading the index file.
	// The offsets are also technically enough to calculate chunk size. but
	// holding the length of each chunk could later allow for adding padding
	// between chunks.
	b := [binary.MaxVarintLen32]byte{}
	n := binary.PutUvarint(b[:], uint64(len(chks)))

	if err := w.write(wr, b[:n]); err != nil {
		return err
	}

	for i := range chks {
		chk := &chks[i]

		chk.Ref = uint64(w.n)

		n = binary.PutUvarint(b[:], uint64(len(chk.Chunk.Bytes())))

		if err := w.write(wr, b[:n]); err != nil {
			return err
		}
		if err := w.write(wr, []byte{byte(chk.Chunk.Encoding())}); err != nil {
			return err
		}
		if err := w.write(wr, chk.Chunk.Bytes()); err != nil {
			return err
		}
		chk.Chunk = nil
	}

	if err := w.write(w.w, w.crc32.Sum(nil)); err != nil {
		return err
	}
	return nil
}

func (w *chunkWriter) Size() int64 {
	return w.n
}

func (w *chunkWriter) Close() error {
	// Initialize block in case no data was written to it.
	if w.n == 0 {
		if err := w.writeMeta(); err != nil {
			return err
		}
	}
	return w.w.Flush()
}

// ChunkMeta holds information about a chunk of data.
type ChunkMeta struct {
	// Ref and Chunk hold either a reference that can be used to retrieve
	// chunk data or the data itself.
	// Generally, only one of them is set.
	Ref   uint64
	Chunk chunks.Chunk

	MinTime, MaxTime int64 // time range the data covers
}

// IndexWriter serialized the index for a block of series data.
// The methods must generally be called in order they are specified.
type IndexWriter interface {
	// AddSeries populates the index writer witha series and its offsets
	// of chunks that the index can reference.
	// The reference number is used to resolve a series against the postings
	// list iterator. It only has to be available during the write processing.
	AddSeries(ref uint32, l labels.Labels, chunks ...ChunkMeta)

	// WriteLabelIndex serializes an index from label names to values.
	// The passed in values chained tuples of strings of the length of names.
	WriteLabelIndex(names []string, values []string) error

	// WritePostings writes a postings list for a single label pair.
	WritePostings(name, value string, it Postings) error

	// Size returns the size of the data written so far.
	Size() int64

	// Close writes any finalization and closes theresources associated with
	// the underlying writer.
	Close() error
}

type indexWriterSeries struct {
	labels labels.Labels
	chunks []ChunkMeta // series file offset of chunks
	offset uint32      // index file offset of series reference
}

// indexWriter implements the IndexWriter interface for the standard
// serialization format.
type indexWriter struct {
	ow      io.Writer
	w       *bufio.Writer
	n       int64
	started bool

	series map[uint32]*indexWriterSeries

	symbols      map[string]uint32 // symbol offsets
	labelIndexes []hashEntry       // label index offsets
	postings     []hashEntry       // postings lists offsets

	crc32 hash.Hash
}

func newIndexWriter(w io.Writer) *indexWriter {
	return &indexWriter{
		w:       bufio.NewWriterSize(w, 1*1024*1024),
		ow:      w,
		n:       0,
		symbols: make(map[string]uint32, 4096),
		series:  make(map[uint32]*indexWriterSeries, 4096),
		crc32:   crc32.New(crc32.MakeTable(crc32.Castagnoli)),
	}
}

func (w *indexWriter) write(wr io.Writer, b []byte) error {
	n, err := wr.Write(b)
	w.n += int64(n)
	return err
}

// section writes a CRC32 checksummed section of length l and guarded by flag.
func (w *indexWriter) section(l uint32, flag byte, f func(w io.Writer) error) error {
	w.crc32.Reset()
	wr := io.MultiWriter(w.crc32, w.w)

	b := [5]byte{flag, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], l)

	if err := w.write(wr, b[:]); err != nil {
		return errors.Wrap(err, "writing header")
	}

	if err := f(wr); err != nil {
		return errors.Wrap(err, "contents write func")
	}
	if err := w.write(w.w, w.crc32.Sum(nil)); err != nil {
		return errors.Wrap(err, "writing checksum")
	}
	return nil
}

func (w *indexWriter) writeMeta() error {
	b := [8]byte{}

	binary.BigEndian.PutUint32(b[:4], MagicIndex)
	b[4] = flagStd

	return w.write(w.w, b[:])
}

func (w *indexWriter) AddSeries(ref uint32, lset labels.Labels, chunks ...ChunkMeta) {
	// Populate the symbol table from all label sets we have to reference.
	for _, l := range lset {
		w.symbols[l.Name] = 0
		w.symbols[l.Value] = 0
	}

	w.series[ref] = &indexWriterSeries{
		labels: lset,
		chunks: chunks,
	}
}

func (w *indexWriter) writeSymbols() error {
	// Generate sorted list of strings we will store as reference table.
	symbols := make([]string, 0, len(w.symbols))
	for s := range w.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// The start of the section plus a 5 byte section header are our base.
	// TODO(fabxc): switch to relative offsets and hold sections in a TOC.
	base := uint32(w.n) + 5

	buf := [binary.MaxVarintLen32]byte{}
	b := append(make([]byte, 0, 4096), flagStd)

	for _, s := range symbols {
		w.symbols[s] = base + uint32(len(b))

		n := binary.PutUvarint(buf[:], uint64(len(s)))
		b = append(b, buf[:n]...)
		b = append(b, s...)
	}

	l := uint32(len(b))

	return w.section(l, flagStd, func(wr io.Writer) error {
		return w.write(wr, b)
	})
}

func (w *indexWriter) writeSeries() error {
	// Series must be stored sorted along their labels.
	series := make([]*indexWriterSeries, 0, len(w.series))

	for _, s := range w.series {
		series = append(series, s)
	}
	slice.Sort(series, func(i, j int) bool {
		return labels.Compare(series[i].labels, series[j].labels) < 0
	})

	// Current end of file plus 5 bytes for section header.
	// TODO(fabxc): switch to relative offsets.
	base := uint32(w.n) + 5

	b := make([]byte, 0, 1<<20) // 1MiB
	buf := make([]byte, binary.MaxVarintLen64)

	for _, s := range series {
		// Write label set symbol references.
		s.offset = base + uint32(len(b))

		n := binary.PutUvarint(buf, uint64(len(s.labels)))
		b = append(b, buf[:n]...)

		for _, l := range s.labels {
			n = binary.PutUvarint(buf, uint64(w.symbols[l.Name]))
			b = append(b, buf[:n]...)
			n = binary.PutUvarint(buf, uint64(w.symbols[l.Value]))
			b = append(b, buf[:n]...)
		}

		// Write chunks meta data including reference into chunk file.
		n = binary.PutUvarint(buf, uint64(len(s.chunks)))
		b = append(b, buf[:n]...)

		for _, c := range s.chunks {
			n = binary.PutVarint(buf, c.MinTime)
			b = append(b, buf[:n]...)
			n = binary.PutVarint(buf, c.MaxTime)
			b = append(b, buf[:n]...)

			n = binary.PutUvarint(buf, uint64(c.Ref))
			b = append(b, buf[:n]...)
		}
	}

	l := uint32(len(b))

	return w.section(l, flagStd, func(wr io.Writer) error {
		return w.write(wr, b)
	})
}

func (w *indexWriter) init() error {
	if err := w.writeMeta(); err != nil {
		return err
	}
	if err := w.writeSymbols(); err != nil {
		return err
	}
	if err := w.writeSeries(); err != nil {
		return err
	}
	w.started = true

	return nil
}

func (w *indexWriter) WriteLabelIndex(names []string, values []string) error {
	if !w.started {
		if err := w.init(); err != nil {
			return err
		}
	}

	valt, err := newStringTuples(values, len(names))
	if err != nil {
		return err
	}
	sort.Sort(valt)

	w.labelIndexes = append(w.labelIndexes, hashEntry{
		name:   strings.Join(names, string(sep)),
		offset: uint32(w.n),
	})

	buf := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(buf, uint64(len(names)))

	l := uint32(n) + uint32(len(values)*4)

	return w.section(l, flagStd, func(wr io.Writer) error {
		// First byte indicates tuple size for index.
		if err := w.write(wr, buf[:n]); err != nil {
			return err
		}

		for _, v := range valt.s {
			binary.BigEndian.PutUint32(buf, w.symbols[v])

			if err := w.write(wr, buf[:4]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (w *indexWriter) WritePostings(name, value string, it Postings) error {
	if !w.started {
		if err := w.init(); err != nil {
			return err
		}
	}

	key := name + string(sep) + value

	w.postings = append(w.postings, hashEntry{
		name:   key,
		offset: uint32(w.n),
	})

	b := make([]byte, 0, 4096)
	buf := [4]byte{}

	// Order of the references in the postings list does not imply order
	// of the series references within the persisted block they are mapped to.
	// We have to sort the new references again.
	var refs []uint32

	for it.Next() {
		s, ok := w.series[it.At()]
		if !ok {
			return errors.Errorf("series for reference %d not found", it.At())
		}
		refs = append(refs, s.offset)
	}
	if err := it.Err(); err != nil {
		return err
	}

	slice.Sort(refs, func(i, j int) bool { return refs[i] < refs[j] })

	for _, r := range refs {
		binary.BigEndian.PutUint32(buf[:], r)
		b = append(b, buf[:]...)
	}

	return w.section(uint32(len(b)), flagStd, func(wr io.Writer) error {
		return w.write(wr, b)
	})
}

func (w *indexWriter) Size() int64 {
	return w.n
}

type hashEntry struct {
	name   string
	offset uint32
}

func (w *indexWriter) writeHashmap(h []hashEntry) error {
	b := make([]byte, 0, 4096)
	buf := [binary.MaxVarintLen32]byte{}

	for _, e := range h {
		n := binary.PutUvarint(buf[:], uint64(len(e.name)))
		b = append(b, buf[:n]...)
		b = append(b, e.name...)

		n = binary.PutUvarint(buf[:], uint64(e.offset))
		b = append(b, buf[:n]...)
	}

	return w.section(uint32(len(b)), flagStd, func(wr io.Writer) error {
		return w.write(wr, b)
	})
}

func (w *indexWriter) finalize() error {
	// Write out hash maps to jump to correct label index and postings sections.
	lo := uint32(w.n)
	if err := w.writeHashmap(w.labelIndexes); err != nil {
		return err
	}

	po := uint32(w.n)
	if err := w.writeHashmap(w.postings); err != nil {
		return err
	}

	// Terminate index file with offsets to hashmaps. This is the entry Pointer
	// for any index query.
	// TODO(fabxc): also store offset to series section to allow plain
	// iteration over all existing series?
	// TODO(fabxc): store references like these that are not resolved via direct
	// mmap using explicit endianness?
	b := [8]byte{}
	binary.BigEndian.PutUint32(b[:4], lo)
	binary.BigEndian.PutUint32(b[4:], po)

	return w.write(w.w, b[:])
}

func (w *indexWriter) Close() error {
	// Handle blocks without any data.
	if !w.started {
		if err := w.init(); err != nil {
			return err
		}
	}
	if err := w.finalize(); err != nil {
		return err
	}
	return w.w.Flush()
}
