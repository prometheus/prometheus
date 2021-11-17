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

package chunks

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// Segment header fields constants.
const (
	// MagicChunks is 4 bytes at the head of a series file.
	MagicChunks = 0x85BD40DD
	// MagicChunksSize is the size in bytes of MagicChunks.
	MagicChunksSize          = 4
	chunksFormatV1           = 1
	ChunksFormatVersionSize  = 1
	segmentHeaderPaddingSize = 3
	// SegmentHeaderSize defines the total size of the header part.
	SegmentHeaderSize = MagicChunksSize + ChunksFormatVersionSize + segmentHeaderPaddingSize
)

// Chunk fields constants.
const (
	// MaxChunkLengthFieldSize defines the maximum size of the data length part.
	MaxChunkLengthFieldSize = binary.MaxVarintLen32
	// ChunkEncodingSize defines the size of the chunk encoding part.
	ChunkEncodingSize = 1
)

// ChunkRef is a generic reference for reading chunk data. In prometheus it
// is either a HeadChunkRef or BlockChunkRef, though other implementations
// may have their own reference types.
type ChunkRef uint64

// HeadSeriesRef refers to in-memory series.
type HeadSeriesRef uint64

// HeadChunkRef packs a HeadSeriesRef and a ChunkID into a global 8 Byte ID.
// The HeadSeriesRef and ChunkID may not exceed 5 and 3 bytes respectively.
type HeadChunkRef uint64

func NewHeadChunkRef(hsr HeadSeriesRef, chunkID HeadChunkID) HeadChunkRef {
	if hsr > (1<<40)-1 {
		panic("series ID exceeds 5 bytes")
	}
	if chunkID > (1<<24)-1 {
		panic("chunk ID exceeds 3 bytes")
	}
	return HeadChunkRef(uint64(hsr<<24) | uint64(chunkID))
}

func (p HeadChunkRef) Unpack() (HeadSeriesRef, HeadChunkID) {
	return HeadSeriesRef(p >> 24), HeadChunkID(p<<40) >> 40
}

// HeadChunkID refers to a specific chunk in a series (memSeries) in the Head.
// Each memSeries has its own monotonically increasing number to refer to its chunks.
// If the HeadChunkID value is...
// * memSeries.firstChunkID+len(memSeries.mmappedChunks), it's the head chunk.
// * less than the above, but >= memSeries.firstID, then it's
//   memSeries.mmappedChunks[i] where i = HeadChunkID - memSeries.firstID.
// Example:
// assume a memSeries.firstChunkID=7 and memSeries.mmappedChunks=[p5,p6,p7,p8,p9].
// | HeadChunkID value | refers to ...                                                                          |
// |-------------------|----------------------------------------------------------------------------------------|
// |               0-6 | chunks that have been compacted to blocks, these won't return data for queries in Head |
// |              7-11 | memSeries.mmappedChunks[i] where i is 0 to 4.                                          |
// |                12 | memSeries.headChunk                                                                    |
type HeadChunkID uint64

// BlockChunkRef refers to a chunk within a persisted block.
// The upper 4 bytes are for the segment index and
// the lower 4 bytes are for the segment offset where the data starts for this chunk.
type BlockChunkRef uint64

// NewBlockChunkRef packs the file index and byte offset into a BlockChunkRef.
func NewBlockChunkRef(fileIndex, fileOffset uint64) BlockChunkRef {
	return BlockChunkRef(fileIndex<<32 | fileOffset)
}

func (b BlockChunkRef) Unpack() (int, int) {
	sgmIndex := int(b >> 32)
	chkStart := int((b << 32) >> 32)
	return sgmIndex, chkStart
}

// Meta holds information about a chunk of data.
type Meta struct {
	// Ref and Chunk hold either a reference that can be used to retrieve
	// chunk data or the data itself.
	// If Chunk is nil, call ChunkReader.Chunk(Meta.Ref) to get the chunk and assign it to the Chunk field
	Ref   ChunkRef
	Chunk chunkenc.Chunk

	// Time range the data covers.
	// When MaxTime == math.MaxInt64 the chunk is still open and being appended to.
	MinTime, MaxTime int64
}

// Iterator iterates over the chunks of a single time series.
type Iterator interface {
	// At returns the current meta.
	// It depends on implementation if the chunk is populated or not.
	At() Meta
	// Next advances the iterator by one.
	Next() bool
	// Err returns optional error if Next is false.
	Err() error
}

// writeHash writes the chunk encoding and raw data into the provided hash.
func (cm *Meta) writeHash(h hash.Hash, buf []byte) error {
	buf = append(buf[:0], byte(cm.Chunk.Encoding()))
	if _, err := h.Write(buf[:1]); err != nil {
		return err
	}
	if _, err := h.Write(cm.Chunk.Bytes()); err != nil {
		return err
	}
	return nil
}

// OverlapsClosedInterval Returns true if the chunk overlaps [mint, maxt].
func (cm *Meta) OverlapsClosedInterval(mint, maxt int64) bool {
	// The chunk itself is a closed interval [cm.MinTime, cm.MaxTime].
	return cm.MinTime <= maxt && mint <= cm.MaxTime
}

var errInvalidSize = fmt.Errorf("invalid size")

var castagnoliTable *crc32.Table

func init() {
	castagnoliTable = crc32.MakeTable(crc32.Castagnoli)
}

// newCRC32 initializes a CRC32 hash with a preconfigured polynomial, so the
// polynomial may be easily changed in one location at a later time, if necessary.
func newCRC32() hash.Hash32 {
	return crc32.New(castagnoliTable)
}

// Writer implements the ChunkWriter interface for the standard
// serialization format.
type Writer struct {
	dirFile *os.File
	files   []*os.File
	wbuf    *bufio.Writer
	n       int64
	crc32   hash.Hash
	buf     [binary.MaxVarintLen32]byte

	segmentSize int64
}

const (
	// DefaultChunkSegmentSize is the default chunks segment size.
	DefaultChunkSegmentSize = 512 * 1024 * 1024
)

// NewWriterWithSegSize returns a new writer against the given directory
// and allows setting a custom size for the segments.
func NewWriterWithSegSize(dir string, segmentSize int64) (*Writer, error) {
	return newWriter(dir, segmentSize)
}

// NewWriter returns a new writer against the given directory
// using the default segment size.
func NewWriter(dir string) (*Writer, error) {
	return newWriter(dir, DefaultChunkSegmentSize)
}

func newWriter(dir string, segmentSize int64) (*Writer, error) {
	if segmentSize <= 0 {
		segmentSize = DefaultChunkSegmentSize
	}

	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, err
	}
	dirFile, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}
	return &Writer{
		dirFile:     dirFile,
		n:           0,
		crc32:       newCRC32(),
		segmentSize: segmentSize,
	}, nil
}

func (w *Writer) tail() *os.File {
	if len(w.files) == 0 {
		return nil
	}
	return w.files[len(w.files)-1]
}

// finalizeTail writes all pending data to the current tail file,
// truncates its size, and closes it.
func (w *Writer) finalizeTail() error {
	tf := w.tail()
	if tf == nil {
		return nil
	}

	if err := w.wbuf.Flush(); err != nil {
		return err
	}
	if err := tf.Sync(); err != nil {
		return err
	}
	// As the file was pre-allocated, we truncate any superfluous zero bytes.
	off, err := tf.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if err := tf.Truncate(off); err != nil {
		return err
	}

	return tf.Close()
}

func (w *Writer) cut() error {
	// Sync current tail to disk and close.
	if err := w.finalizeTail(); err != nil {
		return err
	}

	n, f, _, err := cutSegmentFile(w.dirFile, MagicChunks, chunksFormatV1, w.segmentSize)
	if err != nil {
		return err
	}
	w.n = int64(n)

	w.files = append(w.files, f)
	if w.wbuf != nil {
		w.wbuf.Reset(f)
	} else {
		w.wbuf = bufio.NewWriterSize(f, 8*1024*1024)
	}

	return nil
}

func cutSegmentFile(dirFile *os.File, magicNumber uint32, chunksFormat byte, allocSize int64) (headerSize int, newFile *os.File, seq int, returnErr error) {
	p, seq, err := nextSequenceFile(dirFile.Name())
	if err != nil {
		return 0, nil, 0, errors.Wrap(err, "next sequence file")
	}
	ptmp := p + ".tmp"
	f, err := os.OpenFile(ptmp, os.O_WRONLY|os.O_CREATE, 0o666)
	if err != nil {
		return 0, nil, 0, errors.Wrap(err, "open temp file")
	}
	defer func() {
		if returnErr != nil {
			errs := tsdb_errors.NewMulti(returnErr)
			if f != nil {
				errs.Add(f.Close())
			}
			// Calling RemoveAll on a non-existent file does not return error.
			errs.Add(os.RemoveAll(ptmp))
			returnErr = errs.Err()
		}
	}()
	if allocSize > 0 {
		if err = fileutil.Preallocate(f, allocSize, true); err != nil {
			return 0, nil, 0, errors.Wrap(err, "preallocate")
		}
	}
	if err = dirFile.Sync(); err != nil {
		return 0, nil, 0, errors.Wrap(err, "sync directory")
	}

	// Write header metadata for new file.
	metab := make([]byte, SegmentHeaderSize)
	binary.BigEndian.PutUint32(metab[:MagicChunksSize], magicNumber)
	metab[4] = chunksFormat

	n, err := f.Write(metab)
	if err != nil {
		return 0, nil, 0, errors.Wrap(err, "write header")
	}
	if err := f.Close(); err != nil {
		return 0, nil, 0, errors.Wrap(err, "close temp file")
	}
	f = nil

	if err := fileutil.Rename(ptmp, p); err != nil {
		return 0, nil, 0, errors.Wrap(err, "replace file")
	}

	f, err = os.OpenFile(p, os.O_WRONLY, 0o666)
	if err != nil {
		return 0, nil, 0, errors.Wrap(err, "open final file")
	}
	// Skip header for further writes.
	if _, err := f.Seek(int64(n), 0); err != nil {
		return 0, nil, 0, errors.Wrap(err, "seek in final file")
	}
	return n, f, seq, nil
}

func (w *Writer) write(b []byte) error {
	n, err := w.wbuf.Write(b)
	w.n += int64(n)
	return err
}

// WriteChunks writes as many chunks as possible to the current segment,
// cuts a new segment when the current segment is full and
// writes the rest of the chunks in the new segment.
func (w *Writer) WriteChunks(chks ...Meta) error {
	var (
		batchSize  = int64(0)
		batchStart = 0
		batches    = make([][]Meta, 1)
		batchID    = 0
		firstBatch = true
	)

	for i, chk := range chks {
		// Each chunk contains: data length + encoding + the data itself + crc32
		chkSize := int64(MaxChunkLengthFieldSize) // The data length is a variable length field so use the maximum possible value.
		chkSize += ChunkEncodingSize              // The chunk encoding.
		chkSize += int64(len(chk.Chunk.Bytes()))  // The data itself.
		chkSize += crc32.Size                     // The 4 bytes of crc32.
		batchSize += chkSize

		// Cut a new batch when it is not the first chunk(to avoid empty segments) and
		// the batch is too large to fit in the current segment.
		cutNewBatch := (i != 0) && (batchSize+SegmentHeaderSize > w.segmentSize)

		// When the segment already has some data than
		// the first batch size calculation should account for that.
		if firstBatch && w.n > SegmentHeaderSize {
			cutNewBatch = batchSize+w.n > w.segmentSize
			if cutNewBatch {
				firstBatch = false
			}
		}

		if cutNewBatch {
			batchStart = i
			batches = append(batches, []Meta{})
			batchID++
			batchSize = chkSize
		}
		batches[batchID] = chks[batchStart : i+1]
	}

	// Create a new segment when one doesn't already exist.
	if w.n == 0 {
		if err := w.cut(); err != nil {
			return err
		}
	}

	for i, chks := range batches {
		if err := w.writeChunks(chks); err != nil {
			return err
		}
		// Cut a new segment only when there are more chunks to write.
		// Avoid creating a new empty segment at the end of the write.
		if i < len(batches)-1 {
			if err := w.cut(); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeChunks writes the chunks into the current segment irrespective
// of the configured segment size limit. A segment should have been already
// started before calling this.
func (w *Writer) writeChunks(chks []Meta) error {
	if len(chks) == 0 {
		return nil
	}

	seq := uint64(w.seq())
	for i := range chks {
		chk := &chks[i]

		chk.Ref = ChunkRef(NewBlockChunkRef(seq, uint64(w.n)))

		n := binary.PutUvarint(w.buf[:], uint64(len(chk.Chunk.Bytes())))

		if err := w.write(w.buf[:n]); err != nil {
			return err
		}
		w.buf[0] = byte(chk.Chunk.Encoding())
		if err := w.write(w.buf[:1]); err != nil {
			return err
		}
		if err := w.write(chk.Chunk.Bytes()); err != nil {
			return err
		}

		w.crc32.Reset()
		if err := chk.writeHash(w.crc32, w.buf[:]); err != nil {
			return err
		}
		if err := w.write(w.crc32.Sum(w.buf[:0])); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) seq() int {
	return len(w.files) - 1
}

func (w *Writer) Close() error {
	if err := w.finalizeTail(); err != nil {
		return err
	}

	// close dir file (if not windows platform will fail on rename)
	return w.dirFile.Close()
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

// Reader implements a ChunkReader for a serialized byte stream
// of series data.
type Reader struct {
	// The underlying bytes holding the encoded series data.
	// Each slice holds the data for a different segment.
	bs   []ByteSlice
	cs   []io.Closer // Closers for resources behind the byte slices.
	size int64       // The total size of bytes in the reader.
	pool chunkenc.Pool
}

func newReader(bs []ByteSlice, cs []io.Closer, pool chunkenc.Pool) (*Reader, error) {
	cr := Reader{pool: pool, bs: bs, cs: cs}
	for i, b := range cr.bs {
		if b.Len() < SegmentHeaderSize {
			return nil, errors.Wrapf(errInvalidSize, "invalid segment header in segment %d", i)
		}
		// Verify magic number.
		if m := binary.BigEndian.Uint32(b.Range(0, MagicChunksSize)); m != MagicChunks {
			return nil, errors.Errorf("invalid magic number %x", m)
		}

		// Verify chunk format version.
		if v := int(b.Range(MagicChunksSize, MagicChunksSize+ChunksFormatVersionSize)[0]); v != chunksFormatV1 {
			return nil, errors.Errorf("invalid chunk format version %d", v)
		}
		cr.size += int64(b.Len())
	}
	return &cr, nil
}

// NewDirReader returns a new Reader against sequentially numbered files in the
// given directory.
func NewDirReader(dir string, pool chunkenc.Pool) (*Reader, error) {
	files, err := sequenceFiles(dir)
	if err != nil {
		return nil, err
	}
	if pool == nil {
		pool = chunkenc.NewPool()
	}

	var (
		bs []ByteSlice
		cs []io.Closer
	)
	for _, fn := range files {
		f, err := fileutil.OpenMmapFile(fn)
		if err != nil {
			return nil, tsdb_errors.NewMulti(
				errors.Wrap(err, "mmap files"),
				tsdb_errors.CloseAll(cs),
			).Err()
		}
		cs = append(cs, f)
		bs = append(bs, realByteSlice(f.Bytes()))
	}

	reader, err := newReader(bs, cs, pool)
	if err != nil {
		return nil, tsdb_errors.NewMulti(
			err,
			tsdb_errors.CloseAll(cs),
		).Err()
	}
	return reader, nil
}

func (s *Reader) Close() error {
	return tsdb_errors.CloseAll(s.cs)
}

// Size returns the size of the chunks.
func (s *Reader) Size() int64 {
	return s.size
}

// Chunk returns a chunk from a given reference.
func (s *Reader) Chunk(ref ChunkRef) (chunkenc.Chunk, error) {
	sgmIndex, chkStart := BlockChunkRef(ref).Unpack()
	chkCRC32 := newCRC32()

	if sgmIndex >= len(s.bs) {
		return nil, errors.Errorf("segment index %d out of range", sgmIndex)
	}

	sgmBytes := s.bs[sgmIndex]

	if chkStart+MaxChunkLengthFieldSize > sgmBytes.Len() {
		return nil, errors.Errorf("segment doesn't include enough bytes to read the chunk size data field - required:%v, available:%v", chkStart+MaxChunkLengthFieldSize, sgmBytes.Len())
	}
	// With the minimum chunk length this should never cause us reading
	// over the end of the slice.
	c := sgmBytes.Range(chkStart, chkStart+MaxChunkLengthFieldSize)
	chkDataLen, n := binary.Uvarint(c)
	if n <= 0 {
		return nil, errors.Errorf("reading chunk length failed with %d", n)
	}

	chkEncStart := chkStart + n
	chkEnd := chkEncStart + ChunkEncodingSize + int(chkDataLen) + crc32.Size
	chkDataStart := chkEncStart + ChunkEncodingSize
	chkDataEnd := chkEnd - crc32.Size

	if chkEnd > sgmBytes.Len() {
		return nil, errors.Errorf("segment doesn't include enough bytes to read the chunk - required:%v, available:%v", chkEnd, sgmBytes.Len())
	}

	sum := sgmBytes.Range(chkDataEnd, chkEnd)
	if _, err := chkCRC32.Write(sgmBytes.Range(chkEncStart, chkDataEnd)); err != nil {
		return nil, err
	}

	if act := chkCRC32.Sum(nil); !bytes.Equal(act, sum) {
		return nil, errors.Errorf("checksum mismatch expected:%x, actual:%x", sum, act)
	}

	chkData := sgmBytes.Range(chkDataStart, chkDataEnd)
	chkEnc := sgmBytes.Range(chkEncStart, chkEncStart+ChunkEncodingSize)[0]
	return s.pool.Get(chunkenc.Encoding(chkEnc), chkData)
}

func nextSequenceFile(dir string) (string, int, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", 0, err
	}

	i := uint64(0)
	for _, f := range files {
		j, err := strconv.ParseUint(f.Name(), 10, 64)
		if err != nil {
			continue
		}
		// It is not necessary that we find the files in number order,
		// for example with '1000000' and '200000', '1000000' would come first.
		// Though this is a very very race case, we check anyway for the max id.
		if j > i {
			i = j
		}
	}
	return segmentFile(dir, int(i+1)), int(i + 1), nil
}

func segmentFile(baseDir string, index int) string {
	return filepath.Join(baseDir, fmt.Sprintf("%0.6d", index))
}

func sequenceFiles(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var res []string
	for _, fi := range files {
		if _, err := strconv.ParseUint(fi.Name(), 10, 64); err != nil {
			continue
		}
		res = append(res, filepath.Join(dir, fi.Name()))
	}
	return res, nil
}
