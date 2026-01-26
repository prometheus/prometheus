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

	"github.com/prometheus/client_golang/prometheus"
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
// It provides configurable memory limits, efficient eviction, and Prometheus metrics.
type FileCache struct {
	mu          sync.RWMutex
	maxSize     int64
	currentSize int64
	blockSize   int
	entries     map[cacheKey]*cacheEntry
	lru         *list.List // front = most recently used

	// Buffer pool for allocating blocks
	pool sync.Pool

	// Metrics - all atomic for lock-free reads
	requests  atomic.Uint64 // Total cache access attempts
	misses    atomic.Uint64 // Cache misses
	evictions atomic.Uint64 // Number of evictions

	// Prometheus metrics
	metrics *fileCacheMetrics

	// File ID counter for unique identification
	nextFileID atomic.Uint64
}

// fileCacheMetrics holds Prometheus metrics for the file cache.
type fileCacheMetrics struct {
	cacheRequests   prometheus.CounterFunc
	cacheMisses     prometheus.CounterFunc
	cacheEvictions  prometheus.CounterFunc
	cacheSize       prometheus.GaugeFunc
	cacheMaxSize    prometheus.GaugeFunc
	cacheEntries    prometheus.GaugeFunc
	cacheUsageRatio prometheus.GaugeFunc
}

// FileCacheOptions configures the file cache.
type FileCacheOptions struct {
	MaxSize   int64                // Maximum cache size in bytes
	BlockSize int                  // Size of each cached block
	Reg       prometheus.Registerer // Prometheus registerer for metrics (optional)
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

	fc.metrics = fc.newMetrics()
	if opts.Reg != nil {
		opts.Reg.MustRegister(fc)
	}

	return fc
}

// newMetrics creates the Prometheus metrics for this cache.
func (fc *FileCache) newMetrics() *fileCacheMetrics {
	return &fileCacheMetrics{
		cacheRequests: prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "prometheus_tsdb_file_cache_requests_total",
			Help: "Total number of file cache access requests.",
		}, func() float64 {
			return float64(fc.requests.Load())
		}),

		cacheMisses: prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "prometheus_tsdb_file_cache_misses_total",
			Help: "Total number of file cache misses.",
		}, func() float64 {
			return float64(fc.misses.Load())
		}),

		cacheEvictions: prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "prometheus_tsdb_file_cache_evictions_total",
			Help: "Total number of file cache evictions.",
		}, func() float64 {
			return float64(fc.evictions.Load())
		}),

		cacheSize: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_file_cache_size_bytes",
			Help: "Current size of the file cache in bytes.",
		}, func() float64 {
			fc.mu.RLock()
			defer fc.mu.RUnlock()
			return float64(fc.currentSize)
		}),

		cacheMaxSize: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_file_cache_max_size_bytes",
			Help: "Maximum configured size of the file cache in bytes.",
		}, func() float64 {
			fc.mu.RLock()
			defer fc.mu.RUnlock()
			return float64(fc.maxSize)
		}),

		cacheEntries: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_file_cache_entries",
			Help: "Current number of entries (blocks) in the file cache.",
		}, func() float64 {
			fc.mu.RLock()
			defer fc.mu.RUnlock()
			return float64(len(fc.entries))
		}),

		cacheUsageRatio: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_file_cache_usage_ratio",
			Help: "Ratio of current cache size to maximum size (0 to 1).",
		}, func() float64 {
			fc.mu.RLock()
			defer fc.mu.RUnlock()
			if fc.maxSize == 0 {
				return 0
			}
			return float64(fc.currentSize) / float64(fc.maxSize)
		}),
	}
}

// Describe implements prometheus.Collector.
func (fc *FileCache) Describe(ch chan<- *prometheus.Desc) {
	if fc.metrics == nil {
		return
	}
	fc.metrics.cacheRequests.Describe(ch)
	fc.metrics.cacheMisses.Describe(ch)
	fc.metrics.cacheEvictions.Describe(ch)
	fc.metrics.cacheSize.Describe(ch)
	fc.metrics.cacheMaxSize.Describe(ch)
	fc.metrics.cacheEntries.Describe(ch)
	fc.metrics.cacheUsageRatio.Describe(ch)
}

// Collect implements prometheus.Collector.
func (fc *FileCache) Collect(ch chan<- prometheus.Metric) {
	if fc.metrics == nil {
		return
	}
	fc.metrics.cacheRequests.Collect(ch)
	fc.metrics.cacheMisses.Collect(ch)
	fc.metrics.cacheEvictions.Collect(ch)
	fc.metrics.cacheSize.Collect(ch)
	fc.metrics.cacheMaxSize.Collect(ch)
	fc.metrics.cacheEntries.Collect(ch)
	fc.metrics.cacheUsageRatio.Collect(ch)
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
	fc.requests.Add(1)
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
	fc.evictions.Add(1)

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
func (fc *FileCache) Stats() (requests, misses, evictions uint64, size, maxSize int64, numEntries int) {
	fc.mu.RLock()
	size = fc.currentSize
	numEntries = len(fc.entries)
	fc.mu.RUnlock()
	return fc.requests.Load(), fc.misses.Load(), fc.evictions.Load(), size, fc.maxSize, numEntries
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
