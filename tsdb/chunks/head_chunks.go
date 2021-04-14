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
	"time"

	"github.com/pkg/errors"
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

var (
	// ErrChunkDiskMapperClosed returned by any method indicates
	// that the ChunkDiskMapper was closed.
	ErrChunkDiskMapperClosed = errors.New("ChunkDiskMapper closed")
)

const (
	// MintMaxtSize is the size of the mint/maxt for head chunk file and chunks.
	MintMaxtSize = 8
	// SeriesRefSize is the size of series reference on disk.
	SeriesRefSize = 8
	// HeadChunkFileHeaderSize is the total size of the header for the head chunk file.
	HeadChunkFileHeaderSize = SegmentHeaderSize
	// MaxHeadChunkFileSize is the max size of a head chunk file.
	MaxHeadChunkFileSize = 128 * 1024 * 1024 // 128 MiB.
	// CRCSize is the size of crc32 sum on disk.
	CRCSize = 4
	// MaxHeadChunkMetaSize is the max size of an mmapped chunks minus the chunks data.
	// Max because the uvarint size can be smaller.
	MaxHeadChunkMetaSize = SeriesRefSize + 2*MintMaxtSize + ChunksFormatVersionSize + MaxChunkLengthFieldSize + CRCSize
	// MinWriteBufferSize is the minimum write buffer size allowed.
	MinWriteBufferSize = 64 * 1024 // 64KB.
	// MaxWriteBufferSize is the maximum write buffer size allowed.
	MaxWriteBufferSize = 8 * 1024 * 1024 // 8 MiB.
	// DefaultWriteBufferSize is the default write buffer size.
	DefaultWriteBufferSize = 4 * 1024 * 1024 // 4 MiB.
)

// CorruptionErr is an error that's returned when corruption is encountered.
type CorruptionErr struct {
	Dir       string
	FileIndex int
	Err       error
}

func (e *CorruptionErr) Error() string {
	return errors.Wrapf(e.Err, "corruption in head chunk file %s", segmentFile(e.Dir, e.FileIndex)).Error()
}

// ChunkDiskMapper is for writing the Head block chunks to the disk
// and access chunks via mmapped file.
type ChunkDiskMapper struct {
	curFileNumBytes atomic.Int64 // Bytes written in current open file.

	/// Writer.
	dir             *os.File
	writeBufferSize int

	curFile         *os.File // File being written to.
	curFileSequence int      // Index of current open file being appended to.
	curFileMaxt     int64    // Used for the size retention.

	byteBuf      [MaxHeadChunkMetaSize]byte // Buffer used to write the header of the chunk.
	chkWriter    *bufio.Writer              // Writer for the current open file.
	crc32        hash.Hash
	writePathMtx sync.Mutex

	/// Reader.
	// The int key in the map is the file number on the disk.
	mmappedChunkFilesMx sync.RWMutex
	mmappedChunkFiles   map[int]*mmappedChunkFile // Contains the m-mapped files for each chunk file mapped with its index.
	pool                chunkenc.Pool             // This is used when fetching a chunk from the disk to allocate a chunk.

	// Writer and Reader.
	// We flush chunks to disk in batches. Hence, we store them in this buffer
	// from which chunks are served till they are flushed and are ready for m-mapping.
	chunkBuffer *chunkBuffer

	// If 'true', it indicated that the maxt of all the on-disk files were set
	// after iterating through all the chunks in those files.
	fileMaxtSet bool

	closed bool

	runClose   chan struct{}
	runRoutine sync.WaitGroup
}

type mmappedChunkFileState int

const (
	// The supported states of mmappedChunkFile.
	active = mmappedChunkFileState(iota)
	deleting
)

type mmappedChunkFile struct {
	byteSlice ByteSlice
	maxt      int64

	// The current state of the mmap-ed chunk file.
	state mmappedChunkFileState

	// Closer for resources behind the byte slice.
	byteSliceCloser io.Closer

	// Keeps track of the number of chunks backed by the byteSlice which are
	// currently in use and not released yet.
	pendingReaders atomic.Int32
}

func (f *mmappedChunkFile) Close() error {
	return f.byteSliceCloser.Close()
}

// NewChunkDiskMapper returns a new writer against the given directory
// using the default head chunk file duration.
// NOTE: 'IterateAllChunks' method needs to be called at least once after creating ChunkDiskMapper
// to set the maxt of all the file.
func NewChunkDiskMapper(dir string, pool chunkenc.Pool, writeBufferSize int) (*ChunkDiskMapper, error) {
	// Validate write buffer size.
	if writeBufferSize < MinWriteBufferSize || writeBufferSize > MaxWriteBufferSize {
		return nil, errors.Errorf("ChunkDiskMapper write buffer size should be between %d and %d (actual: %d)", MinWriteBufferSize, MaxHeadChunkFileSize, writeBufferSize)
	}
	if writeBufferSize%1024 != 0 {
		return nil, errors.Errorf("ChunkDiskMapper write buffer size should be a multiple of 1024 (actual: %d)", writeBufferSize)
	}

	if err := os.MkdirAll(dir, 0777); err != nil {
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
		runClose:        make(chan struct{}),
	}

	if m.pool == nil {
		m.pool = chunkenc.NewPool()
	}

	if err := m.openMMapFiles(); err != nil {
		return nil, err
	}

	// Start a background routine to periodically close and deleted mmap-ed files
	// which are in the deleting state.
	m.runRoutine.Add(1)
	go m.run()

	return m, nil
}

func (cdm *ChunkDiskMapper) openMMapFiles() (returnErr error) {
	cdm.mmappedChunkFiles = map[int]*mmappedChunkFile{}
	defer func() {
		if returnErr != nil {
			returnErr = tsdb_errors.NewMulti(returnErr, closeAllFromMap(cdm.mmappedChunkFiles)).Err()

			cdm.mmappedChunkFiles = nil
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
		cdm.mmappedChunkFiles[seq] = &mmappedChunkFile{
			state:           active,
			byteSlice:       realByteSlice(f.Bytes()),
			byteSliceCloser: f,
		}
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

	return nil
}

func (cdm *ChunkDiskMapper) run() {
	defer cdm.runRoutine.Done()

	for {
		select {
		case <-cdm.runClose:
			return
		case <-time.Tick(time.Minute):
			// TODO log the error
			_ = cdm.safeDeleteTruncatedFiles()
		}
	}
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
// Because we don't fsync when creating these file, we could end
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
func (cdm *ChunkDiskMapper) WriteChunk(seriesRef uint64, mint, maxt int64, chk chunkenc.Chunk) (chkRef uint64, err error) {
	cdm.writePathMtx.Lock()
	defer cdm.writePathMtx.Unlock()

	if cdm.closed {
		return 0, ErrChunkDiskMapperClosed
	}

	if cdm.shouldCutNewFile(len(chk.Bytes())) {
		if err := cdm.cut(); err != nil {
			return 0, err
		}
	}

	// if len(chk.Bytes())+MaxHeadChunkMetaSize >= writeBufferSize, it means that chunk >= the buffer size;
	// so no need to flush here, as we have to flush at the end (to not keep partial chunks in buffer).
	if len(chk.Bytes())+MaxHeadChunkMetaSize < cdm.writeBufferSize && cdm.chkWriter.Available() < MaxHeadChunkMetaSize+len(chk.Bytes()) {
		if err := cdm.flushBuffer(); err != nil {
			return 0, err
		}
	}

	cdm.crc32.Reset()
	bytesWritten := 0

	// The upper 4 bytes are for the head chunk file index and
	// the lower 4 bytes are for the head chunk file offset where to start reading this chunk.
	chkRef = chunkRef(uint64(cdm.curFileSequence), uint64(cdm.curFileSize()))

	binary.BigEndian.PutUint64(cdm.byteBuf[bytesWritten:], seriesRef)
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
		return 0, err
	}
	if err := cdm.writeAndAppendToCRC32(chk.Bytes()); err != nil {
		return 0, err
	}
	if err := cdm.writeCRC32(); err != nil {
		return 0, err
	}

	if maxt > cdm.curFileMaxt {
		cdm.curFileMaxt = maxt
	}

	cdm.chunkBuffer.put(chkRef, chk)

	if len(chk.Bytes())+MaxHeadChunkMetaSize >= cdm.writeBufferSize {
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

// shouldCutNewFile decides the cutting of a new file based on time and size retention.
// Size retention: because depending on the system architecture, there is a limit on how big of a file we can m-map.
// Time retention: so that we can delete old chunks with some time guarantee in low load environments.
func (cdm *ChunkDiskMapper) shouldCutNewFile(chunkSize int) bool {
	return cdm.curFileSize() == 0 || // First head chunk file.
		cdm.curFileSize()+int64(chunkSize+MaxHeadChunkMetaSize) > MaxHeadChunkFileSize // Exceeds the max head chunk file size.
}

// CutNewFile creates a new m-mapped file.
func (cdm *ChunkDiskMapper) CutNewFile() (returnErr error) {
	cdm.writePathMtx.Lock()
	defer cdm.writePathMtx.Unlock()

	return cdm.cut()
}

// cut creates a new m-mapped file. The write lock should be held before calling this.
func (cdm *ChunkDiskMapper) cut() (returnErr error) {
	// Sync current tail to disk and close.
	if err := cdm.finalizeCurFile(); err != nil {
		return err
	}

	n, newFile, seq, err := cutSegmentFile(cdm.dir, MagicHeadChunks, headChunksFormatV1, HeadChunkFilePreallocationSize)
	if err != nil {
		return err
	}
	defer func() {
		// The file should not be closed if there is no error,
		// its kept open in the ChunkDiskMapper.
		if returnErr != nil {
			returnErr = tsdb_errors.NewMulti(returnErr, newFile.Close()).Err()
		}
	}()

	cdm.curFileNumBytes.Store(int64(n))

	if cdm.curFile != nil {
		cdm.mmappedChunkFilesMx.Lock()
		cdm.mmappedChunkFiles[cdm.curFileSequence].maxt = cdm.curFileMaxt
		cdm.mmappedChunkFilesMx.Unlock()
	}

	mmapFile, err := fileutil.OpenMmapFileWithSize(newFile.Name(), int(MaxHeadChunkFileSize))
	if err != nil {
		return err
	}

	cdm.mmappedChunkFilesMx.Lock()
	cdm.curFileSequence = seq
	cdm.curFile = newFile
	if cdm.chkWriter != nil {
		cdm.chkWriter.Reset(newFile)
	} else {
		cdm.chkWriter = bufio.NewWriterSize(newFile, cdm.writeBufferSize)
	}

	cdm.mmappedChunkFiles[cdm.curFileSequence] = &mmappedChunkFile{
		state:           active,
		byteSlice:       realByteSlice(mmapFile.Bytes()),
		byteSliceCloser: mmapFile,
	}
	cdm.mmappedChunkFilesMx.Unlock()

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

	if err := cdm.curFile.Sync(); err != nil {
		return err
	}

	return cdm.curFile.Close()
}

func (cdm *ChunkDiskMapper) write(b []byte) error {
	n, err := cdm.chkWriter.Write(b)
	cdm.curFileNumBytes.Add(int64(n))
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

// GetChunk returns a chunk from a given reference and the reference to use to release
// it once done.
func (cdm *ChunkDiskMapper) GetChunk(ref uint64) (chk chunkenc.Chunk, chkReleaseRef uint64, err error) {
	cdm.mmappedChunkFilesMx.RLock()
	// We hold this read lock for the entire duration because if the Close()
	// is called, the data in the byte slice will get corrupted as the mmapped
	// file will be closed.
	defer cdm.mmappedChunkFilesMx.RUnlock()

	var (
		// Get the upper 4 bytes.
		// These contain the head chunk file index.
		sgmIndex = int(ref >> 32)
		// Get the lower 4 bytes.
		// These contain the head chunk file offset where the chunk starts.
		// We skip the series ref and the mint/maxt beforehand.
		chkStart = int((ref<<32)>>32) + SeriesRefSize + (2 * MintMaxtSize)
		chkCRC32 = newCRC32()
	)

	if cdm.closed {
		return nil, 0, ErrChunkDiskMapperClosed
	}

	// If it is the current open file, then the chunks can be in the buffer too.
	if sgmIndex == cdm.curFileSequence {
		chunk := cdm.chunkBuffer.get(ref)
		if chunk != nil {
			// The chunk is read from the in-memory buffer, so there's no need to release it.
			return chunk, 0, nil
		}
	}

	mmapFile, ok := cdm.mmappedChunkFiles[sgmIndex]
	if !ok {
		if sgmIndex > cdm.curFileSequence {
			return nil, 0, &CorruptionErr{
				Dir:       cdm.dir.Name(),
				FileIndex: -1,
				Err:       errors.Errorf("head chunk file index %d more than current open file", sgmIndex),
			}
		}
		return nil, 0, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.New("head chunk file index %d does not exist on disk"),
		}
	}

	// Ensure the file is not in the deleting state. This should never happen unless a bug
	// because references for chunks in deleting files are expected to have been already
	// removed from the caller.
	if mmapFile.state == deleting {
		return nil, 0, errors.Errorf("head chunk file index %d is in deleting state", sgmIndex)
	}

	if chkStart+MaxChunkLengthFieldSize > mmapFile.byteSlice.Len() {
		return nil, 0, &CorruptionErr{
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
		return nil, 0, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.Errorf("reading chunk length failed with %d", n),
		}
	}

	// Verify the chunk data end.
	chkDataEnd := chkDataLenStart + n + int(chkDataLen)
	if chkDataEnd > mmapFile.byteSlice.Len() {
		return nil, 0, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.Errorf("head chunk file doesn't include enough bytes to read the chunk - required:%v, available:%v", chkDataEnd, mmapFile.byteSlice.Len()),
		}
	}

	// Check the CRC.
	sum := mmapFile.byteSlice.Range(chkDataEnd, chkDataEnd+CRCSize)
	if _, err := chkCRC32.Write(mmapFile.byteSlice.Range(chkStart-(SeriesRefSize+2*MintMaxtSize), chkDataEnd)); err != nil {
		return nil, 0, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       err,
		}
	}
	if act := chkCRC32.Sum(nil); !bytes.Equal(act, sum) {
		return nil, 0, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       errors.Errorf("checksum mismatch expected:%x, actual:%x", sum, act),
		}
	}

	// The chunk data itself.
	chkData := mmapFile.byteSlice.Range(chkDataEnd-int(chkDataLen), chkDataEnd)
	chk, err = cdm.pool.Get(chunkenc.Encoding(chkEnc), chkData)
	if err != nil {
		return nil, 0, &CorruptionErr{
			Dir:       cdm.dir.Name(),
			FileIndex: sgmIndex,
			Err:       err,
		}
	}

	// Keep track of the pending reader.
	mmapFile.pendingReaders.Inc()

	// The chunk is read from the mmap-ed file so must be released once done, because we can't unmmap
	// chunk files while their byte slice is still in use.
	return chk, ref, nil
}

// ReleaseChunks releases a chunk previously obtained calling GetChunk(). This function must be called
// for each chunk returned by GetChunk() which has been flagged to be released.
func (cdm *ChunkDiskMapper) ReleaseChunks(refs ...uint64) {
	// Count the number of references by chunk file.
	countBySgm := map[int]int32{}
	for _, ref := range refs {
		// The reference 0 is a special value we use for chunk which don't need to be
		// released, so we're just going to skip it.
		if ref == 0 {
			continue
		}

		// Get the upper 4 bytes. These contain the head chunk file index.
		sgmIndex := int(ref >> 32)
		countBySgm[sgmIndex]++
	}

	// Decrease the number of pending readers for each chunk file.
	cdm.mmappedChunkFilesMx.RLock()
	defer cdm.mmappedChunkFilesMx.RUnlock()

	for sgmIndex, count := range countBySgm {
		mmapFile := cdm.mmappedChunkFiles[sgmIndex]
		if mmapFile == nil {
			// If this happens then it's a bug.
			// TODO log it
			continue
		}

		newPendingReaders := mmapFile.pendingReaders.Sub(count)
		if newPendingReaders < 0 {
			// If this happens then it's a bug.
			// TODO log it
		}
	}
}

// IterateAllChunks iterates on all the chunks in its byte slices in the order of the head chunk file sequence
// and runs the provided function on each chunk. It returns on the first error encountered.
// NOTE: This method needs to be called at least once after creating ChunkDiskMapper
// to set the maxt of all the file.
func (cdm *ChunkDiskMapper) IterateAllChunks(f func(seriesRef, chunkRef uint64, mint, maxt int64, numSamples uint16) error) (err error) {
	// TODO why doesn't take a read lock too?
	cdm.writePathMtx.Lock()
	defer cdm.writePathMtx.Unlock()

	defer func() {
		cdm.fileMaxtSet = true
	}()

	chkCRC32 := newCRC32()

	// Iterate files in ascending order.
	segIDs := make([]int, 0, len(cdm.mmappedChunkFiles))
	for seg, file := range cdm.mmappedChunkFiles {
		// Skip deleting files.
		// TODO possible race condition: since this function doesn't take the read lock,
		// we may change the file.state to deleting while running this function.
		if file.state == deleting {
			continue
		}

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
			chunkRef := chunkRef(uint64(segID), uint64(idx))

			startIdx := idx
			seriesRef := binary.BigEndian.Uint64(mmapFile.byteSlice.Range(idx, idx+SeriesRefSize))
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
	cdm.mmappedChunkFilesMx.RLock()

	// Sort the file indices, else if files deletion fails in between,
	// it can lead to unsequential files as the map is not sorted.
	chkFileIndices := make([]int, 0, len(cdm.mmappedChunkFiles))
	for seq := range cdm.mmappedChunkFiles {
		chkFileIndices = append(chkFileIndices, seq)
	}
	sort.Ints(chkFileIndices)

	var removedFiles []int
	for _, seq := range chkFileIndices {
		if cdm.mmappedChunkFiles[seq].state == deleting {
			// We should skip all files in deleting state because it means they've already
			// been truncated but the deletion is still in progress due to pending readers.
			continue
		}
		if seq == cdm.curFileSequence || cdm.mmappedChunkFiles[seq].maxt >= mint {
			break
		}

		removedFiles = append(removedFiles, seq)
	}
	cdm.mmappedChunkFilesMx.RUnlock()

	errs := tsdb_errors.NewMulti()

	// Cut a new file only if the current file has some chunks.
	if cdm.curFileSize() > HeadChunkFileHeaderSize {
		errs.Add(cdm.CutNewFile())
	}

	// Change the file state to deleting but do not immediately  delete it because
	// there may be pending readers (will be checked later).
	// Takes an exclusive lock to change the state, in order to guarantee no other
	// goroutine will concurrently read the state.
	cdm.mmappedChunkFilesMx.Lock()
	for _, seq := range removedFiles {
		cdm.mmappedChunkFiles[seq].state = deleting
	}
	cdm.mmappedChunkFilesMx.Unlock()

	// Try to immediately delete files marked as deleting.
	errs.Add(cdm.safeDeleteTruncatedFiles())

	return errs.Err()
}

// safeDeleteTruncatedFiles checks if mmap-ed chunk files in the deleting state are safe
// to be deleted and, if so, removes them from the map of open files and delete them.
func (cdm *ChunkDiskMapper) safeDeleteTruncatedFiles() error {
	var (
		lowestID     = math.MaxInt32
		deletableIDs []int
	)

	// Find the file indices what are in deleting state and safe to be deleted
	// because there are no more pending readers.
	cdm.mmappedChunkFilesMx.RLock()
	for seq, file := range cdm.mmappedChunkFiles {
		if seq < lowestID {
			lowestID = seq
		}

		if file.state == deleting && file.pendingReaders.Load() <= 0 {
			deletableIDs = append(deletableIDs, seq)
		}
	}
	cdm.mmappedChunkFilesMx.RUnlock()

	// Return if there's nothing to do.
	if len(deletableIDs) == 0 {
		return nil
	}

	// We must guarantee to delete chunk files in order, otherwise it can lead
	// to unsequential files. For this reason, we look for consecutive sequence IDs
	// starting from lowest one.
	deleteIDs := make([]int, 0, len(deletableIDs))
	sort.Ints(deletableIDs)
	for _, id := range deletableIDs {
		if id != lowestID {
			break
		}

		deleteIDs = append(deleteIDs, id)
		lowestID = id + 1
	}

	return cdm.deleteFiles(deleteIDs)
}

// deleteFiles closes and deletes the provided chunk files, referenced by the provided
// seq number. This function doesn't check if the files are safe to be deleted, cause such
// check is expected to be done by the caller. Breaks on first error encountered.
func (cdm *ChunkDiskMapper) deleteFiles(seqs []int) error {
	for _, seq := range seqs {
		if err := cdm.deleteFile(seq, true); err != nil {
			return err
		}
	}
	return nil
}

// deleteFile closes and deletes a single mmap-ed chunk file, referenced by the provided
// seq number. This function doesn't check if the file is safe to be deleted, cause such
// check is expected to be done by the caller.
func (cdm *ChunkDiskMapper) deleteFile(seq int, lock bool) error {
	mx := cdm.mmappedChunkFilesMx
	if !lock {
		// If we shouldn't lock, we just lock a new mutex to keep the rest
		// of this function logic easier. This doesn't not run on the hot path.
		mx = sync.RWMutex{}
	}

	// Close the mmap-ed chunk file.
	mx.Lock()
	if err := cdm.mmappedChunkFiles[seq].Close(); err != nil {
		mx.Unlock()
		return err
	}
	delete(cdm.mmappedChunkFiles, seq)
	mx.Unlock()

	// We actually delete the files separately to not block the readPathMtx for long.
	return os.Remove(segmentFile(cdm.dir.Name(), seq))
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
	cdm.mmappedChunkFilesMx.RLock()
	for seg, file := range cdm.mmappedChunkFiles {
		if file.state != deleting && seg >= cerr.FileIndex {
			segs = append(segs, seg)
		}
	}
	cdm.mmappedChunkFilesMx.RUnlock()

	// Immediately delete chunk files without going through the deleting state
	// because corrupted.
	return cdm.deleteFiles(segs)
}

// Size returns the size of the chunk files.
func (cdm *ChunkDiskMapper) Size() (int64, error) {
	return fileutil.DirSize(cdm.dir.Name())
}

func (cdm *ChunkDiskMapper) curFileSize() int64 {
	return cdm.curFileNumBytes.Load()
}

// Close closes all the open files in ChunkDiskMapper.
// It is not longer safe to access chunks from this struct after calling Close.
func (cdm *ChunkDiskMapper) Close() error {
	// Stop the run loop and wait until done.
	// TODO should be done after the check if already closed, but we cannot do it
	// after taking the lock otherwise we may endup in a deadlock.
	close(cdm.runClose)
	cdm.runRoutine.Wait()

	// 'WriteChunk' locks writePathMtx first and then readPathMtx for cutting head chunk file.
	// The lock order should not be reversed here else it can cause deadlocks.
	cdm.writePathMtx.Lock()
	defer cdm.writePathMtx.Unlock()
	cdm.mmappedChunkFilesMx.Lock()
	defer cdm.mmappedChunkFilesMx.Unlock()

	if cdm.closed {
		return nil
	}
	cdm.closed = true

	errs := tsdb_errors.NewMulti()

	// Delete all chunk files in deleting state, regardless they have pending readers.
	for seq, file := range cdm.mmappedChunkFiles {
		if file.state == deleting {
			errs.Add(cdm.deleteFile(seq, false))
		}
	}

	errs.Add(
		closeAllFromMap(cdm.mmappedChunkFiles),
		cdm.finalizeCurFile(),
		cdm.dir.Close(),
	)
	cdm.mmappedChunkFiles = map[int]*mmappedChunkFile{}

	return errs.Err()
}

func closeAllFromMap(files map[int]*mmappedChunkFile) error {
	errs := tsdb_errors.NewMulti()
	for _, file := range files {
		errs.Add(file.Close())
	}
	return errs.Err()
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
