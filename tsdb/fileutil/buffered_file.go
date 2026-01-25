// Copyright The Prometheus Authors
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

package fileutil

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// BufferedFile provides buffered access to a file with optional caching.
// It implements an interface compatible with byte slice access patterns
// used by chunks and index readers.
type BufferedFile struct {
	f      *os.File
	size   int64
	fileID uint64
	cache  *FileCache

	// Mutex for file operations (pread is thread-safe on most systems,
	// but we need to protect against concurrent close)
	mu     sync.RWMutex
	closed bool
}

// OpenBufferedFile opens a file for buffered reading with optional caching.
// If cache is nil, every read goes directly to disk.
func OpenBufferedFile(path string, cache *FileCache) (*BufferedFile, error) {
	return OpenBufferedFileWithSize(path, 0, cache)
}

// OpenBufferedFileWithSize opens a file for buffered reading with an expected size.
// If size is 0 or negative, the actual file size is used.
func OpenBufferedFileWithSize(path string, size int, cache *FileCache) (*BufferedFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	fileSize := int64(size)
	if fileSize <= 0 {
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("stat: %w", err)
		}
		fileSize = info.Size()
	}

	var fileID uint64
	if cache != nil {
		fileID = cache.NextFileID()
	}

	return &BufferedFile{
		f:      f,
		size:   fileSize,
		fileID: fileID,
		cache:  cache,
	}, nil
}

// Len returns the total size of the file.
func (bf *BufferedFile) Len() int {
	return int(bf.size)
}

// Range returns a byte slice for the given range [start, end).
// The returned slice is a copy and safe to use after the file is closed.
func (bf *BufferedFile) Range(start, end int) []byte {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	if bf.closed {
		return nil
	}

	length := end - start
	if length <= 0 {
		return nil
	}

	// Clamp to file size
	if int64(end) > bf.size {
		end = int(bf.size)
		length = end - start
	}
	if length <= 0 {
		return nil
	}

	result := make([]byte, length)

	if bf.cache != nil {
		bf.readWithCache(start, result)
	} else {
		bf.readDirect(start, result)
	}

	return result
}

// readWithCache reads data using the cache.
func (bf *BufferedFile) readWithCache(offset int, buf []byte) {
	blockSize := bf.cache.BlockSize()
	remaining := len(buf)
	bufOffset := 0

	for remaining > 0 {
		block := int64(offset / blockSize)
		blockStart := int(block) * blockSize
		offsetInBlock := offset - blockStart

		// Try to get from cache
		cached := bf.cache.Get(bf.fileID, block)
		if cached == nil {
			// Cache miss - read the block from disk
			cached = bf.readBlock(block)
			if cached != nil {
				bf.cache.Put(bf.fileID, block, cached)
			}
		}

		if cached == nil {
			// Failed to read, fill with zeros or return partial
			break
		}

		// Copy from cached block to result
		available := len(cached) - offsetInBlock
		if available <= 0 {
			break
		}
		toCopy := remaining
		if toCopy > available {
			toCopy = available
		}

		copy(buf[bufOffset:bufOffset+toCopy], cached[offsetInBlock:offsetInBlock+toCopy])

		bufOffset += toCopy
		offset += toCopy
		remaining -= toCopy
	}
}

// readBlock reads a full block from disk.
func (bf *BufferedFile) readBlock(block int64) []byte {
	blockSize := bf.cache.BlockSize()
	offset := block * int64(blockSize)

	// Determine how much to read
	toRead := int64(blockSize)
	if offset+toRead > bf.size {
		toRead = bf.size - offset
	}
	if toRead <= 0 {
		return nil
	}

	buf := make([]byte, toRead)
	n, err := bf.f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil
	}

	return buf[:n]
}

// readDirect reads data directly from disk without caching.
func (bf *BufferedFile) readDirect(offset int, buf []byte) {
	n, err := bf.f.ReadAt(buf, int64(offset))
	if err != nil && err != io.EOF {
		// Zero out unread portion
		for i := n; i < len(buf); i++ {
			buf[i] = 0
		}
	}
}

// ReadAt implements io.ReaderAt for compatibility.
func (bf *BufferedFile) ReadAt(p []byte, off int64) (n int, err error) {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	if bf.closed {
		return 0, os.ErrClosed
	}

	if bf.cache != nil {
		bf.readWithCache(int(off), p)
		// Determine actual bytes read
		if off+int64(len(p)) > bf.size {
			n = int(bf.size - off)
			if n < 0 {
				n = 0
			}
			if n < len(p) {
				return n, io.EOF
			}
		}
		return len(p), nil
	}

	return bf.f.ReadAt(p, off)
}

// File returns the underlying os.File.
func (bf *BufferedFile) File() *os.File {
	return bf.f
}

// Bytes returns the entire file content as a byte slice.
// WARNING: This loads the entire file into memory.
// Use Range() for large files.
func (bf *BufferedFile) Bytes() []byte {
	return bf.Range(0, int(bf.size))
}

// Close closes the file and invalidates any cached data.
func (bf *BufferedFile) Close() error {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	if bf.closed {
		return nil
	}

	bf.closed = true

	// Invalidate cache entries for this file
	if bf.cache != nil {
		bf.cache.InvalidateFile(bf.fileID)
	}

	return bf.f.Close()
}

// IsClosed returns true if the file has been closed.
func (bf *BufferedFile) IsClosed() bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	return bf.closed
}
