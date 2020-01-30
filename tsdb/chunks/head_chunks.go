// Copyright 2020 The Prometheus Authors
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
	"encoding/binary"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// Segment header fields constants.
const (
	headChunksFormatV1 = 1
	inBufferShards     = 16
	writeBufferSize    = 2 * 1024 * 1024
)

var (
	// ErrHeadReadWriterClosed returned by any method indicates
	// that the HeadReadWriter was closed.
	ErrHeadReadWriterClosed = errors.New("HeadReadWriter closed")
)

// HeadReadWriter is for writing the Head block chunks to the disk
// and access chunks via an mmapped file.
type HeadReadWriter struct {
	// Writer.
	dirFile *os.File
	curFile *os.File // Segment file being written to.

	curFileMint     int64 // In milliseconds.
	curFileMaxt     int64 // In milliseconds.
	curFileSequence int
	n               int64 // Bytes written in current segment.
	segmentTime     int64 // Duration in milliseconds.

	buf                 [8]byte
	wbuf                *bufio.Writer
	wbufLock            sync.RWMutex
	inBufferChunks      [inBufferShards]map[uint64]chunkenc.Chunk
	inBufferChunksLocks [inBufferShards]sync.RWMutex

	// Reader
	bs map[int]ByteSlice
	cs map[int]io.Closer // Closers for resources behind the byte slices.

	size  int64 // The total size of bytes in the reader.
	bsMtx sync.RWMutex
	pool  chunkenc.Pool

	quit chan struct{}
}

const (
	// DefaultHeadChunkSegmentTime is the default chunks segment time range.
	// Assuming a general scrape interval of 15s, a chunk with 120 samples would
	// be cut every 30m, so anything <30m will cause lots of empty files. And keeping
	// it exactly 30m also has a chance of having empty files as its near that border.
	// Hence keeping it a little more than 30m, i.e. 40m.
	DefaultHeadChunkSegmentTime = 40 * time.Minute
	// HeadSegmentHeaderSize is the total size of the header for the segment file.
	HeadSegmentHeaderSize = SegmentHeaderSize + 16
	// HeaderMintOffset is the offset where the first byte of MinT for segment file exists.
	HeaderMintOffset = SegmentHeaderSize
	// HeaderMaxtOffset is the offset where the first byte of MaxT for segment file exists.
	HeaderMaxtOffset = HeaderMintOffset + 8
	// MaxSegmentSize is the max size of a segment file.
	// Setting size to the max int32 as setting it to max int 64 crashes 64 systems too.
	MaxSegmentSize = math.MaxInt32
)

// NewHeadReadWriter returns a new writer against the given directory
// using the default segment time.
func NewHeadReadWriter(dir string, pool chunkenc.Pool) (*HeadReadWriter, error) {
	return newHeadReadWriter(dir, DefaultHeadChunkSegmentTime, pool)
}

func newHeadReadWriter(dir string, segmentTime time.Duration, pool chunkenc.Pool) (*HeadReadWriter, error) {
	if segmentTime <= 0 {
		segmentTime = DefaultHeadChunkSegmentTime
	}

	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	dirFile, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}

	hrw := &HeadReadWriter{
		dirFile:     dirFile,
		n:           0,
		segmentTime: segmentTime.Milliseconds(),
		pool:        pool,
		quit:        make(chan struct{}),
	}

	for i := 0; i < inBufferShards; i++ {
		hrw.inBufferChunks[i] = make(map[uint64]chunkenc.Chunk)
	}

	if err := hrw.initReader(); err != nil {
		return nil, err
	}

	return hrw, nil
}

func (w *HeadReadWriter) initReader() (err error) {
	bs := map[int]ByteSlice{}
	cs := map[int]io.Closer{}
	defer func() {
		if err != nil {
			var merr tsdb_errors.MultiError
			merr.Add(err)
			merr.Add(closeAllFromMap(cs))
			err = merr
		}
	}()

	files, err := sequenceFilesMap(w.dirFile.Name())
	if err != nil {
		return err
	}
	if w.pool == nil {
		w.pool = chunkenc.NewPool()
	}

	seqs := make([]int, 0, len(files))
	for seq, fn := range files {
		f, err := fileutil.OpenMmapFile(fn)
		if err != nil {
			return errors.Wrap(err, "mmap files")
		}
		cs[seq] = f
		bs[seq] = realByteSlice(f.Bytes())
		seqs = append(seqs, seq)
	}

	w.bs = bs
	w.cs = cs
	w.size = 0

	// Check for unsequential files.
	sort.Ints(seqs)
	if len(seqs) == 0 {
		return nil
	}
	lastSeq := seqs[0]
	for _, seq := range seqs[1:] {
		if seq != lastSeq+1 {
			return errors.Errorf("found unsequential segment files %d and %d", lastSeq, seq)
		}
		lastSeq = seq
	}

	for i, b := range w.bs {
		if b.Len() < HeadSegmentHeaderSize {
			return errors.Wrapf(errInvalidSize, "invalid segment header in segment %d", i)
		}
		// Verify magic number.
		if m := binary.BigEndian.Uint32(b.Range(0, MagicChunksSize)); m != MagicChunks {
			return errors.Errorf("invalid magic number %x", m)
		}

		// Verify chunk format version.
		if v := int(b.Range(MagicChunksSize, MagicChunksSize+ChunksFormatVersionSize)[0]); v != chunksFormatV1 {
			return errors.Errorf("invalid chunk format version %d", v)
		}

		maxt := binary.BigEndian.Uint64(b.Range(HeaderMaxtOffset, HeaderMaxtOffset+8))
		if maxt == 0 {
			// This is possible if Prometheus crashes and the maxt was unwritten to the
			// last segment. As a safe buffer, we set the maxt to mint + 1.5 times segment time range.
			f, err := os.OpenFile(files[i], os.O_WRONLY|os.O_CREATE, 0666)
			if err != nil {
				return err
			}
			defer f.Close()
			mint := binary.BigEndian.Uint64(b.Range(HeaderMintOffset, HeaderMintOffset+8))
			binary.BigEndian.PutUint64(w.buf[:], mint+(uint64(w.segmentTime)*3/2))
			if _, err := f.WriteAt(w.buf[:8], HeaderMaxtOffset); err != nil {
				return err
			}
		}

		w.size += int64(b.Len())
	}

	return nil
}

func sequenceFilesMap(dir string) (map[int]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	res := map[int]string{}
	for _, fi := range files {
		seq, err := strconv.ParseUint(fi.Name(), 10, 64)
		if err != nil {
			continue
		}
		res[int(seq)] = filepath.Join(dir, fi.Name())
	}
	return res, nil
}

func closeAllFromMap(cs map[int]io.Closer) error {
	var merr tsdb_errors.MultiError

	for _, c := range cs {
		merr.Add(c.Close())
	}
	return merr.Err()
}

// IterateAllChunks iterates on all the chunks in its byte slices in the order of the segment file sequence
// and runs the provided function on each chunk. It returns on the first error encountered.
func (w *HeadReadWriter) IterateAllChunks(f func(seriesRef, chunkRef uint64, mint, maxt int64) error) error {
	// Iterate files in ascending order.
	seqs := make([]int, 0, len(w.bs))
	for seg := range w.bs {
		seqs = append(seqs, seg)
	}
	sort.Ints(seqs)
	for _, seq := range seqs {
		bs := w.bs[seq]
		sliceLen := bs.Len()
		idx := HeadSegmentHeaderSize
		for idx < sliceLen {
			if sliceLen-idx < 30 {
				return errors.Errorf("segment doesn't include enough bytes to read the chunk header - required:%v, available:%v", idx+25, sliceLen)
			}

			seriesRef := binary.BigEndian.Uint64(bs.Range(idx, idx+8))
			idx += 8

			mint := int64(binary.BigEndian.Uint64(bs.Range(idx, idx+8)))
			idx += 8

			maxt := int64(binary.BigEndian.Uint64(bs.Range(idx, idx+8)))
			idx += 8

			chunkRef := uint64(seq)<<32 | uint64(idx)
			if err := f(seriesRef, chunkRef, mint, maxt); err != nil {
				return err
			}

			idx++ // Skip encoding.
			// Skip the data.
			dataLen, n := binary.Uvarint(bs.Range(idx, idx+MaxChunkLengthFieldSize))
			idx += n + int(dataLen)
		}
	}

	return nil
}

// WriteChunk writes the chunk to the disk.
// The returned chunk ref is the reference from where the chunk encoding starts for the chunk.
func (w *HeadReadWriter) WriteChunk(seriesRef uint64, mint, maxt int64, chk chunkenc.Chunk) (chunkRef uint64, err error) {
	w.wbufLock.Lock()
	defer w.wbufLock.Unlock()

	select {
	case <-w.quit:
		return 0, ErrHeadReadWriterClosed
	default:
	}

	if w.shouldCutSegment(len(chk.Bytes()), maxt) {
		if err := w.cut(mint); err != nil {
			return 0, err
		}
		w.curFileMaxt = 0
	}

	// seriesRef(8) + mint(8) + maxt(8) + encoding(1) + max uvarint size (5) + the bytes is
	// the total size to be written.
	// if len(chk.Bytes()) >= writeBufferSize-30, it means that chunk >= the buffer size;
	// so no need to flush here, as we have to flush at the end (to not keep partial chunks in buffer).
	if len(chk.Bytes()) < writeBufferSize-30 && w.wbuf.Available() < 8+8+8+1+5+len(chk.Bytes()) {
		if err := w.flushBuffer(); err != nil {
			return 0, err
		}
	}

	binary.BigEndian.PutUint64(w.buf[:], seriesRef)
	if err := w.write(w.buf[:8]); err != nil {
		return 0, err
	}

	binary.BigEndian.PutUint64(w.buf[:], uint64(mint))
	if err := w.write(w.buf[:8]); err != nil {
		return 0, err
	}

	binary.BigEndian.PutUint64(w.buf[:], uint64(maxt))
	if err := w.write(w.buf[:8]); err != nil {
		return 0, err
	}

	// The reference is set to the segment index and the offset where
	// the data starts for this chunk.
	//
	// The upper 4 bytes are for the segment index and
	// the lower 4 bytes are for the segment offset where to start reading this chunk.
	chunkRef = w.chunkRef(uint64(w.seq()), uint64(w.n))

	w.buf[0] = byte(chk.Encoding())
	if err := w.write(w.buf[:1]); err != nil {
		return 0, err
	}

	n := binary.PutUvarint(w.buf[:], uint64(len(chk.Bytes())))
	if err := w.write(w.buf[:n]); err != nil {
		return 0, err
	}

	if err := w.write(chk.Bytes()); err != nil {
		return 0, err
	}

	if maxt > w.curFileMaxt {
		w.curFileMaxt = maxt
	}

	if mint < w.curFileMint {
		w.curFileMint = mint
		// As this wont happen a whole lot of time, we don't wait
		// for a new segment to be cut to write it off.
		binary.BigEndian.PutUint64(w.buf[:], uint64(mint))
		if _, err := w.curFile.WriteAt(w.buf[:8], HeaderMintOffset); err != nil {
			return 0, err
		}
	}

	w.inBufferChunks[chunkRef%inBufferShards][chunkRef] = chk

	if len(chk.Bytes()) >= writeBufferSize-30 {
		if err := w.flushBuffer(); err != nil {
			return 0, err
		}
	}

	return chunkRef, nil
}

// Chunk returns a chunk from a given reference.
func (w *HeadReadWriter) Chunk(ref uint64) (chunkenc.Chunk, error) {
	var (
		// Get the upper 4 bytes.
		// These contain the segment index.
		sgmIndex = int(ref >> 32)
		// Get the lower 4 bytes.
		// These contain the segment offset where the data for this chunk starts.
		chkStart = int((ref << 32) >> 32)
	)

	// TODO(codesome): take care of possible race with w.wbuf.
	if sgmIndex == w.curFileSequence && chkStart > int(w.n)-w.wbuf.Buffered() {
		chunk := w.getChunkFromBuffer(ref)
		if chunk != nil {
			return chunk, nil
		}
	}

	w.bsMtx.RLock()
	// We hold this read lock for the entire duration because if the Close()
	// is called, the data in the byte slice will get corrupted as the mmapped
	// file will be closed.
	defer w.bsMtx.RUnlock()

	select {
	case <-w.quit:
		return nil, ErrHeadReadWriterClosed
	default:
	}

	sgmBytes, ok := w.bs[sgmIndex]
	if !ok {
		if sgmIndex > w.curFileSequence {
			return nil, errors.Errorf("segment index %d more than current segment", sgmIndex)
		}
		return nil, errors.Errorf("segment index %d does not exist on disk", sgmIndex)
	}

	if chkStart+MaxChunkLengthFieldSize > sgmBytes.Len() {
		return nil, errors.Errorf("segment doesn't include enough bytes to read the chunk size data field - required:%v, available:%v", chkStart+MaxChunkLengthFieldSize, sgmBytes.Len())
	}

	// Encoding.
	chkEnc := sgmBytes.Range(chkStart, chkStart+ChunkEncodingSize)[0]

	// Data length.
	// With the minimum chunk length this should never cause us reading
	// over the end of the slice.
	chkDataLenStart := chkStart + ChunkEncodingSize
	c := sgmBytes.Range(chkDataLenStart, chkDataLenStart+MaxChunkLengthFieldSize)
	chkDataLen, n := binary.Uvarint(c)
	if n <= 0 {
		return nil, errors.Errorf("reading chunk length failed with %d", n)
	}

	// Data itself.
	chkEnd := chkDataLenStart + n + int(chkDataLen)
	if chkEnd > sgmBytes.Len() {
		return nil, errors.Errorf("segment doesn't include enough bytes to read the chunk - required:%v, available:%v", chkEnd, sgmBytes.Len())
	}
	chkData := sgmBytes.Range(chkEnd-int(chkDataLen), chkEnd)

	return w.pool.Get(chunkenc.Encoding(chkEnc), chkData)
}

// Truncate deletes the segment files which are strictly below the mint.
// mint should be in milliseconds.
func (w *HeadReadWriter) Truncate(mint int64) error {
	var removedFiles []int

	w.bsMtx.RLock()
	for seq, bs := range w.bs {
		if seq == w.curFileSequence {
			continue
		}
		b := bs.Range(HeaderMaxtOffset, HeaderMaxtOffset+8)
		maxt := binary.BigEndian.Uint64(b)
		if int64(maxt) < mint {
			removedFiles = append(removedFiles, seq)
		}
	}
	w.bsMtx.RUnlock()

	closers := make([]io.Closer, 0, len(removedFiles))
	w.bsMtx.Lock()
	for _, seq := range removedFiles {
		w.size -= int64(w.bs[seq].Len())
		closers = append(closers, w.cs[seq])
		delete(w.bs, seq)
		delete(w.cs, seq)
		if err := os.Remove(segmentFile(w.dirFile.Name(), seq)); err != nil {
			w.bsMtx.Unlock()
			return err
		}
	}
	w.bsMtx.Unlock()

	return closeAll(closers)
}

func (w *HeadReadWriter) Close() error {
	// 'WriteChunk' locks wbufLock first and then bsMtx for cutting segment.
	// The lock order should not be reversed here else it can cause deadlocks.
	w.wbufLock.Lock()
	defer w.wbufLock.Unlock()
	w.bsMtx.Lock()
	defer w.bsMtx.Unlock()

	select {
	case <-w.quit:
		return nil
	default:
	}

	close(w.quit)

	if err := w.finalizeCurFile(); err != nil {
		return err
	}

	if err := w.dirFile.Close(); err != nil {
		return err
	}

	return closeAllFromMap(w.cs)
}

func (w *HeadReadWriter) flushBuffer() error {
	// Assumes that wbufLock is locked before calling this method.
	if err := w.wbuf.Flush(); err != nil {
		return nil
	}
	for i := 0; i < inBufferShards; i++ {
		w.inBufferChunksLocks[i].Lock()
	}
	for i := 0; i < inBufferShards; i++ {
		w.inBufferChunks[i] = make(map[uint64]chunkenc.Chunk)
	}
	for i := inBufferShards - 1; i >= 0; i-- {
		w.inBufferChunksLocks[i].Unlock()
	}
	return nil
}

func (w *HeadReadWriter) getChunkFromBuffer(chunkRef uint64) chunkenc.Chunk {
	shardIdx := chunkRef % inBufferShards

	w.inBufferChunksLocks[shardIdx].Lock()
	defer w.inBufferChunksLocks[shardIdx].Unlock()

	return w.inBufferChunks[shardIdx][chunkRef]
}

func (w *HeadReadWriter) chunkRef(seq, offset uint64) (chunkRef uint64) {
	return (seq << 32) | offset
}

func (w *HeadReadWriter) shouldCutSegment(chunkLength int, maxt int64) bool {
	return w.n == 0 || // First segment
		// TODO: tune this boolean, cutting a segment for only 1 chunk would be inefficient.
		(maxt-w.curFileMint > w.segmentTime && w.n > HeadSegmentHeaderSize) || // Time duration reached for the existing file.
		w.n+int64(chunkLength+27) >= MaxSegmentSize
}

func (w *HeadReadWriter) seq() int {
	return w.curFileSequence
}

func (w *HeadReadWriter) write(b []byte) error {
	n, err := w.wbuf.Write(b)
	w.n += int64(n)
	return err
}

func (w *HeadReadWriter) cut(mint int64) (returnErr error) {
	// Sync current tail to disk and close.
	if err := w.finalizeCurFile(); err != nil {
		return err
	}

	n, f, seq, err := cutSegmentFile(w.dirFile, headChunksFormatV1, 0)
	if err != nil {
		return err
	}
	defer func() {
		// The file should not be closed if there is no error,
		// its kept open in the HeadReadWriter.
		if returnErr != nil {
			f.Close()
		}
	}()

	w.size += w.n
	w.n = int64(n)
	oldSeq := w.curFileSequence
	w.curFileSequence = seq

	// Write current time in milliseconds for mint and 0 for maxt.
	w.curFileMint = mint
	binary.BigEndian.PutUint64(w.buf[:], uint64(mint))
	if _, err := f.Write(w.buf[:8]); err != nil {
		return err
	}
	w.n += 8
	binary.BigEndian.PutUint64(w.buf[:], 0)
	if _, err := f.Write(w.buf[:8]); err != nil {
		return err
	}
	w.n += 8

	oldFile := w.curFile

	w.curFile = f
	if w.wbuf != nil {
		w.wbuf.Reset(f)
	} else {
		w.wbuf = bufio.NewWriterSize(f, writeBufferSize)
	}

	if oldFile != nil {
		// Open it again with the new size.
		newTailFile, err := fileutil.OpenMmapFile(oldFile.Name())
		if err != nil {
			return err
		}
		w.bsMtx.Lock()
		// Closing the last mmapped file.
		if err := w.cs[oldSeq].Close(); err != nil {
			w.bsMtx.Unlock()
			return err
		}
		w.cs[oldSeq] = newTailFile
		w.bs[oldSeq] = realByteSlice(newTailFile.Bytes())
		w.bsMtx.Unlock()
	}

	mmapFile, err := fileutil.OpenMmapFileWithSize(f.Name(), int(MaxSegmentSize))
	if err != nil {
		return err
	}
	w.bsMtx.Lock()
	w.cs[w.curFileSequence] = mmapFile
	w.bs[w.curFileSequence] = realByteSlice(mmapFile.Bytes())
	w.bsMtx.Unlock()

	return nil
}

// Size returns the size of the chunks.
func (w *HeadReadWriter) Size() int64 {
	return w.size + w.n
}

// finalizeCurFile writes all pending data to the current tail file,
// truncates its size, and closes it.
func (w *HeadReadWriter) finalizeCurFile() error {
	if w.curFile == nil {
		return nil
	}

	if err := w.flushBuffer(); err != nil {
		return err
	}

	// Writing maxt of the file in milliseconds.
	binary.BigEndian.PutUint64(w.buf[:], uint64(w.curFileMaxt))
	if _, err := w.curFile.WriteAt(w.buf[:8], HeaderMaxtOffset); err != nil {
		return nil
	}

	if err := w.curFile.Sync(); err != nil {
		return err
	}

	return w.curFile.Close()
}
