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
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"

	"github.com/dennwc/varint"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// Head chunk file header fields constants.
const (
	// MagicHeadChunks is 4 bytes at the beginning of a head chunk file.
	MagicHeadChunks = 0x0130BC91

	headChunksFormatV1 = 1
)

// ErrChunkDiskMapperClosed returned by any method indicates
// that the ChunkDiskMapper was closed.
var ErrChunkDiskMapperClosed = errors.New("ChunkDiskMapper closed")

const (
	// MintMaxtSize is the size of the mint/maxt for head chunk file and chunks.
	MintMaxtSize = 8
	// SeriesRefSize is the size of series reference on disk.
	SeriesRefSize = 8
	// HeadChunkFileHeaderSize is the total size of the header for a head chunk file.
	HeadChunkFileHeaderSize = SegmentHeaderSize
	// MaxHeadChunkFileSize is the max size of a head chunk file.
	MaxHeadChunkFileSize = 128 * 1024 * 1024 // 128 MiB.
	// CRCSize is the size of crc32 sum on disk.
	CRCSize = 4
	// MaxHeadChunkMetaSize is the max size of an mmapped chunks minus the chunks data.
	// Max because the uvarint size can be smaller.
	MaxHeadChunkMetaSize = SeriesRefSize + 2*MintMaxtSize + ChunkEncodingSize + MaxChunkLengthFieldSize + CRCSize
	// MinWriteBufferSize is the minimum write buffer size allowed.
	MinWriteBufferSize = 64 * 1024 // 64KB.
	// MaxWriteBufferSize is the maximum write buffer size allowed.
	MaxWriteBufferSize = 8 * 1024 * 1024 // 8 MiB.
	// DefaultWriteBufferSize is the default write buffer size.
	DefaultWriteBufferSize = 4 * 1024 * 1024 // 4 MiB.
	// DefaultWriteQueueSize is the default size of the in-memory queue used before flushing chunks to the disk.
	DefaultWriteQueueSize = 1000
)

// ChunkDiskMapperRef represents the location of a head chunk on disk.
// The upper 4 bytes hold the index of the head chunk file and
// the lower 4 bytes hold the byte offset in the head chunk file where the chunk starts.
type ChunkDiskMapperRef uint64

func newChunkDiskMapperRef(seq, offset uint64) ChunkDiskMapperRef {
	return ChunkDiskMapperRef((seq << 32) | offset)
}

func (ref ChunkDiskMapperRef) Unpack() (seq, offset int) {
	seq = int(ref >> 32)
	offset = int((ref << 32) >> 32)
	return seq, offset
}

// CorruptionErr is an error that's returned when corruption is encountered.
type CorruptionErr struct {
	Dir       string
	FileIndex int
	Err       error
}

func (e *CorruptionErr) Error() string {
	return errors.Wrapf(e.Err, "corruption in head chunk file %s", segmentFile(e.Dir, e.FileIndex)).Error()
}

// chunkPos keeps track of the position in the head chunk files.
// chunkPos is not thread-safe, a lock must be used to protect it.
type chunkPos struct {
	seq     uint64 // Index of chunk file.
	offset  uint64 // Offset within chunk file.
	cutFile bool   // When true then the next chunk will be written to a new file.
}

// getNextChunkRef takes a chunk and returns the chunk reference which will refer to it once it has been written.
// getNextChunkRef also decides whether a new file should be cut before writing this chunk, and it returns the decision via the second return value.
// The order of calling getNextChunkRef must be the order in which chunks are written to the disk.
func (f *chunkPos) getNextChunkRef(chk chunkenc.Chunk) (chkRef ChunkDiskMapperRef, cutFile bool) {
	chkLen := uint64(len(chk.Bytes()))
	bytesToWrite := f.bytesToWriteForChunk(chkLen)

	if f.shouldCutNewFile(bytesToWrite) {
		f.toNewFile()
		f.cutFile = false
		cutFile = true
	}

	chkOffset := f.offset
	f.offset += bytesToWrite

	return newChunkDiskMapperRef(f.seq, chkOffset), cutFile
}

// toNewFile updates the seq/offset position to point to the beginning of a new chunk file.
func (f *chunkPos) toNewFile() {
	f.seq++
	f.offset = SegmentHeaderSize
}

// cutFileOnNextChunk triggers that the next chunk will be written in to a new file.
// Not thread safe, a lock must be held when calling this.
func (f *chunkPos) cutFileOnNextChunk() {
	f.cutFile = true
}

// initSeq sets the sequence number of the head chunk file.
// Should only be used for initialization, after that the sequence number will be managed by chunkPos.
func (f *chunkPos) initSeq(seq uint64) {
	f.seq = seq
}

// shouldCutNewFile returns whether a new file should be cut based on the file size.
// Not thread safe, a lock must be held when calling this.
func (f *chunkPos) shouldCutNewFile(bytesToWrite uint64) bool {
	if f.cutFile {
		return true
	}

	return f.offset == 0 || // First head chunk file.
		f.offset+bytesToWrite > MaxHeadChunkFileSize // Exceeds the max head chunk file size.
}

// bytesToWriteForChunk returns the number of bytes that will need to be written for the given chunk size,
// including all meta data before and after the chunk data.
// Head chunk format: https://github.com/prometheus/prometheus/blob/main/tsdb/docs/format/head_chunks.md#chunk
func (f *chunkPos) bytesToWriteForChunk(chkLen uint64) uint64 {
	// Headers.
	bytes := uint64(SeriesRefSize) + 2*MintMaxtSize + ChunkEncodingSize

	// Size of chunk length encoded as uvarint.
	bytes += uint64(varint.UvarintSize(chkLen))

	// Chunk length.
	bytes += chkLen

	// crc32.
	bytes += CRCSize

	return bytes
}

// ChunkDiskMapper is for writing the Head block chunks to the disk
// and access chunks via mmapped file.
type ChunkDiskMapper struct {
	/// Writer.
	dir             *os.File
	writeBufferSize int

	curFile         *os.File      // File being written to.
	curFileSequence int           // Index of current open file being appended to.
	curFileOffset   atomic.Uint64 // Bytes written in current open file.
	curFileMaxt     int64         // Used for the size retention.

	// The values in evtlPos represent the file position which will eventually be
	// reached once the content of the write queue has been fully processed.
	evtlPosMtx sync.Mutex
	evtlPos    chunkPos

	byteBuf      [MaxHeadChunkMetaSize]byte // Buffer used to write the header of the chunk.
	chkWriter    *bufio.Writer              // Writer for the current open file.
	crc32        hash.Hash
	writePathMtx sync.Mutex

	/// Reader.
	// The int key in the map is the file number on the disk.
	mmappedChunkFiles map[int]*mmappedChunkFile // Contains the m-mapped files for each chunk file mapped with its index.
	closers           map[int]io.Closer         // Closers for resources behind the byte slices.
	readPathMtx       sync.RWMutex              // Mutex used to protect the above 2 maps.
	pool              chunkenc.Pool             // This is used when fetching a chunk from the disk to allocate a chunk.

	// Writer and Reader.
	// We flush chunks to disk in batches. Hence, we store them in this buffer
	// from which chunks are served till they are flushed and are ready for m-mapping.
	chunkBuffer *chunkBuffer

	// Whether the maxt field is set for all mmapped chunk files tracked within the mmappedChunkFiles map.
	// This is done after iterating through all the chunks in those files using the IterateAllChunks method.
	fileMaxtSet bool

	writeQueue *chunkWriteQueue

	closed bool
}

// mmappedChunkFile provides mmapp access to an entire head chunks file that holds many chunks.
type mmappedChunkFile struct {
	byteSlice ByteSlice
	maxt      int64 // Max timestamp among all of this file's chunks.
}

// NewChunkDiskMapper returns a new ChunkDiskMapper against the given directory
// using the default head chunk file duration.
// NOTE: 'IterateAllChunks' method needs to be called at least once after creating ChunkDiskMapper
// to set the maxt of all the file.
func NewChunkDiskMapper(reg prometheus.Registerer, dir string, pool chunkenc.Pool, writeBufferSize, writeQueueSize int) (*ChunkDiskMapper, error) {
	// Validate write buffer size.
	if writeBufferSize < MinWriteBufferSize || writeBufferSize > MaxWriteBufferSize {
		return nil, errors.Errorf("ChunkDiskMapper write buffer size should be between %d and %d (actual: %d)", MinWriteBufferSize, MaxWriteBufferSize, writeBufferSize)
	}
	if writeBufferSize%1024 != 0 {
		return nil, errors.Errorf("ChunkDiskMapper write buffer size should be a multiple of 1024 (actual: %d)", writeBufferSize)
	}

	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, err
	}
	dirFile, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}

	m := &ChunkDiskMapper{
		dir:             dirFile,
		pool:            pool,
		writeBufferSize: writeBufferSize,
		crc32:           newCRC32(),
		chunkBuffer:     newChunkBuffer(),
	}
	m.writeQueue = newChunkWriteQueue(reg, writeQueueSize, m.writeChunk)

	if m.pool == nil {
		m.pool = chunkenc.NewPool()
	}

	return m, m.openMMapFiles()
}

// openMMapFiles opens all files within dir for mmapping.
func (cdm *ChunkDiskMapper) openMMapFiles() (returnErr error) {
	cdm.mmappedChunkFiles = map[int]*mmappedChunkFile{}
	cdm.closers = map[int]io.Closer{}
	defer func() {
		if returnErr != nil {
			returnErr = tsdb_errors.NewMulti(returnErr, closeAllFromMap(cdm.closers)).Err()

			cdm.mmappedChunkFiles = nil
			cdm.closers = nil
		}
	}()

	files, err := listChunkFiles(cdm.dir.Name())
	if err != nil {
		return err
	}

	files, err = repairLastChunkFile(files)
	if err != nil {
		return err
	}

	chkFileIndices := make([]int, 0, len(files))
	for seq, fn := range files {
		f, err := fileutil.OpenMmapFile(fn)
		if err != nil {
			return errors.Wrapf(err, "mmap files, file: %s", fn)
		}
		cdm.closers[seq] = f
		cdm.mmappedChunkFiles[seq] = &mmappedChunkFile{byteSlice: realByteSlice(f.Bytes())}
		chkFileIndices = append(chkFileIndices, seq)
	}

	// Check for gaps in the files.
	sort.Ints(chkFileIndices)
	if len(chkFileIndices) == 0 {
		return nil
	}
	lastSeq := chkFileIndices[0]
	for _, seq := range chkFileIndices[1:] {
		if seq != lastSeq+1 {
			return errors.Errorf("found unsequential head chunk files %s (index: %d) and %s (index: %d)", files[lastSeq], lastSeq, files[seq], seq)
		}
		lastSeq = seq
	}

	for i, b := range cdm.mmappedChunkFiles {
		if b.byteSlice.Len() < HeadChunkFileHeaderSize {
			return errors.Wrapf(errInvalidSize, "%s: invalid head chunk file header", files[i])
		}
		// Verify magic number.
		if m := binary.BigEndian.Uint32(b.byteSlice.Range(0, MagicChunksSize)); m != MagicHeadChunks {
			return errors.Errorf("%s: invalid magic number %x", files[i], m)
		}

		// Verify chunk format version.
		if v := int(b.byteSlice.Range(MagicChunksSize, MagicChunksSize+ChunksFormatVersionSize)[0]); v != chunksFormatV1 {
			return errors.Errorf("%s: invalid chunk format version %d", files[i], v)
		}
	}

	cdm.evtlPos.initSeq(uint64(lastSeq))

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

// repairLastChunkFile deletes the last file if it's empty.
// Because we don't fsync when creating these files, we could end
// up with an empty file at the end during an abrupt shutdown.
func repairLastChunkFile(files map[int]string) (_ map[int]string, returnErr error) {
	lastFile := -1
	for seq := range files {
		if seq > lastFile {
			lastFile = seq
		}
	}

	if lastFile <= 0 {
		return files, nil
	}

	info, err := os.Stat(files[lastFile])
	if err != nil {
		return files, errors.Wrap(err, "file stat during last head chunk file repair")
	}
	if info.Size() == 0 {
		// Corrupt file, hence remove it.
		if err := os.RemoveAll(files[lastFile]); err != nil {
			return files, errors.Wrap(err, "delete corrupted, empty head chunk file during last file repair")
		}
		delete(files, lastFile)
	}

	return files, nil
}

// WriteChunk writes the chunk to the disk.
// The returned chunk ref is the reference from where the chunk encoding starts for the chunk.
func (cdm *ChunkDiskMapper) WriteChunk(seriesRef HeadSeriesRef, mint, maxt int64, chk chunkenc.Chunk, callback func(err error)) (chkRef ChunkDiskMapperRef) {
	var err error
	defer func() {
		if err != nil && callback != nil {
			callback(err)
		}
	}()

	// cdm.evtlPosMtx must be held to serialize the calls to .getNextChunkRef() and .addJob().
	cdm.evtlPosMtx.Lock()
	defer cdm.evtlPosMtx.Unlock()

	ref, cutFile := cdm.evtlPos.getNextChunkRef(chk)
	err = cdm.writeQueue.addJob(chunkWriteJob{
		cutFile:   cutFile,
		seriesRef: seriesRef,
		mint:      mint,
		maxt:      maxt,
		chk:       chk,
		ref:       ref,
		callback:  callback,
	})

	return ref
}

func (cdm *ChunkDiskMapper) writeChunk(seriesRef HeadSeriesRef, mint, maxt int64, chk chunkenc.Chunk, ref ChunkDiskMapperRef, cutFile bool) (err error) {
	cdm.writePathMtx.Lock()
	defer cdm.writePathMtx.Unlock()

	if cdm.closed {
		return ErrChunkDiskMapperClosed
	}

	if cutFile {
		err := cdm.cutAndExpectRef(ref)
		if err != nil {
			return err
		}
	}

	// if len(chk.Bytes())+MaxHeadChunkMetaSize >= writeBufferSize, it means that chunk >= the buffer size;
	// so no need to flush here, as we have to flush at the end (to not keep partial chunks in buffer).
	if len(chk.Bytes())+MaxHeadChunkMetaSize < cdm.writeBufferSize && cdm.chkWriter.Available() < MaxHeadChunkMetaSize+len(chk.Bytes()) {
		if err := cdm.flushBuffer(); err != nil {
			return err
		}
	}

	cdm.crc32.Reset()
	bytesWritten := 0

	binary.BigEndian.PutUint64(cdm.byteBuf[bytesWritten:], uint64(seriesRef))
	bytesWritten += SeriesRefSize
	binary.BigEndian.PutUint64(cdm.byteBuf[bytesWritten:], uint64(mint))
	bytesWritten += MintMaxtSize
	binary.BigEndian.PutUint64(cdm.byteBuf[bytesWritten:], uint64(maxt))
	bytesWritten += MintMaxtSize
	cdm.byteBuf[bytesWritten] = byte(chk.Encoding())
	bytesWritten += ChunkEncodingSize
	n := binary.PutUvarint(cdm.byteBuf[bytesWritten:], uint64(len(chk.Bytes())))
	bytesWritten += n

	if err := cdm.writeAndAppendToCRC32(cdm.byteBuf[:bytesWritten]); err != nil {
		return err
	}
	if err := cdm.writeAndAppendToCRC32(chk.Bytes()); err != nil {
		return err
	}
	if err := cdm.writeCRC32(); err != nil {
		return err
	}

	if maxt > cdm.curFileMaxt {
		cdm.curFileMaxt = maxt
	}

	cdm.chunkBuffer.put(ref, chk)

	if len(chk.Bytes())+MaxHeadChunkMetaSize >= cdm.writeBufferSize {
		// The chunk was bigger than the buffer itself.
		// Flushing to not keep partial chunks in buffer.
		if err := cdm.flushBuffer(); err != nil {
			return err
		}
	}

	return nil
}

// CutNewFile makes that a new file will be created the next time a chunk is written.
func (cdm *ChunkDiskMapper) CutNewFile() {
	cdm.evtlPosMtx.Lock()
	defer cdm.evtlPosMtx.Unlock()

	cdm.evtlPos.cutFileOnNextChunk()
}

func (cdm *ChunkDiskMapper) IsQueueEmpty() bool {
	return cdm.writeQueue.queueIsEmpty()
}

// cutAndExpectRef creates a new m-mapped file.
// The write lock should be held before calling this.
// It ensures that the position in the new file matches the given chunk reference, if not then it errors.
func (cdm *ChunkDiskMapper) cutAndExpectRef(chkRef ChunkDiskMapperRef) (err error) {
	seq, offset, err := cdm.cut()
	if err != nil {
		return err
	}

	if expSeq, expOffset := chkRef.Unpack(); seq != expSeq || offset != expOffset {
		return errors.Errorf("expected newly cut file to have sequence:offset %d:%d, got %d:%d", expSeq, expOffset, seq, offset)
	}

	return nil
}

// cut creates a new m-mapped file. The write lock should be held before calling this.
// It returns the file sequence and the offset in that file to start writing chunks.
func (cdm *ChunkDiskMapper) cut() (seq, offset int, returnErr error) {
	// Sync current tail to disk and close.
	if err := cdm.finalizeCurFile(); err != nil {
		return 0, 0, err
	}

	offset, newFile, seq, err := cutSegmentFile(cdm.dir, MagicHeadChunks, headChunksFormatV1, HeadChunkFilePreallocationSize)
	if err != nil {
		return 0, 0, err
	}

	defer func() {
		// The file should not be closed if there is no error,
		// its kept open in the ChunkDiskMapper.
		if returnErr != nil {
			returnErr = tsdb_errors.NewMulti(returnErr, newFile.Close()).Err()
		}
	}()

	cdm.curFileOffset.Store(uint64(offset))

	if cdm.curFile != nil {
		cdm.readPathMtx.Lock()
		cdm.mmappedChunkFiles[cdm.curFileSequence].maxt = cdm.curFileMaxt
		cdm.readPathMtx.Unlock()
	}

	mmapFile, err := fileutil.OpenMmapFileWithSize(newFile.Name(), MaxHeadChunkFileSize)
	if err != nil {
		return 0, 0, err
	}

	cdm.readPathMtx.Lock()
	cdm.curFileSequence = seq
	cdm.curFile = newFile
	if cdm.chkWriter != nil {
		cdm.chkWriter.Reset(newFile)
	} else {
		cdm.chkWriter = bufio.NewWriterSize(newFile, cdm.writeBufferSize)
	}

	cdm.closers[cdm.curFileSequence] = mmapFile
	cdm.mmappedChunkFiles[cdm.curFileSequence] = &mmappedChunkFile{byteSlice: realByteSlice(mmapFile.Bytes())}
	cdm.readPathMtx.Unlock()

	cdm.curFileMaxt = 0

	return seq, offset, nil
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

	if err := cdm.curFile.Sync(); err != nil {
		return err
	}

	return cdm.curFile.Close()
}

func (cdm *ChunkDiskMapper) write(b []byte) error {
	n, err := cdm.chkWriter.Write(b)
	cdm.curFileOffset.Add(uint64(n))
	return err
}

func (cdm *ChunkDiskMapper) writeAndAppendToCRC32(b []byte) error {
	if err := cdm.write(b); err != nil {
		return err
	}
	_, err := cdm.crc32.Write(b)
	return err
}

func (cdm *ChunkDiskMapper) writeCRC32() error {
	return cdm.write(cdm.crc32.Sum(cdm.byteBuf[:0]))
}

// flushBuffer flushes the current in-memory chunks.
// Assumes that writePathMtx is _write_ locked before calling this method.
func (cdm *ChunkDiskMapper) flushBuffer() error {
	if err := cdm.chkWriter.Flush(); err != nil {
		return err
	}
	cdm.chunkBuffer.clear()
	return nil
}

// Chunk returns a chunk from a given reference.
func (cdm *ChunkDiskMapper) Chunk(ref ChunkDiskMapperRef) (chunkenc.Chunk, error) {
	cdm.readPathMtx.RLock()
	// We hold this read lock for the entire duration because if Close()
	// is called, the data in the byte slice will get corrupted as the mmapped
	// file will be closed.
	defer cdm.readPathMtx.RUnlock()

	if cdm.closed {
		return nil, ErrChunkDiskMapperClosed
	}

	chunk := cdm.writeQueue.get(ref)
	if chunk != nil {
		return chunk, nil
	}

	sgmIndex, chkStart := ref.Unpack()
	// We skip the series ref and the mint/maxt beforehand.
	chkStart += SeriesRefSize + (2 * MintMaxtSize)
	chkCRC32 := newCRC32()

	// If it is the current open file, then the chunks can be in the buffer too.
	if sgmIndex == cdm.curFileSequence {
		chunk := cdm.chunkBuffer.get(ref)
		if chunk != nil {
			return chunk, nil
		}
	}

	mmapFile, ok := cdm.mmappedChunkFiles[sgmIndex]
	if !ok {
		if sgmIndex > cdm.curFileSequence {
			return nil, &CorruptionErr{
				Dir:       cdm.dir.Name(),
				FileIndex: -1,
				Err:       errors.Errorf("head chunk file index %d more than current open file", sgmIndex),
			}
		}
		return nil, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.New("head chunk file index %d does not exist on disk"),
		}
	}

	if chkStart+MaxChunkLengthFieldSize > mmapFile.byteSlice.Len() {
		return nil, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.Errorf("head chunk file doesn't include enough bytes to read the chunk size data field - required:%v, available:%v", chkStart+MaxChunkLengthFieldSize, mmapFile.byteSlice.Len()),
		}
	}

	// Encoding.
	chkEnc := mmapFile.byteSlice.Range(chkStart, chkStart+ChunkEncodingSize)[0]

	// Data length.
	// With the minimum chunk length this should never cause us reading
	// over the end of the slice.
	chkDataLenStart := chkStart + ChunkEncodingSize
	c := mmapFile.byteSlice.Range(chkDataLenStart, chkDataLenStart+MaxChunkLengthFieldSize)
	chkDataLen, n := binary.Uvarint(c)
	if n <= 0 {
		return nil, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.Errorf("reading chunk length failed with %d", n),
		}
	}

	// Verify the chunk data end.
	chkDataEnd := chkDataLenStart + n + int(chkDataLen)
	if chkDataEnd > mmapFile.byteSlice.Len() {
		return nil, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.Errorf("head chunk file doesn't include enough bytes to read the chunk - required:%v, available:%v", chkDataEnd, mmapFile.byteSlice.Len()),
		}
	}

	// Check the CRC.
	sum := mmapFile.byteSlice.Range(chkDataEnd, chkDataEnd+CRCSize)
	if _, err := chkCRC32.Write(mmapFile.byteSlice.Range(chkStart-(SeriesRefSize+2*MintMaxtSize), chkDataEnd)); err != nil {
		return nil, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       err,
		}
	}
	if act := chkCRC32.Sum(nil); !bytes.Equal(act, sum) {
		return nil, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.Errorf("checksum mismatch expected:%x, actual:%x", sum, act),
		}
	}

	// The chunk data itself.
	chkData := mmapFile.byteSlice.Range(chkDataEnd-int(chkDataLen), chkDataEnd)

	// Make a copy of the chunk data to prevent a panic occurring because the returned
	// chunk data slice references an mmap-ed file which could be closed after the
	// function returns but while the chunk is still in use.
	chkDataCopy := make([]byte, len(chkData))
	copy(chkDataCopy, chkData)

	chk, err := cdm.pool.Get(chunkenc.Encoding(chkEnc), chkDataCopy)
	if err != nil {
		return nil, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       err,
		}
	}
	return chk, nil
}

// IterateAllChunks iterates all mmappedChunkFiles (in order of head chunk file name/number) and all the chunks within it
// and runs the provided function with information about each chunk. It returns on the first error encountered.
// NOTE: This method needs to be called at least once after creating ChunkDiskMapper
// to set the maxt of all the file.
func (cdm *ChunkDiskMapper) IterateAllChunks(f func(seriesRef HeadSeriesRef, chunkRef ChunkDiskMapperRef, mint, maxt int64, numSamples uint16) error) (err error) {
	cdm.writePathMtx.Lock()
	defer cdm.writePathMtx.Unlock()

	defer func() {
		cdm.fileMaxtSet = true
	}()

	chkCRC32 := newCRC32()

	// Iterate files in ascending order.
	segIDs := make([]int, 0, len(cdm.mmappedChunkFiles))
	for seg := range cdm.mmappedChunkFiles {
		segIDs = append(segIDs, seg)
	}
	sort.Ints(segIDs)
	for _, segID := range segIDs {
		mmapFile := cdm.mmappedChunkFiles[segID]
		fileEnd := mmapFile.byteSlice.Len()
		if segID == cdm.curFileSequence {
			fileEnd = int(cdm.curFileSize())
		}
		idx := HeadChunkFileHeaderSize
		for idx < fileEnd {
			if fileEnd-idx < MaxHeadChunkMetaSize {
				// Check for all 0s which marks the end of the file.
				allZeros := true
				for _, b := range mmapFile.byteSlice.Range(idx, fileEnd) {
					if b != byte(0) {
						allZeros = false
						break
					}
				}
				if allZeros {
					// End of segment chunk file content.
					break
				}
				return &CorruptionErr{
					Dir:       cdm.dir.Name(),
					FileIndex: segID,
					Err: errors.Errorf("head chunk file has some unread data, but doesn't include enough bytes to read the chunk header"+
						" - required:%v, available:%v, file:%d", idx+MaxHeadChunkMetaSize, fileEnd, segID),
				}
			}
			chkCRC32.Reset()
			chunkRef := newChunkDiskMapperRef(uint64(segID), uint64(idx))

			startIdx := idx
			seriesRef := HeadSeriesRef(binary.BigEndian.Uint64(mmapFile.byteSlice.Range(idx, idx+SeriesRefSize)))
			idx += SeriesRefSize
			mint := int64(binary.BigEndian.Uint64(mmapFile.byteSlice.Range(idx, idx+MintMaxtSize)))
			idx += MintMaxtSize
			maxt := int64(binary.BigEndian.Uint64(mmapFile.byteSlice.Range(idx, idx+MintMaxtSize)))
			idx += MintMaxtSize

			// We preallocate file to help with m-mapping (especially windows systems).
			// As series ref always starts from 1, we assume it being 0 to be the end of the actual file data.
			// We are not considering possible file corruption that can cause it to be 0.
			// Additionally we are checking mint and maxt just to be sure.
			if seriesRef == 0 && mint == 0 && maxt == 0 {
				break
			}

			idx += ChunkEncodingSize // Skip encoding.
			dataLen, n := binary.Uvarint(mmapFile.byteSlice.Range(idx, idx+MaxChunkLengthFieldSize))
			idx += n

			numSamples := binary.BigEndian.Uint16(mmapFile.byteSlice.Range(idx, idx+2))
			idx += int(dataLen) // Skip the data.

			// In the beginning we only checked for the chunk meta size.
			// Now that we have added the chunk data length, we check for sufficient bytes again.
			if idx+CRCSize > fileEnd {
				return &CorruptionErr{
					Dir:       cdm.dir.Name(),
					FileIndex: segID,
					Err:       errors.Errorf("head chunk file doesn't include enough bytes to read the chunk header - required:%v, available:%v, file:%d", idx+CRCSize, fileEnd, segID),
				}
			}

			// Check CRC.
			sum := mmapFile.byteSlice.Range(idx, idx+CRCSize)
			if _, err := chkCRC32.Write(mmapFile.byteSlice.Range(startIdx, idx)); err != nil {
				return err
			}
			if act := chkCRC32.Sum(nil); !bytes.Equal(act, sum) {
				return &CorruptionErr{
					Dir:       cdm.dir.Name(),
					FileIndex: segID,
					Err:       errors.Errorf("checksum mismatch expected:%x, actual:%x", sum, act),
				}
			}
			idx += CRCSize

			if maxt > mmapFile.maxt {
				mmapFile.maxt = maxt
			}

			if err := f(seriesRef, chunkRef, mint, maxt, numSamples); err != nil {
				if cerr, ok := err.(*CorruptionErr); ok {
					cerr.Dir = cdm.dir.Name()
					cerr.FileIndex = segID
					return cerr
				}
				return err
			}
		}

		if idx > fileEnd {
			// It should be equal to the slice length.
			return &CorruptionErr{
				Dir:       cdm.dir.Name(),
				FileIndex: segID,
				Err:       errors.Errorf("head chunk file doesn't include enough bytes to read the last chunk data - required:%v, available:%v, file:%d", idx, fileEnd, segID),
			}
		}
	}

	return nil
}

// Truncate deletes the head chunk files which are strictly below the mint.
// mint should be in milliseconds.
func (cdm *ChunkDiskMapper) Truncate(mint int64) error {
	if !cdm.fileMaxtSet {
		return errors.New("maxt of the files are not set")
	}
	cdm.readPathMtx.RLock()

	// Sort the file indices, else if files deletion fails in between,
	// it can lead to unsequential files as the map is not sorted.
	chkFileIndices := make([]int, 0, len(cdm.mmappedChunkFiles))
	for seq := range cdm.mmappedChunkFiles {
		chkFileIndices = append(chkFileIndices, seq)
	}
	sort.Ints(chkFileIndices)

	var removedFiles []int
	for _, seq := range chkFileIndices {
		if seq == cdm.curFileSequence || cdm.mmappedChunkFiles[seq].maxt >= mint {
			break
		}
		if cdm.mmappedChunkFiles[seq].maxt < mint {
			removedFiles = append(removedFiles, seq)
		}
	}
	cdm.readPathMtx.RUnlock()

	errs := tsdb_errors.NewMulti()
	// Cut a new file only if the current file has some chunks.
	if cdm.curFileSize() > HeadChunkFileHeaderSize {
		// There is a known race condition here because between the check of curFileSize() and the call to CutNewFile()
		// a new file could already be cut, this is acceptable because it will simply result in an empty file which
		// won't do any harm.
		cdm.CutNewFile()
	}
	errs.Add(cdm.deleteFiles(removedFiles))
	return errs.Err()
}

func (cdm *ChunkDiskMapper) deleteFiles(removedFiles []int) error {
	cdm.readPathMtx.Lock()
	for _, seq := range removedFiles {
		if err := cdm.closers[seq].Close(); err != nil {
			cdm.readPathMtx.Unlock()
			return err
		}
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

// DeleteCorrupted deletes all the head chunk files after the one which had the corruption
// (including the corrupt file).
func (cdm *ChunkDiskMapper) DeleteCorrupted(originalErr error) error {
	err := errors.Cause(originalErr) // So that we can pick up errors even if wrapped.
	cerr, ok := err.(*CorruptionErr)
	if !ok {
		return errors.Wrap(originalErr, "cannot handle error")
	}

	// Delete all the head chunk files following the corrupt head chunk file.
	segs := []int{}
	cdm.readPathMtx.RLock()
	for seg := range cdm.mmappedChunkFiles {
		if seg >= cerr.FileIndex {
			segs = append(segs, seg)
		}
	}
	cdm.readPathMtx.RUnlock()

	return cdm.deleteFiles(segs)
}

// Size returns the size of the chunk files.
func (cdm *ChunkDiskMapper) Size() (int64, error) {
	return fileutil.DirSize(cdm.dir.Name())
}

func (cdm *ChunkDiskMapper) curFileSize() uint64 {
	return cdm.curFileOffset.Load()
}

// Close closes all the open files in ChunkDiskMapper.
// It is not longer safe to access chunks from this struct after calling Close.
func (cdm *ChunkDiskMapper) Close() error {
	// Locking the eventual position lock blocks WriteChunk()
	cdm.evtlPosMtx.Lock()
	defer cdm.evtlPosMtx.Unlock()

	cdm.writeQueue.stop()

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

	errs := tsdb_errors.NewMulti(
		closeAllFromMap(cdm.closers),
		cdm.finalizeCurFile(),
		cdm.dir.Close(),
	)
	cdm.mmappedChunkFiles = map[int]*mmappedChunkFile{}
	cdm.closers = map[int]io.Closer{}

	return errs.Err()
}

func closeAllFromMap(cs map[int]io.Closer) error {
	errs := tsdb_errors.NewMulti()
	for _, c := range cs {
		errs.Add(c.Close())
	}
	return errs.Err()
}

const inBufferShards = 128 // 128 is a randomly chosen number.

// chunkBuffer is a thread safe lookup table for chunks by their ref.
type chunkBuffer struct {
	inBufferChunks     [inBufferShards]map[ChunkDiskMapperRef]chunkenc.Chunk
	inBufferChunksMtxs [inBufferShards]sync.RWMutex
}

func newChunkBuffer() *chunkBuffer {
	cb := &chunkBuffer{}
	for i := 0; i < inBufferShards; i++ {
		cb.inBufferChunks[i] = make(map[ChunkDiskMapperRef]chunkenc.Chunk)
	}
	return cb
}

func (cb *chunkBuffer) put(ref ChunkDiskMapperRef, chk chunkenc.Chunk) {
	shardIdx := ref % inBufferShards

	cb.inBufferChunksMtxs[shardIdx].Lock()
	cb.inBufferChunks[shardIdx][ref] = chk
	cb.inBufferChunksMtxs[shardIdx].Unlock()
}

func (cb *chunkBuffer) get(ref ChunkDiskMapperRef) chunkenc.Chunk {
	shardIdx := ref % inBufferShards

	cb.inBufferChunksMtxs[shardIdx].RLock()
	defer cb.inBufferChunksMtxs[shardIdx].RUnlock()

	return cb.inBufferChunks[shardIdx][ref]
}

func (cb *chunkBuffer) clear() {
	for i := 0; i < inBufferShards; i++ {
		cb.inBufferChunksMtxs[i].Lock()
		cb.inBufferChunks[i] = make(map[ChunkDiskMapperRef]chunkenc.Chunk)
		cb.inBufferChunksMtxs[i].Unlock()
	}
}
