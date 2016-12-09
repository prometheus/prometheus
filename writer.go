package tsdb

import (
	"hash/crc32"
	"io"
	"os"
	"unsafe"
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
	WriteSeries(Labels, []*chunkDesc) error

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

	chunkOffsets  map[uint32][]uint32
	seriesOffsets map[uint32]uint32
}

func newSeriesWriter(w io.Writer, base int64) *seriesWriter {
	return &seriesWriter{
		w:             w,
		n:             0,
		baseTimestamp: base,
	}
}

func (w *seriesWriter) write(wr io.Writer, b []byte) error {
	n, err := wr.Write(b)
	w.n += int64(n)
	return err
}

func (w *seriesWriter) writeMeta() error {
	meta := &meta{magic: MagicSeries, flag: flagStd}
	metab := ((*[metaSize]byte)(unsafe.Pointer(meta)))[:]

	return w.write(w.w, metab)
}

func (w *seriesWriter) WriteSeries(lset Labels, chks []*chunkDesc) error {
	// Initialize with meta data.
	if w.n == 0 {
		if err := w.writeMeta(); err != nil {
			return err
		}
	}

	// TODO(fabxc): is crc32 enough for chunks of one series?
	h := crc32.NewIEEE()
	wr := io.MultiWriter(h, w.w)

	l := uint32(0)
	for _, cd := range chks {
		l += uint32(len(cd.chunk.Bytes()))
	}
	// For normal reads we don't need the length of the chunk section but
	// it allows us to verify checksums without reading the index file.
	if err := w.write(w.w, ((*[4]byte)(unsafe.Pointer(&l)))[:]); err != nil {
		return err
	}

	offsets := make([]ChunkOffset, 0, len(chks))
	lastTimestamp := w.baseTimestamp

	for _, cd := range chks {
		offsets = append(offsets, ChunkOffset{
			Value:  lastTimestamp,
			Offset: uint32(w.n),
		})

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
		w.index.AddOffsets(lset, offsets...)
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
	// AddOffsets populates the index writer with offsets of chunks
	// for a series that the index can reference.
	AddOffsets(Labels, ...ChunkOffset)

	// WriteStats writes final stats for the indexed block.
	WriteStats(*BlockStats) error

	// WriteSymbols serializes all encountered string symbols.
	WriteSymbols([]string) error

	// WriteLabelIndex serializes an index from label names to values.
	// The passed in values chained tuples of strings of the length of names.
	WriteLabelIndex(names []string, values []string) error

	// WritesSeries serializes series identifying labels.
	WriteSeries(ref uint32, ls ...Labels) error

	// WritePostings writes a postings list for a single label pair.
	WritePostings(name, value string, it Iterator) error

	// Size returns the size of the data written so far.
	Size() int64

	// Closes writes any finalization and closes theresources associated with
	// the underlying writer.
	Close() error
}

// indexWriter implements the IndexWriter interface for the standard
// serialization format.
type indexWriter struct {
	w io.Writer
	n int64

	series  []Labels
	offsets [][]ChunkOffset
}

func (w *indexWriter) AddOffsets(lset Labels, offsets ...ChunkOffset) {
	w.series = append(w.series, lset)
	w.offsets = append(w.offsets, offsets)
}

func (w *indexWriter) WriteStats(*BlockStats) error {
	return nil
}

func (w *indexWriter) WriteSymbols(symbols []string) error {
	return nil
}

func (w *indexWriter) WriteLabelIndex(names []string, values []string) error {
	return nil
}

func (w *indexWriter) WriteSeries(ref uint32, ls ...Labels) error {
	return nil
}

func (w *indexWriter) WritePostings(name, value string, it Iterator) error {
	return nil
}

func (w *indexWriter) Size() int64 {
	return w.n
}
func (w *indexWriter) Close() error {
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
