package tsdb

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"sort"
	"strings"
)

const (
	// MagicSeries 4 bytes at the head of series file.
	MagicSeries = 0x85BD40DD

	// MagicIndex 4 bytes at the head of an index file.
	MagicIndex = 0xBAAAD700
)

// SeriesWriter serializes a time block of chunked series data.
type SeriesWriter interface {
	// WriteSeries writes the time series data chunks for a single series.
	// The reference is used to resolve the correct series in the written index.
	// It only has to be valid for the duration of the write.
	WriteSeries(ref uint32, l Labels, cds []*chunkDesc) error

	// Size returns the size of the data written so far.
	Size() int64

	// Close writes any required finalization and closes the resources
	// associated with the underlying writer.
	Close() error
}

// seriesWriter implements the SeriesWriter interface for the standard
// serialization format.
type seriesWriter struct {
	w io.Writer
	n int64
	c int

	baseTimestamp int64
	index         IndexWriter
}

func newSeriesWriter(w io.Writer, index IndexWriter, base int64) *seriesWriter {
	return &seriesWriter{
		w:             w,
		n:             0,
		index:         index,
		baseTimestamp: base,
	}
}

func (w *seriesWriter) write(wr io.Writer, b []byte) error {
	n, err := wr.Write(b)
	w.n += int64(n)
	return err
}

func (w *seriesWriter) writeMeta() error {
	b := [8]byte{}

	binary.BigEndian.PutUint32(b[:4], MagicSeries)
	b[4] = flagStd

	return w.write(w.w, b[:])
}

func (w *seriesWriter) WriteSeries(ref uint32, lset Labels, chks []*chunkDesc) error {
	// Initialize with meta data.
	if w.n == 0 {
		if err := w.writeMeta(); err != nil {
			return err
		}
	}

	// TODO(fabxc): is crc32 enough for chunks of one series?
	h := crc32.NewIEEE()
	wr := io.MultiWriter(h, w.w)

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

	offsets := make([]ChunkOffset, 0, len(chks))
	lastTimestamp := w.baseTimestamp

	for _, cd := range chks {
		offsets = append(offsets, ChunkOffset{
			Value:  lastTimestamp,
			Offset: uint32(w.n),
		})
		n = binary.PutUvarint(b[:], uint64(len(cd.chunk.Bytes())))

		if err := w.write(wr, b[:n]); err != nil {
			return err
		}
		if err := w.write(wr, []byte{byte(cd.chunk.Encoding())}); err != nil {
			return err
		}
		if err := w.write(wr, cd.chunk.Bytes()); err != nil {
			return err
		}
		lastTimestamp = cd.lastTimestamp
	}

	if err := w.write(w.w, h.Sum(nil)); err != nil {
		return err
	}

	if w.index != nil {
		w.index.AddSeries(ref, lset, offsets...)
	}
	return nil
}

func (w *seriesWriter) Size() int64 {
	return w.n
}

func (w *seriesWriter) Close() error {
	if f, ok := w.w.(*os.File); ok {
		if err := f.Sync(); err != nil {
			return err
		}
	}

	if c, ok := w.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type ChunkOffset struct {
	Value  int64
	Offset uint32
}

type BlockStats struct {
}

// IndexWriter serialized the index for a block of series data.
// The methods must generally be called in order they are specified.
type IndexWriter interface {
	// AddSeries populates the index writer witha series and its offsets
	// of chunks that the index can reference.
	// The reference number is used to resolve a series against the postings
	// list iterator. It only has to be available during the write processing.
	AddSeries(ref uint32, l Labels, o ...ChunkOffset)

	// WriteStats writes final stats for the indexed block.
	WriteStats(*BlockStats) error

	// WriteLabelIndex serializes an index from label names to values.
	// The passed in values chained tuples of strings of the length of names.
	WriteLabelIndex(names []string, values []string) error

	// WritePostings writes a postings list for a single label pair.
	WritePostings(name, value string, it Iterator) error

	// Size returns the size of the data written so far.
	Size() int64

	// Close writes any finalization and closes theresources associated with
	// the underlying writer.
	Close() error
}

type indexWriterSeries struct {
	labels Labels
	chunks []ChunkOffset // series file offset of chunks
	offset uint32        // index file offset of series reference
}

// indexWriter implements the IndexWriter interface for the standard
// serialization format.
type indexWriter struct {
	w io.Writer
	n int64

	series map[uint32]*indexWriterSeries

	symbols      map[string]uint32 // symbol offsets
	labelIndexes []hashEntry       // label index offsets
	postings     []hashEntry       // postings lists offsets
}

func newIndexWriter(w io.Writer) *indexWriter {
	return &indexWriter{
		w:            w,
		n:            0,
		symbols:      make(map[string]uint32, 4096),
		series:       make(map[uint32]*indexWriterSeries, 4096),
		labelIndexes: make([]hashEntry, 10),
	}
}

func (w *indexWriter) write(wr io.Writer, b []byte) error {
	n, err := wr.Write(b)
	w.n += int64(n)
	return err
}

// section writes a CRC32 checksummed section of length l and guarded by flag.
func (w *indexWriter) section(l uint32, flag byte, f func(w io.Writer) error) error {
	l++ // account for flag byte

	h := crc32.NewIEEE()
	wr := io.MultiWriter(h, w.w)

	b := [5]byte{flagStd, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], l)

	if err := w.write(wr, b[:]); err != nil {
		return err
	}

	if err := f(wr); err != nil {
		return err
	}
	return w.write(w.w, h.Sum(nil))
}

func (w *indexWriter) writeMeta() error {
	b := [8]byte{}

	binary.BigEndian.PutUint32(b[:4], MagicIndex)
	b[4] = flagStd

	return w.write(w.w, b[:])
}

func (w *indexWriter) AddSeries(ref uint32, lset Labels, offsets ...ChunkOffset) {
	// Populate the symbol table from all label sets we have to reference.
	for _, l := range lset {
		w.symbols[l.Name] = 0
		w.symbols[l.Value] = 0
	}

	w.series[ref] = &indexWriterSeries{
		labels: lset,
		chunks: offsets,
	}
}

func (w *indexWriter) WriteStats(*BlockStats) error {
	if w.n == 0 {
		if err := w.writeMeta(); err != nil {
			return err
		}
		if err := w.writeSymbols(); err != nil {
			return err
		}
		if err := w.writeSeries(); err != nil {
			return err
		}
	}
	return nil
}

func (w *indexWriter) writeSymbols() error {
	// Generate sorted list of strings we will store as reference table.
	symbols := make([]string, 0, len(w.symbols))
	for s := range w.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	buf := [binary.MaxVarintLen32]byte{}
	b := append(make([]byte, 0, 4096), flagStd)

	for _, s := range symbols {
		w.symbols[s] = uint32(w.n) + uint32(len(b))

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
	b := make([]byte, 0, 4096)
	buf := make([]byte, binary.MaxVarintLen64)

	for _, s := range w.series {
		// Write label set symbol references.
		s.offset = uint32(w.n) + uint32(len(b))

		n := binary.PutUvarint(buf, uint64(len(s.labels)))
		b = append(b, buf[:n]...)

		for _, l := range s.labels {
			n = binary.PutUvarint(buf, uint64(w.symbols[l.Name]))
			b = append(b, buf[:n]...)
			n = binary.PutUvarint(buf, uint64(w.symbols[l.Value]))
			b = append(b, buf[:n]...)
		}

		// Write skiplist to chunk offsets.
		n = binary.PutUvarint(buf, uint64(len(s.chunks)))
		b = append(b, buf[:n]...)

		for _, c := range s.chunks {
			n = binary.PutVarint(buf, c.Value)
			b = append(b, buf[:n]...)

			n = binary.PutUvarint(buf, uint64(c.Offset))
			b = append(b, buf[:n]...)
		}
	}

	l := uint32(len(b))

	return w.section(l, flagStd, func(wr io.Writer) error {
		return w.write(wr, b)
	})
}

func (w *indexWriter) WriteLabelIndex(names []string, values []string) error {
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

			if err := w.write(wr, buf); err != nil {
				return err
			}
		}
		return nil
	})
}

func (w *indexWriter) WritePostings(name, value string, it Iterator) error {
	key := name + string(sep) + value

	w.postings = append(w.postings, hashEntry{
		name:   key,
		offset: uint32(w.n),
	})

	b := make([]byte, 0, 4096)
	buf := [4]byte{}

	for it.Next() {
		v := w.series[it.Value()].offset
		binary.BigEndian.PutUint32(buf[:], v)

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

	return w.section(uint32(len(buf)), flagStd, func(wr io.Writer) error {
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
	if err := w.finalize(); err != nil {
		return err
	}

	if f, ok := w.w.(*os.File); ok {
		if err := f.Sync(); err != nil {
			return err
		}
	}

	if c, ok := w.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
