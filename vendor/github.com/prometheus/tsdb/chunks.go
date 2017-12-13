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

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/fileutil"
)

const (
	// MagicChunks is 4 bytes at the head of a series file.
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

// writeHash writes the chunk encoding and raw data into the provided hash.
func (cm *ChunkMeta) writeHash(h hash.Hash) error {
	if _, err := h.Write([]byte{byte(cm.Chunk.Encoding())}); err != nil {
		return err
	}
	if _, err := h.Write(cm.Chunk.Bytes()); err != nil {
		return err
	}
	return nil
}

// deletedIterator wraps an Iterator and makes sure any deleted metrics are not
// returned.
type deletedIterator struct {
	it chunks.Iterator

	intervals Intervals
}

func (it *deletedIterator) At() (int64, float64) {
	return it.it.At()
}

func (it *deletedIterator) Next() bool {
Outer:
	for it.it.Next() {
		ts, _ := it.it.At()

		for _, tr := range it.intervals {
			if tr.inBounds(ts) {
				continue Outer
			}

			if ts > tr.Maxt {
				it.intervals = it.intervals[1:]
				continue
			}

			return true
		}

		return true
	}

	return false
}

func (it *deletedIterator) Err() error {
	return it.it.Err()
}

// ChunkWriter serializes a time block of chunked series data.
type ChunkWriter interface {
	// WriteChunks writes several chunks. The Chunk field of the ChunkMetas
	// must be populated.
	// After returning successfully, the Ref fields in the ChunkMetas
	// are set and can be used to retrieve the chunks from the written data.
	WriteChunks(chunks ...ChunkMeta) error

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
		crc32:       newCRC32(),
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
	if err := w.finalizeTail(); err != nil {
		return err
	}

	p, _, err := nextSequenceFile(w.dirFile.Name())
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

func (w *chunkWriter) write(b []byte) error {
	n, err := w.wbuf.Write(b)
	w.n += int64(n)
	return err
}

func (w *chunkWriter) WriteChunks(chks ...ChunkMeta) error {
	// Calculate maximum space we need and cut a new segment in case
	// we don't fit into the current one.
	maxLen := int64(binary.MaxVarintLen32) // The number of chunks.
	for _, c := range chks {
		maxLen += binary.MaxVarintLen32 + 1 // The number of bytes in the chunk and its encoding.
		maxLen += int64(len(c.Chunk.Bytes()))
	}
	newsz := w.n + maxLen

	if w.wbuf == nil || w.n > w.segmentSize || newsz > w.segmentSize && maxLen <= w.segmentSize {
		if err := w.cut(); err != nil {
			return err
		}
	}

	var (
		b   = [binary.MaxVarintLen32]byte{}
		seq = uint64(w.seq()) << 32
	)
	for i := range chks {
		chk := &chks[i]

		chk.Ref = seq | uint64(w.n)

		n := binary.PutUvarint(b[:], uint64(len(chk.Chunk.Bytes())))

		if err := w.write(b[:n]); err != nil {
			return err
		}
		b[0] = byte(chk.Chunk.Encoding())
		if err := w.write(b[:1]); err != nil {
			return err
		}
		if err := w.write(chk.Chunk.Bytes()); err != nil {
			return err
		}

		w.crc32.Reset()
		if err := chk.writeHash(w.crc32); err != nil {
			return err
		}
		if err := w.write(w.crc32.Sum(b[:0])); err != nil {
			return err
		}
	}

	return nil
}

func (w *chunkWriter) seq() int {
	return len(w.files) - 1
}

func (w *chunkWriter) Close() error {
	if err := w.finalizeTail(); err != nil {
		return err
	}

	// close dir file (if not windows platform will fail on rename)
	return w.dirFile.Close()
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
	bs []ByteSlice

	// Closers for resources behind the byte slices.
	cs []io.Closer

	pool chunks.Pool
}

func newChunkReader(bs []ByteSlice, cs []io.Closer, pool chunks.Pool) (*chunkReader, error) {
	cr := chunkReader{pool: pool, bs: bs, cs: cs}

	for i, b := range cr.bs {
		if b.Len() < 4 {
			return nil, errors.Wrapf(errInvalidSize, "validate magic in segment %d", i)
		}
		// Verify magic number.
		if m := binary.BigEndian.Uint32(b.Range(0, 4)); m != MagicChunks {
			return nil, fmt.Errorf("invalid magic number %x", m)
		}
	}
	return &cr, nil
}

// NewChunkReader returns a new chunk reader against the given byte slices.
func NewChunkReader(bs []ByteSlice, pool chunks.Pool) (ChunkReader, error) {
	if pool == nil {
		pool = chunks.NewPool()
	}
	return newChunkReader(bs, nil, pool)
}

// NewDirChunkReader returns a new ChunkReader against sequentially numbered files in the
// given directory.
func NewDirChunkReader(dir string, pool chunks.Pool) (ChunkReader, error) {
	files, err := sequenceFiles(dir)
	if err != nil {
		return nil, err
	}
	if pool == nil {
		pool = chunks.NewPool()
	}

	var bs []ByteSlice
	var cs []io.Closer

	for _, fn := range files {
		f, err := openMmapFile(fn)
		if err != nil {
			return nil, errors.Wrapf(err, "mmap files")
		}
		cs = append(cs, f)
		bs = append(bs, realByteSlice(f.b))
	}
	return newChunkReader(bs, cs, pool)
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

	if int(off) >= b.Len() {
		return nil, errors.Errorf("offset %d beyond data size %d", off, b.Len())
	}
	// With the minimum chunk length this should never cause us reading
	// over the end of the slice.
	r := b.Range(off, off+binary.MaxVarintLen32)

	l, n := binary.Uvarint(r)
	if n < 0 {
		return nil, fmt.Errorf("reading chunk length failed")
	}
	r = b.Range(off+n, off+n+int(l))

	return s.pool.Get(chunks.Encoding(r[0]), r[1:1+l])
}
