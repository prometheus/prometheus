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
	"container/list"
	"sync"
	"sync/atomic"
)

const (
	// DefaultBlockSize is the size of each cached block (64KiB).
	// This is chosen to balance between cache granularity and overhead.
	DefaultBlockSize = 64 * 1024

	// DefaultCacheSize is the default maximum cache size (512MiB).
	DefaultCacheSize = 512 * 1024 * 1024
)

// cacheKey uniquely identifies a block in the cache.
type cacheKey struct {
	fileID uint64
	block  int64 // block index (offset / blockSize)
}

// cacheEntry holds the cached data and its position in the LRU list.
type cacheEntry struct {
	key  cacheKey
	data []byte
	elem *list.Element
	size int // actual size of data (may be less than blockSize for last block)
}

// FileCache is a shared LRU cache for file blocks.
// It provides configurable memory limits and efficient eviction.
type FileCache struct {
	mu          sync.RWMutex
	maxSize     int64
	currentSize int64
	blockSize   int
	entries     map[cacheKey]*cacheEntry
	lru         *list.List // front = most recently used

	// Buffer pool for allocating blocks
	pool sync.Pool

	// Metrics
	hits   atomic.Uint64
	misses atomic.Uint64

	// File ID counter for unique identification
	nextFileID atomic.Uint64
}

// FileCacheOptions configures the file cache.
type FileCacheOptions struct {
	MaxSize   int64 // Maximum cache size in bytes
	BlockSize int   // Size of each cached block
}

// DefaultFileCacheOptions returns the default cache configuration.
func DefaultFileCacheOptions() FileCacheOptions {
	return FileCacheOptions{
		MaxSize:   DefaultCacheSize,
		BlockSize: DefaultBlockSize,
	}
}

// NewFileCache creates a new file cache with the given options.
func NewFileCache(opts FileCacheOptions) *FileCache {
	if opts.MaxSize <= 0 {
		opts.MaxSize = DefaultCacheSize
	}
	if opts.BlockSize <= 0 {
		opts.BlockSize = DefaultBlockSize
	}

	fc := &FileCache{
		maxSize:   opts.MaxSize,
		blockSize: opts.BlockSize,
		entries:   make(map[cacheKey]*cacheEntry),
		lru:       list.New(),
	}

	fc.pool = sync.Pool{
		New: func() interface{} {
			return make([]byte, fc.blockSize)
		},
	}

	return fc
}

// NextFileID returns a unique file ID for cache key generation.
func (fc *FileCache) NextFileID() uint64 {
	return fc.nextFileID.Add(1)
}

// BlockSize returns the configured block size.
func (fc *FileCache) BlockSize() int {
	return fc.blockSize
}

// Get retrieves a block from the cache.
// Returns nil if the block is not cached.
func (fc *FileCache) Get(fileID uint64, block int64) []byte {
	key := cacheKey{fileID: fileID, block: block}

	fc.mu.RLock()
	entry, ok := fc.entries[key]
	fc.mu.RUnlock()

	if !ok {
		fc.misses.Add(1)
		return nil
	}

	// Move to front (most recently used)
	fc.mu.Lock()
	// Re-check after acquiring write lock
	entry, ok = fc.entries[key]
	if ok {
		fc.lru.MoveToFront(entry.elem)
	}
	fc.mu.Unlock()

	if ok {
		fc.hits.Add(1)
		return entry.data[:entry.size]
	}

	fc.misses.Add(1)
	return nil
}

// Put adds a block to the cache.
// If the cache is full, the least recently used blocks are evicted.
func (fc *FileCache) Put(fileID uint64, block int64, data []byte) {
	key := cacheKey{fileID: fileID, block: block}
	dataSize := len(data)

	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Check if already exists
	if entry, ok := fc.entries[key]; ok {
		// Update existing entry
		fc.lru.MoveToFront(entry.elem)
		// If size changed, update
		if entry.size != dataSize {
			fc.currentSize += int64(dataSize - entry.size)
			entry.size = dataSize
			copy(entry.data, data)
		}
		return
	}

	// Evict if necessary
	for fc.currentSize+int64(fc.blockSize) > fc.maxSize && fc.lru.Len() > 0 {
		fc.evictLocked()
	}

	// Allocate from pool
	buf := fc.pool.Get().([]byte)
	copy(buf, data)

	entry := &cacheEntry{
		key:  key,
		data: buf,
		size: dataSize,
	}
	entry.elem = fc.lru.PushFront(entry)
	fc.entries[key] = entry
	fc.currentSize += int64(fc.blockSize) // Account for full block allocation
}

// evictLocked removes the least recently used entry.
// Caller must hold fc.mu.
func (fc *FileCache) evictLocked() {
	elem := fc.lru.Back()
	if elem == nil {
		return
	}

	entry := elem.Value.(*cacheEntry)
	fc.lru.Remove(elem)
	delete(fc.entries, entry.key)
	fc.currentSize -= int64(fc.blockSize)

	// Return buffer to pool
	fc.pool.Put(entry.data)
}

// InvalidateFile removes all cached blocks for a specific file.
// Call this when a file is closed or deleted.
func (fc *FileCache) InvalidateFile(fileID uint64) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Collect entries to remove
	var toRemove []*cacheEntry
	for key, entry := range fc.entries {
		if key.fileID == fileID {
			toRemove = append(toRemove, entry)
		}
	}

	// Remove them
	for _, entry := range toRemove {
		fc.lru.Remove(entry.elem)
		delete(fc.entries, entry.key)
		fc.currentSize -= int64(fc.blockSize)
		fc.pool.Put(entry.data)
	}
}

// Clear removes all entries from the cache.
func (fc *FileCache) Clear() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	for _, entry := range fc.entries {
		fc.pool.Put(entry.data)
	}

	fc.entries = make(map[cacheKey]*cacheEntry)
	fc.lru.Init()
	fc.currentSize = 0
}

// Stats returns cache statistics.
func (fc *FileCache) Stats() (hits, misses uint64, size, maxSize int64) {
	fc.mu.RLock()
	size = fc.currentSize
	fc.mu.RUnlock()
	return fc.hits.Load(), fc.misses.Load(), size, fc.maxSize
}

// Size returns the current cache size in bytes.
func (fc *FileCache) Size() int64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.currentSize
}

// SetMaxSize updates the maximum cache size and evicts entries if necessary.
func (fc *FileCache) SetMaxSize(maxSize int64) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.maxSize = maxSize
	for fc.currentSize > fc.maxSize && fc.lru.Len() > 0 {
		fc.evictLocked()
	}
}
