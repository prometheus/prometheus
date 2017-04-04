package tsdb

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/chunks"
)

const (
	// MagicChunks is 4 bytes at the head of series file.
	MagicChunks = 0x85BD40DD
)

// ChunkMeta holds information about a chunk of data.
type ChunkMeta struct {
	// Ref and Chunk hold either a reference that can be used to retrieve
	// chunk data or the data itself.
	// Generally, only one of them is set.
	Ref   uint64
	Chunk chunks.Chunk

	MinTime, MaxTime int64 // time range the data covers
}

// ChunkWriter serializes a time block of chunked series data.
type ChunkWriter interface {
	// WriteChunks writes several chunks. The data field of the ChunkMetas
	// must be populated.
	// After returning successfully, the Ref fields in the ChunkMetas
	// is set and can be used to retrieve the chunks from the written data.
	WriteChunks(chunks ...*ChunkMeta) error

	// Close writes any required finalization and closes the resources
	// associated with the underlying writer.
	Close() error
}

// chunkWriter implements the ChunkWriter interface for the standard
// serialization format.
type chunkWriter struct {
	dirFile *os.File
	files   []*os.File
	wbuf    *bufio.Writer
	n       int64
	crc32   hash.Hash

	segmentSize int64
}

const (
	defaultChunkSegmentSize = 512 * 1024 * 1024

	chunksFormatV1 = 1
)

func newChunkWriter(dir string) (*chunkWriter, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	dirFile, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}
	cw := &chunkWriter{
		dirFile:     dirFile,
		n:           0,
		crc32:       crc32.New(crc32.MakeTable(crc32.Castagnoli)),
		segmentSize: defaultChunkSegmentSize,
	}
	return cw, nil
}

func (w *chunkWriter) tail() *os.File {
	if len(w.files) == 0 {
		return nil
	}
	return w.files[len(w.files)-1]
}

// finalizeTail writes all pending data to the current tail file,
// truncates its size, and closes it.
func (w *chunkWriter) finalizeTail() error {
	tf := w.tail()
	if tf == nil {
		return nil
	}

	if err := w.wbuf.Flush(); err != nil {
		return err
	}
	if err := fileutil.Fsync(tf); err != nil {
		return err
	}
	// As the file was pre-allocated, we truncate any superfluous zero bytes.
	off, err := tf.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := tf.Truncate(off); err != nil {
		return err
	}
	return tf.Close()
}

func (w *chunkWriter) cut() error {
	// Sync current tail to disk and close.
	w.finalizeTail()

	p, _, err := nextSequenceFile(w.dirFile.Name(), "")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	if err = fileutil.Preallocate(f, w.segmentSize, true); err != nil {
		return err
	}
	if err = w.dirFile.Sync(); err != nil {
		return err
	}

	// Write header metadata for new file.

	metab := make([]byte, 8)
	binary.BigEndian.PutUint32(metab[:4], MagicChunks)
	metab[4] = chunksFormatV1

	if _, err := f.Write(metab); err != nil {
		return err
	}

	w.files = append(w.files, f)
	if w.wbuf != nil {
		w.wbuf.Reset(f)
	} else {
		w.wbuf = bufio.NewWriterSize(f, 8*1024*1024)
	}
	w.n = 8

	return nil
}

func (w *chunkWriter) write(wr io.Writer, b []byte) error {
	n, err := wr.Write(b)
	w.n += int64(n)
	return err
}

func (w *chunkWriter) WriteChunks(chks ...*ChunkMeta) error {
	// Calculate maximum space we need and cut a new segment in case
	// we don't fit into the current one.
	maxLen := int64(binary.MaxVarintLen32)
	for _, c := range chks {
		maxLen += binary.MaxVarintLen32 + 1
		maxLen += int64(len(c.Chunk.Bytes()))
	}
	newsz := w.n + maxLen

	if w.wbuf == nil || w.n > w.segmentSize || newsz > w.segmentSize && maxLen <= w.segmentSize {
		if err := w.cut(); err != nil {
			return err
		}
	}

	// Write chunks sequentially and set the reference field in the ChunkMeta.
	w.crc32.Reset()
	wr := io.MultiWriter(w.crc32, w.wbuf)

	b := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(b, uint64(len(chks)))

	if err := w.write(wr, b[:n]); err != nil {
		return err
	}
	seq := uint64(w.seq()) << 32

	for _, chk := range chks {
		chk.Ref = seq | uint64(w.n)

		n = binary.PutUvarint(b, uint64(len(chk.Chunk.Bytes())))

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

	if err := w.write(w.wbuf, w.crc32.Sum(nil)); err != nil {
		return err
	}
	return nil
}

func (w *chunkWriter) seq() int {
	return len(w.files) - 1
}

func (w *chunkWriter) Close() error {
	return w.finalizeTail()
}

// ChunkReader provides reading access of serialized time series data.
type ChunkReader interface {
	// Chunk returns the series data chunk with the given reference.
	Chunk(ref uint64) (chunks.Chunk, error)

	// Close releases all underlying resources of the reader.
	Close() error
}

// chunkReader implements a SeriesReader for a serialized byte stream
// of series data.
type chunkReader struct {
	// The underlying bytes holding the encoded series data.
	bs [][]byte

	// Closers for resources behind the byte slices.
	cs []io.Closer
}

// newChunkReader returns a new chunkReader based on mmaped files found in dir.
func newChunkReader(dir string) (*chunkReader, error) {
	files, err := sequenceFiles(dir, "")
	if err != nil {
		return nil, err
	}
	var cr chunkReader

	for _, fn := range files {
		f, err := openMmapFile(fn)
		if err != nil {
			return nil, errors.Wrapf(err, "mmap files")
		}
		cr.cs = append(cr.cs, f)
		cr.bs = append(cr.bs, f.b)
	}

	for i, b := range cr.bs {
		if len(b) < 4 {
			return nil, errors.Wrapf(errInvalidSize, "validate magic in segment %d", i)
		}
		// Verify magic number.
		if m := binary.BigEndian.Uint32(b[:4]); m != MagicChunks {
			return nil, fmt.Errorf("invalid magic number %x", m)
		}
	}
	return &cr, nil
}

func (s *chunkReader) Close() error {
	return closeAll(s.cs...)
}

func (s *chunkReader) Chunk(ref uint64) (chunks.Chunk, error) {
	var (
		seq = int(ref >> 32)
		off = int((ref << 32) >> 32)
	)
	if seq >= len(s.bs) {
		return nil, errors.Errorf("reference sequence %d out of range", seq)
	}
	b := s.bs[seq]

	if int(off) >= len(b) {
		return nil, errors.Errorf("offset %d beyond data size %d", off, len(b))
	}
	b = b[off:]

	l, n := binary.Uvarint(b)
	if n < 0 {
		return nil, fmt.Errorf("reading chunk length failed")
	}
	b = b[n:]
	enc := chunks.Encoding(b[0])

	c, err := chunks.FromData(enc, b[1:1+l])
	if err != nil {
		return nil, err
	}
	return c, nil
}
