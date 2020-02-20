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
	"bytes"
	"encoding/binary"
	"hash"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// Head chunk file header fields constants.
const (
	// MagicHeadChunks is 4 bytes at the beginning of a head chunk file.
	MagicHeadChunks = 0x0130BC91

	headChunksFormatV1 = 1
	writeBufferSize    = 2 * 1024 * 1024 // 2 MiB.
)

var (
	// DefaultHeadChunkFileMaxTimeRange is the default head chunk file time range.
	// Assuming a general scrape interval of 15s, a chunk with 120 samples would
	// be cut every 30m, so anything <30m will cause lots of empty files. And keeping
	// it exactly 30m also has a chance of having empty files as its near that border.
	// Hence keeping it a little more than 30m, i.e. 40m.
	DefaultHeadChunkFileMaxTimeRange = 40 * int64(time.Minute/time.Millisecond)

	// ErrChunkDiskMapperClosed returned by any method indicates
	// that the ChunkDiskMapper was closed.
	ErrChunkDiskMapperClosed = errors.New("ChunkDiskMapper closed")
)

// corruptionErr is an error that's returned when corruption is encountered.
type corruptionErr struct {
	Dir       string
	FileIndex int
	Err       error
}

func (e *corruptionErr) Error() string {
	return errors.Wrapf(e.Err, "corruption in head chunk file %s", segmentFile(e.Dir, e.FileIndex)).Error()
}

// ChunkDiskMapper is for writing the Head block chunks to the disk
// and access chunks via an mmapped file.
type ChunkDiskMapper struct {
	// Writer.
	dir     *os.File
	curFile *os.File // File being written to.

	curFileMint     int64 // In milliseconds.
	curFileMaxt     int64 // In milliseconds.
	curFileSequence int
	n               int64 // Bytes written in current open file.
	maxFileMs       int64

	writeBuf     [8]byte
	wbuf         *bufio.Writer
	crc32        hash.Hash
	writePathMtx sync.RWMutex

	// Reader.
	// The int key in the map is the file number on the disk.
	mmappedChunkFiles map[int]ByteSlice
	closers           map[int]io.Closer // Closers for resources behind the byte slices.
	pool              chunkenc.Pool
	readPathMtx       sync.RWMutex
	chunkBuffer       *chunkBuffer

	size   int64 // The total size of bytes in the reader.
	closed bool
}

const (
	// MintMaxtSize is the size of the mint/maxt for head chunk file and chunks.
	MintMaxtSize = 8
	// SeriesRefSize is the size of series reference on disk.
	SeriesRefSize = 8
	// HeadChunkFileHeaderSize is the total size of the header for the head chunk file.
	HeadChunkFileHeaderSize = SegmentHeaderSize + 2*MintMaxtSize
	// HeaderMintOffset is the offset where the first byte of MinT for head chunk file exists.
	HeaderMintOffset = SegmentHeaderSize
	// HeaderMaxtOffset is the offset where the first byte of MaxT for head chunk file exists.
	HeaderMaxtOffset = HeaderMintOffset + MintMaxtSize
	// MaxHeadChunkFileSize is the max size of a head chunk file.
	// Setting size to the max int32 as setting it to max int64 crashes 64 systems too.
	MaxHeadChunkFileSize = math.MaxInt32
	// CRCSize is the size of crc32 sum on disk.
	CRCSize = 4
	// MaxHeadChunkMetaSize is the max size of an mmapped chunks minus the chunks data.
	// Max because the uvarint size can be smaller.
	MaxHeadChunkMetaSize = SeriesRefSize + 2*MintMaxtSize + ChunksFormatVersionSize + MaxChunkLengthFieldSize + CRCSize
)

// NewChunkDiskMapper returns a new writer against the given directory
// using the default head chunk file duration.
func NewChunkDiskMapper(dir string, pool chunkenc.Pool) (*ChunkDiskMapper, error) {
	return newChunkDiskMapper(dir, DefaultHeadChunkFileMaxTimeRange, pool)
}

func newChunkDiskMapper(dir string, maxFileDuration int64, pool chunkenc.Pool) (*ChunkDiskMapper, error) {
	if maxFileDuration <= 0 {
		maxFileDuration = DefaultHeadChunkFileMaxTimeRange
	}

	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	dirFile, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}

	m := &ChunkDiskMapper{
		dir:         dirFile,
		maxFileMs:   maxFileDuration,
		pool:        pool,
		crc32:       newCRC32(),
		chunkBuffer: newChunkBuffer(),
	}

	return m, m.openMMapFiles()
}

func (cdm *ChunkDiskMapper) openMMapFiles() (returnErr error) {
	bs := map[int]ByteSlice{}
	cs := map[int]io.Closer{}
	defer func() {
		if returnErr != nil {
			var merr tsdb_errors.MultiError
			merr.Add(returnErr)
			merr.Add(closeAllFromMap(cs))
			returnErr = merr.Err()
		}
	}()

	files, err := listChunkFiles(cdm.dir.Name())
	if err != nil {
		return err
	}
	if cdm.pool == nil {
		cdm.pool = chunkenc.NewPool()
	}

	chkFileIndices := make([]int, 0, len(files))
	for seq, fn := range files {
		f, err := fileutil.OpenMmapFile(fn)
		if err != nil {
			return errors.Wrap(err, "mmap files")
		}
		cs[seq] = f
		bs[seq] = realByteSlice(f.Bytes())
		chkFileIndices = append(chkFileIndices, seq)
	}

	cdm.mmappedChunkFiles = bs
	cdm.closers = cs
	cdm.size = 0

	// Check for gaps in the files.
	sort.Ints(chkFileIndices)
	if len(chkFileIndices) == 0 {
		return nil
	}
	lastSeq := chkFileIndices[0]
	for _, seq := range chkFileIndices[1:] {
		if seq != lastSeq+1 {
			return errors.Errorf("found unsequential head chunk files %d and %d", lastSeq, seq)
		}
		lastSeq = seq
	}

	for i, b := range cdm.mmappedChunkFiles {
		if b.Len() < HeadChunkFileHeaderSize {
			return errors.Wrapf(errInvalidSize, "invalid head chunk file header in file %d", i)
		}
		// Verify magic number.
		if m := binary.BigEndian.Uint32(b.Range(0, MagicChunksSize)); m != MagicHeadChunks {
			return errors.Errorf("invalid magic number %x", m)
		}

		// Verify chunk format version.
		if v := int(b.Range(MagicChunksSize, MagicChunksSize+ChunksFormatVersionSize)[0]); v != chunksFormatV1 {
			return errors.Errorf("invalid chunk format version %d", v)
		}

		maxt := binary.BigEndian.Uint64(b.Range(HeaderMaxtOffset, HeaderMaxtOffset+MintMaxtSize))
		if maxt == 0 {
			// This is possible if Prometheus crashes and the maxt was unwritten to the
			// last head chunk file. As a safe buffer, we set the maxt to mint + 1.5 times head chunk file time range.
			f, err := os.OpenFile(files[i], os.O_WRONLY|os.O_CREATE, 0666)
			if err != nil {
				return err
			}
			defer func() {
				var merr tsdb_errors.MultiError
				merr.Add(returnErr)
				merr.Add(f.Close())
				returnErr = merr.Err()
			}()
			mint := binary.BigEndian.Uint64(b.Range(HeaderMintOffset, HeaderMintOffset+MintMaxtSize))
			binary.BigEndian.PutUint64(cdm.writeBuf[:], mint+(uint64(cdm.maxFileMs)*3/2))
			if _, err := f.WriteAt(cdm.writeBuf[:MintMaxtSize], HeaderMaxtOffset); err != nil {
				return err
			}
		}

		cdm.size += int64(b.Len())
	}

	return nil
}

func listChunkFiles(dir string) (map[int]string, error) {
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

// WriteChunk writes the chunk to the disk.
// The returned chunk ref is the reference from where the chunk encoding starts for the chunk.
func (cdm *ChunkDiskMapper) WriteChunk(seriesRef uint64, mint, maxt int64, chk chunkenc.Chunk) (chkRef uint64, err error) {
	cdm.writePathMtx.Lock()
	defer cdm.writePathMtx.Unlock()

	if cdm.closed {
		return 0, ErrChunkDiskMapperClosed
	}

	if cdm.shouldCutNewFile(len(chk.Bytes()), maxt) {
		if err := cdm.cut(mint); err != nil {
			return 0, err
		}
	}

	// if len(chk.Bytes())+MaxHeadChunkMetaSize >= writeBufferSize, it means that chunk >= the buffer size;
	// so no need to flush here, as we have to flush at the end (to not keep partial chunks in buffer).
	if len(chk.Bytes())+MaxHeadChunkMetaSize < writeBufferSize && cdm.wbuf.Available() < MaxHeadChunkMetaSize+len(chk.Bytes()) {
		if err := cdm.flushBuffer(); err != nil {
			return 0, err
		}
	}

	cdm.crc32.Reset()
	binary.BigEndian.PutUint64(cdm.writeBuf[:], seriesRef)
	if err := cdm.writeWithCRC32(cdm.writeBuf[:SeriesRefSize]); err != nil {
		return 0, err
	}

	binary.BigEndian.PutUint64(cdm.writeBuf[:], uint64(mint))
	if err := cdm.writeWithCRC32(cdm.writeBuf[:MintMaxtSize]); err != nil {
		return 0, err
	}

	binary.BigEndian.PutUint64(cdm.writeBuf[:], uint64(maxt))
	if err := cdm.writeWithCRC32(cdm.writeBuf[:MintMaxtSize]); err != nil {
		return 0, err
	}

	// The reference is set to the head chunk file index and the offset where
	// the data starts for this chunk.
	//
	// The upper 4 bytes are for the head chunk file index and
	// the lower 4 bytes are for the head chunk file offset where to start reading this chunk.
	chkRef = chunkRef(uint64(cdm.curFileSequence), uint64(cdm.n))

	cdm.writeBuf[0] = byte(chk.Encoding())
	if err := cdm.writeWithCRC32(cdm.writeBuf[:1]); err != nil {
		return 0, err
	}

	n := binary.PutUvarint(cdm.writeBuf[:], uint64(len(chk.Bytes())))
	if err := cdm.writeWithCRC32(cdm.writeBuf[:n]); err != nil {
		return 0, err
	}

	if err := cdm.writeWithCRC32(chk.Bytes()); err != nil {
		return 0, err
	}

	if err := cdm.write(cdm.crc32.Sum(cdm.writeBuf[:0])); err != nil {
		return 0, err
	}

	if maxt > cdm.curFileMaxt {
		cdm.curFileMaxt = maxt
	}

	if mint < cdm.curFileMint {
		cdm.curFileMint = mint
		// As this wont happen a whole lot of time, we don't wait
		// for a new head chunk file to be cut to write it off.
		binary.BigEndian.PutUint64(cdm.writeBuf[:], uint64(mint))
		if _, err := cdm.curFile.WriteAt(cdm.writeBuf[:MintMaxtSize], HeaderMintOffset); err != nil {
			return 0, err
		}
	}

	cdm.chunkBuffer.put(chkRef, chk)

	if len(chk.Bytes())+MaxHeadChunkMetaSize >= writeBufferSize {
		// The chunk was bigger than the buffer itself.
		// Flushing to not keep partial chunks in buffer.
		if err := cdm.flushBuffer(); err != nil {
			return 0, err
		}
	}

	return chkRef, nil
}

func chunkRef(seq, offset uint64) (chunkRef uint64) {
	return (seq << 32) | offset
}

func (cdm *ChunkDiskMapper) shouldCutNewFile(chunkSize int, maxt int64) bool {
	return cdm.n == 0 || // First head chunk file.
		(maxt-cdm.curFileMint > cdm.maxFileMs && cdm.n > HeadChunkFileHeaderSize) || // Time duration reached for the existing file.
		cdm.n+int64(chunkSize+MaxHeadChunkMetaSize) >= MaxHeadChunkFileSize // Exceeds the max head chunk file size.
}

func (cdm *ChunkDiskMapper) cut(mint int64) (returnErr error) {
	// Sync current tail to disk and close.
	if err := cdm.finalizeCurFile(); err != nil {
		return err
	}

	n, f, seq, err := cutSegmentFile(cdm.dir, MagicHeadChunks, headChunksFormatV1, 0)
	if err != nil {
		return err
	}
	defer func() {
		// The file should not be closed if there is no error,
		// its kept open in the ChunkDiskMapper.
		if returnErr != nil {
			var merr tsdb_errors.MultiError
			merr.Add(returnErr)
			merr.Add(f.Close())
			returnErr = merr.Err()
		}
	}()

	cdm.size += cdm.n
	atomic.StoreInt64(&cdm.n, int64(n))
	oldSeq := cdm.curFileSequence
	cdm.curFileSequence = seq

	cdm.curFileMint = mint
	binary.BigEndian.PutUint64(cdm.writeBuf[:], uint64(mint))
	if _, err := f.Write(cdm.writeBuf[:MintMaxtSize]); err != nil {
		return err
	}
	binary.BigEndian.PutUint64(cdm.writeBuf[:], 0)
	if _, err := f.Write(cdm.writeBuf[:MintMaxtSize]); err != nil {
		return err
	}
	atomic.AddInt64(&cdm.n, 16)

	oldFile := cdm.curFile

	cdm.curFile = f
	if cdm.wbuf != nil {
		cdm.wbuf.Reset(f)
	} else {
		cdm.wbuf = bufio.NewWriterSize(f, writeBufferSize)
	}

	if oldFile != nil {
		// Open it again with the new size.
		newTailFile, err := fileutil.OpenMmapFile(oldFile.Name())
		if err != nil {
			return err
		}
		cdm.readPathMtx.Lock()
		oldMmapFile := cdm.closers[oldSeq]
		cdm.closers[oldSeq] = newTailFile
		cdm.mmappedChunkFiles[oldSeq] = realByteSlice(newTailFile.Bytes())
		cdm.readPathMtx.Unlock()
		// Closing the last mmapped file.
		if err := oldMmapFile.Close(); err != nil {
			return err
		}
	}

	mmapFile, err := fileutil.OpenMmapFileWithSize(f.Name(), int(MaxHeadChunkFileSize))
	if err != nil {
		return err
	}
	cdm.readPathMtx.Lock()
	cdm.closers[cdm.curFileSequence] = mmapFile
	cdm.mmappedChunkFiles[cdm.curFileSequence] = realByteSlice(mmapFile.Bytes())
	cdm.readPathMtx.Unlock()

	cdm.curFileMaxt = 0

	return nil
}

// finalizeCurFile writes all pending data to the current tail file,
// truncates its size, and closes it.
func (cdm *ChunkDiskMapper) finalizeCurFile() error {
	if cdm.curFile == nil {
		return nil
	}

	if err := cdm.flushBuffer(); err != nil {
		return err
	}

	binary.BigEndian.PutUint64(cdm.writeBuf[:], uint64(cdm.curFileMaxt))
	if _, err := cdm.curFile.WriteAt(cdm.writeBuf[:MintMaxtSize], HeaderMaxtOffset); err != nil {
		return nil
	}

	if err := cdm.curFile.Sync(); err != nil {
		return err
	}

	return cdm.curFile.Close()
}

func (cdm *ChunkDiskMapper) write(b []byte) error {
	n, err := cdm.wbuf.Write(b)
	atomic.AddInt64(&cdm.n, int64(n))
	return err
}

func (cdm *ChunkDiskMapper) writeWithCRC32(b []byte) error {
	if err := cdm.write(b); err != nil {
		return err
	}
	_, err := cdm.crc32.Write(b)
	return err
}

// flushBuffer flushes the current in-memory chunks.
// Assumes that writePathMtx is _write_ locked before calling this method.
func (cdm *ChunkDiskMapper) flushBuffer() error {
	if err := cdm.wbuf.Flush(); err != nil {
		return err
	}
	cdm.chunkBuffer.clear()
	return nil
}

// Chunk returns a chunk from a given reference.
// Note: The returned chunk will turn invalid after closing ChunkDiskMapper.
func (cdm *ChunkDiskMapper) Chunk(ref uint64) (chunkenc.Chunk, error) {
	var (
		// Get the upper 4 bytes.
		// These contain the head chunk file index.
		sgmIndex = int(ref >> 32)
		// Get the lower 4 bytes.
		// These contain the head chunk file offset where the encoding for this chunk starts.
		chkStart = int((ref << 32) >> 32)
		chkCRC32 = newCRC32()
	)

	cdm.readPathMtx.RLock()
	// We hold this read lock for the entire duration because if the Close()
	// is called, the data in the byte slice will get corrupted as the mmapped
	// file will be closed.
	defer cdm.readPathMtx.RUnlock()

	if sgmIndex == cdm.curFileSequence {
		chunk := cdm.chunkBuffer.get(ref)
		if chunk != nil {
			return chunk, nil
		}
	}

	if cdm.closed {
		return nil, ErrChunkDiskMapperClosed
	}

	sgmBytes, ok := cdm.mmappedChunkFiles[sgmIndex]
	if !ok {
		if sgmIndex > cdm.curFileSequence {
			return nil, errors.Errorf("head chunk file index %d more than current open file", sgmIndex)
		}
		return nil, errors.Errorf("head chunk file index %d does not exist on disk", sgmIndex)
	}

	if chkStart+MaxChunkLengthFieldSize > sgmBytes.Len() {
		return nil, errors.Errorf("head chunk file doesn't include enough bytes to read the chunk size data field - required:%v, available:%v, file:%d", chkStart+MaxChunkLengthFieldSize, sgmBytes.Len(), sgmIndex)
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

	// Verify the chunk data end.
	chkDataEnd := chkDataLenStart + n + int(chkDataLen)
	if chkDataEnd > sgmBytes.Len() {
		return nil, errors.Errorf("head chunk file doesn't include enough bytes to read the chunk - required:%v, available:%v", chkDataEnd, sgmBytes.Len())
	}

	// Check the CRC.
	sum := sgmBytes.Range(chkDataEnd, chkDataEnd+CRCSize)
	if _, err := chkCRC32.Write(sgmBytes.Range(chkStart-(SeriesRefSize+2*MintMaxtSize), chkDataEnd)); err != nil {
		return nil, err
	}
	if act := chkCRC32.Sum(nil); !bytes.Equal(act, sum) {
		return nil, errors.Errorf("checksum mismatch expected:%x, actual:%x", sum, act)
	}

	// The chunk data itself.
	chkData := sgmBytes.Range(chkDataEnd-int(chkDataLen), chkDataEnd)
	return cdm.pool.Get(chunkenc.Encoding(chkEnc), chkData)
}

// IterateAllChunks iterates on all the chunks in its byte slices in the order of the head chunk file sequence
// and runs the provided function on each chunk. It returns on the first error encountered.
func (cdm *ChunkDiskMapper) IterateAllChunks(f func(seriesRef, chunkRef uint64, mint, maxt int64) error) error {
	chkCRC32 := newCRC32()

	// Iterate files in ascending order.
	seqs := make([]int, 0, len(cdm.mmappedChunkFiles))
	for seg := range cdm.mmappedChunkFiles {
		seqs = append(seqs, seg)
	}
	sort.Ints(seqs)
	for _, seq := range seqs {
		bs := cdm.mmappedChunkFiles[seq]
		sliceLen := bs.Len()
		idx := HeadChunkFileHeaderSize
		for idx < sliceLen {
			if sliceLen-idx < MaxHeadChunkMetaSize {
				return &corruptionErr{
					Dir:       cdm.dir.Name(),
					FileIndex: seq,
					Err:       errors.Errorf("head chunk file doesn't include enough bytes to read the chunk header - required:%v, available:%v, file:%d", idx+MaxHeadChunkMetaSize, sliceLen, seq),
				}
			}
			chkCRC32.Reset()
			startIdx := idx
			seriesRef := binary.BigEndian.Uint64(bs.Range(idx, idx+SeriesRefSize))
			idx += SeriesRefSize

			mint := int64(binary.BigEndian.Uint64(bs.Range(idx, idx+MintMaxtSize)))
			idx += MintMaxtSize

			maxt := int64(binary.BigEndian.Uint64(bs.Range(idx, idx+MintMaxtSize)))
			idx += MintMaxtSize

			chunkRef := chunkRef(uint64(seq), uint64(idx))

			idx += ChunkEncodingSize // Skip encoding.
			// Skip the data.
			dataLen, n := binary.Uvarint(bs.Range(idx, idx+MaxChunkLengthFieldSize))
			idx += n + int(dataLen)

			// In the beginning we only checked for the chunk meta size.
			// Now that we have added the chunk data length, we check for sufficient bytes again.
			if idx+CRCSize > sliceLen {
				return &corruptionErr{
					Dir:       cdm.dir.Name(),
					FileIndex: seq,
					Err:       errors.Errorf("head chunk file doesn't include enough bytes to read the chunk header - required:%v, available:%v, file:%d", idx+CRCSize, sliceLen, seq),
				}
			}

			// Check CRC.
			sum := bs.Range(idx, idx+CRCSize)
			if _, err := chkCRC32.Write(bs.Range(startIdx, idx)); err != nil {
				return err
			}
			if act := chkCRC32.Sum(nil); !bytes.Equal(act, sum) {
				return &corruptionErr{
					Dir:       cdm.dir.Name(),
					FileIndex: seq,
					Err:       errors.Errorf("checksum mismatch expected:%x, actual:%x", sum, act),
				}
			}

			idx += CRCSize

			if err := f(seriesRef, chunkRef, mint, maxt); err != nil {
				return err
			}
		}

		if idx > sliceLen {
			// It should be equal to the slice length.
			return &corruptionErr{
				Dir:       cdm.dir.Name(),
				FileIndex: seq,
				Err:       errors.Errorf("head chunk file doesn't include enough bytes to read the last chunk data - required:%v, available:%v, file:%d", idx, sliceLen, seq),
			}
		}
	}

	return nil
}

// Truncate deletes the head chunk files which are strictly below the mint.
// mint should be in milliseconds.
func (cdm *ChunkDiskMapper) Truncate(mint int64) error {
	var removedFiles []int

	cdm.readPathMtx.RLock()
	for seq, bs := range cdm.mmappedChunkFiles {
		if seq == cdm.curFileSequence {
			continue
		}
		b := bs.Range(HeaderMaxtOffset, HeaderMaxtOffset+MintMaxtSize)
		maxt := binary.BigEndian.Uint64(b)
		if int64(maxt) < mint {
			removedFiles = append(removedFiles, seq)
		}
	}
	cdm.readPathMtx.RUnlock()

	return cdm.deleteFiles(removedFiles)
}

func (cdm *ChunkDiskMapper) deleteFiles(removedFiles []int) error {
	cdm.readPathMtx.Lock()
	for _, seq := range removedFiles {
		if err := cdm.closers[seq].Close(); err != nil {
			cdm.readPathMtx.Unlock()
			return err
		}
		cdm.size -= int64(cdm.mmappedChunkFiles[seq].Len())
		delete(cdm.mmappedChunkFiles, seq)
		delete(cdm.closers, seq)
	}
	cdm.readPathMtx.Unlock()

	// We actually delete the files separately to not block the readPathMtx for long.
	for _, seq := range removedFiles {
		if err := os.Remove(segmentFile(cdm.dir.Name(), seq)); err != nil {
			return err
		}
	}

	return nil
}

// Repair deletes all the head chunk files after the one which had the corruption
// (including the corrupt file).
func (cdm *ChunkDiskMapper) Repair(originalErr error) error {
	err := errors.Cause(originalErr) // So that we can pick up errors even if wrapped.
	cerr, ok := err.(*corruptionErr)
	if !ok {
		return errors.Wrap(originalErr, "cannot handle error")
	}

	// Delete all the head chunk files following the corrupt head chunk file.
	segs := []int{}
	for seg := range cdm.mmappedChunkFiles {
		if seg >= cerr.FileIndex {
			segs = append(segs, seg)
		}
	}
	return cdm.deleteFiles(segs)
}

// Size returns the size of the chunk files.
func (cdm *ChunkDiskMapper) Size() int64 {
	n := atomic.LoadInt64(&cdm.n)
	return cdm.size + n
}

func (cdm *ChunkDiskMapper) Close() error {
	// 'WriteChunk' locks writePathMtx first and then readPathMtx for cutting head chunk file.
	// The lock order should not be reversed here else it can cause deadlocks.
	cdm.writePathMtx.Lock()
	defer cdm.writePathMtx.Unlock()
	cdm.readPathMtx.Lock()
	defer cdm.readPathMtx.Unlock()

	if cdm.closed {
		return nil
	}
	cdm.closed = true

	if err := cdm.finalizeCurFile(); err != nil {
		return err
	}

	if err := cdm.dir.Close(); err != nil {
		return err
	}

	return closeAllFromMap(cdm.closers)
}

func closeAllFromMap(cs map[int]io.Closer) error {
	var merr tsdb_errors.MultiError
	for _, c := range cs {
		merr.Add(c.Close())
	}
	return merr.Err()
}

const inBufferShards = 128 // 128 is a randomly chosen number.

// chunkBuffer is a thread safe buffer for chunks.
type chunkBuffer struct {
	inBufferChunks     [inBufferShards]map[uint64]chunkenc.Chunk
	inBufferChunksMtxs [inBufferShards]sync.RWMutex
}

func newChunkBuffer() *chunkBuffer {
	cb := &chunkBuffer{}
	for i := 0; i < inBufferShards; i++ {
		cb.inBufferChunks[i] = make(map[uint64]chunkenc.Chunk)
	}
	return cb
}

func (cb *chunkBuffer) put(ref uint64, chk chunkenc.Chunk) {
	shardIdx := ref % inBufferShards

	cb.inBufferChunksMtxs[shardIdx].Lock()
	cb.inBufferChunks[shardIdx][ref] = chk
	cb.inBufferChunksMtxs[shardIdx].Unlock()
}

func (cb *chunkBuffer) get(ref uint64) chunkenc.Chunk {
	shardIdx := ref % inBufferShards

	cb.inBufferChunksMtxs[shardIdx].RLock()
	defer cb.inBufferChunksMtxs[shardIdx].RUnlock()

	return cb.inBufferChunks[shardIdx][ref]
}

func (cb *chunkBuffer) clear() {
	for i := 0; i < inBufferShards; i++ {
		cb.inBufferChunksMtxs[i].Lock()
		cb.inBufferChunks[i] = make(map[uint64]chunkenc.Chunk)
		cb.inBufferChunksMtxs[i].Unlock()
	}
}
