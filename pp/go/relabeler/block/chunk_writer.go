package block

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
)

const (
	chunksFormatV1 = 1
)

type ChunkMetadata struct {
	MinT int64
	MaxT int64
	Ref  uint64
}

type ChunkWriter struct {
	dirFile     *os.File
	files       []*os.File
	wbuf        *bufio.Writer
	n           int64
	crc32       hash.Hash
	segmentSize int64
	buf         [binary.MaxVarintLen32]byte
}

func NewChunkWriter(dir string, segmentSize int64) (*ChunkWriter, error) {
	if segmentSize < 0 {
		segmentSize = DefaultChunkSegmentSize
	}

	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, fmt.Errorf("failed to create all dirs: %w", err)
	}

	dirFile, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open dir: %w", err)
	}

	return &ChunkWriter{
		dirFile:     dirFile,
		crc32:       crc32.New(crc32.MakeTable(crc32.Castagnoli)),
		segmentSize: segmentSize,
	}, nil
}

func (w *ChunkWriter) Write(chunk Chunk) (meta ChunkMetadata, err error) {
	// calculate chunk size
	chunkSize := int64(chunks.MaxChunkLengthFieldSize)
	chunkSize += chunks.ChunkEncodingSize
	chunkSize += int64(len(chunk.Bytes()))
	chunkSize += crc32.Size

	// check segment boundaries and cut if needed
	if w.n == 0 || w.n+chunkSize > w.segmentSize {
		if err = w.cut(); err != nil {
			return meta, fmt.Errorf("failed to cut file: %w", err)
		}
	}

	// write chunk
	return w.writeChunk(chunk)
}

func (w *ChunkWriter) writeChunk(chunk Chunk) (meta ChunkMetadata, err error) {
	meta.Ref = uint64(chunks.NewBlockChunkRef(uint64(w.seq()), uint64(w.n)))

	n := binary.PutUvarint(w.buf[:], uint64(len(chunk.Bytes())))
	if err = w.write(w.buf[:n]); err != nil {
		return meta, err
	}

	w.buf[0] = byte(chunk.Encoding())
	if err = w.write(w.buf[:1]); err != nil {
		return meta, err
	}

	if err = w.write(chunk.Bytes()); err != nil {
		return meta, err
	}

	w.crc32.Reset()

	buf := append(w.buf[:0], byte(chunk.Encoding()))
	if _, err = w.crc32.Write(buf[:1]); err != nil {
		return meta, err
	}

	if _, err = w.crc32.Write(chunk.Bytes()); err != nil {
		return meta, err
	}

	if err = w.write(w.crc32.Sum(w.buf[:0])); err != nil {
		return meta, err
	}

	meta.MinT = chunk.MinT()
	meta.MaxT = chunk.MaxT()

	return meta, nil
}

func (w *ChunkWriter) Close() (err error) {
	if err = w.finalizeTail(); err != nil {
		return fmt.Errorf("failed to finalize tail on close: %w", err)
	}

	return w.dirFile.Close()
}

func (w *ChunkWriter) write(b []byte) error {
	n, err := w.wbuf.Write(b)
	w.n += int64(n)
	return err
}

func (w *ChunkWriter) seq() int {
	return len(w.files) - 1
}

func (w *ChunkWriter) tail() *os.File {
	if len(w.files) == 0 {
		return nil
	}
	return w.files[len(w.files)-1]
}

// finalizeTail writes all pending data to the current tail file,
// truncates its size, and closes it.
func (w *ChunkWriter) finalizeTail() error {
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

func (w *ChunkWriter) cut() error {
	// Sync current tail to disk and close.
	if err := w.finalizeTail(); err != nil {
		return err
	}

	n, f, _, err := cutSegmentFile(w.dirFile, w.seq(), chunks.MagicChunks, chunksFormatV1, w.segmentSize)
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

func cutSegmentFile(dirFile *os.File, currentSeq int, magicNumber uint32, chunksFormat byte, allocSize int64) (headerSize int, newFile *os.File, seq int, returnErr error) {
	p, seq, err := nextSequenceFile(dirFile.Name(), currentSeq)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("next sequence file: %w", err)
	}
	ptmp := p + ".tmp"
	f, err := os.Create(ptmp)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("open temp file: %w", err)
	}
	defer func() {
		if returnErr != nil {
			if f != nil {
				returnErr = errors.Join(returnErr, f.Close())
			}
			// Calling RemoveAll on a non-existent file does not return error.
			returnErr = errors.Join(returnErr, os.RemoveAll(ptmp))
		}
	}()
	if allocSize > 0 {
		if err = fileutil.Preallocate(f, allocSize, true); err != nil {
			return 0, nil, 0, fmt.Errorf("preallocate: %w", err)
		}
	}

	if err = dirFile.Sync(); err != nil {
		return 0, nil, 0, fmt.Errorf("sync directory: %w", err)
	}

	// Write header metadata for new file.
	metab := make([]byte, chunks.SegmentHeaderSize)
	binary.BigEndian.PutUint32(metab[:chunks.MagicChunksSize], magicNumber)
	metab[4] = chunksFormat

	n, err := f.Write(metab)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("write header: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, nil, 0, fmt.Errorf("close temp file: %w", err)
	}
	f = nil

	if err := fileutil.Rename(ptmp, p); err != nil {
		return 0, nil, 0, fmt.Errorf("replace file: %w", err)
	}

	f, err = os.OpenFile(p, os.O_WRONLY, 0o666)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("open final file: %w", err)
	}
	// Skip header for further writes.
	if _, err := f.Seek(int64(n), 0); err != nil {
		return 0, nil, 0, fmt.Errorf("seek in final file: %w", err)
	}
	return n, f, seq, nil
}

func nextSequenceFile(dir string, currentSeq int) (string, int, error) {
	return segmentFile(dir, currentSeq+1), currentSeq + 1, nil
}

func segmentFile(baseDir string, index int) string {
	return filepath.Join(baseDir, fmt.Sprintf("%0.6d", index))
}
