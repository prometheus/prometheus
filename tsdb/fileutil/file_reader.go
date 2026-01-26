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
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// BufferedFileReaderConfig holds configuration for buffered file reading.
type BufferedFileReaderConfig struct {
	// CacheSize is the maximum cache size in bytes.
	// Default is 512MiB.
	CacheSize int64

	// BlockSize is the size of each cached block.
	// Default is 64KiB.
	BlockSize int

	// Reg is the Prometheus registerer for metrics.
	// If nil, metrics will not be registered.
	Reg prometheus.Registerer
}

// DefaultBufferedFileReaderConfig returns the default configuration.
func DefaultBufferedFileReaderConfig() BufferedFileReaderConfig {
	return BufferedFileReaderConfig{
		CacheSize: DefaultCacheSize,
		BlockSize: DefaultBlockSize,
	}
}

// bufferedFileReaderManager manages shared resources for buffered file reading.
type bufferedFileReaderManager struct {
	mu     sync.RWMutex
	config BufferedFileReaderConfig
	cache  *FileCache
}

var globalManager = &bufferedFileReaderManager{
	config: DefaultBufferedFileReaderConfig(),
}

// SetBufferedFileReaderConfig sets the global buffered file reader configuration.
// This should be called before opening any files, typically during
// database initialization. A new cache is created or updated based on the config.
func SetBufferedFileReaderConfig(cfg BufferedFileReaderConfig) {
	globalManager.mu.Lock()
	defer globalManager.mu.Unlock()

	globalManager.config = cfg

	if globalManager.cache == nil {
		globalManager.cache = NewFileCache(FileCacheOptions{
			MaxSize:   cfg.CacheSize,
			BlockSize: cfg.BlockSize,
			Reg:       cfg.Reg,
		})
	} else {
		// Update cache settings
		globalManager.cache.SetMaxSize(cfg.CacheSize)
	}
}

// GetBufferedFileReaderConfig returns the current global buffered file reader configuration.
func GetBufferedFileReaderConfig() BufferedFileReaderConfig {
	globalManager.mu.RLock()
	defer globalManager.mu.RUnlock()
	return globalManager.config
}

// GetGlobalCache returns the global file cache.
func GetGlobalCache() *FileCache {
	globalManager.mu.RLock()
	defer globalManager.mu.RUnlock()
	return globalManager.cache
}

// ensureCache ensures the global cache is initialized.
func ensureCache() *FileCache {
	globalManager.mu.Lock()
	defer globalManager.mu.Unlock()

	if globalManager.cache == nil {
		globalManager.cache = NewFileCache(FileCacheOptions{
			MaxSize:   globalManager.config.CacheSize,
			BlockSize: globalManager.config.BlockSize,
		})
	}
	return globalManager.cache
}

// OpenBufferedFileReader opens a file for buffered reading with caching.
// This is the primary entry point for opening files in the TSDB.
func OpenBufferedFileReader(path string) (*BufferedFile, error) {
	return OpenBufferedFileReaderWithSize(path, 0)
}

// OpenBufferedFileReaderWithSize opens a file for buffered reading with an expected size.
func OpenBufferedFileReaderWithSize(path string, size int) (*BufferedFile, error) {
	cache := ensureCache()
	return OpenBufferedFileWithSize(path, size, cache)
}

// ClearGlobalCache clears the global file cache.
// This can be useful during testing or when memory pressure is high.
func ClearGlobalCache() {
	globalManager.mu.RLock()
	cache := globalManager.cache
	globalManager.mu.RUnlock()

	if cache != nil {
		cache.Clear()
	}
}

// GlobalCacheStats returns statistics for the global file cache.
// Returns zeros if cache is not initialized.
func GlobalCacheStats() (requests, misses, evictions uint64, size, maxSize int64, numEntries int) {
	globalManager.mu.RLock()
	cache := globalManager.cache
	globalManager.mu.RUnlock()

	if cache != nil {
		return cache.Stats()
	}
	return 0, 0, 0, 0, 0, 0
}
