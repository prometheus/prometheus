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
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestBufferedFileReaderCorrectness tests that buffered file reader returns correct data.
func TestBufferedFileReaderCorrectness(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	// Create a file with substantial data that spans multiple cache blocks
	data := make([]byte, DefaultBlockSize*5+12345) // ~5.2 blocks
	for i := range data {
		data[i] = byte((i * 7) % 256)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Test ranges to verify
	testRanges := []struct {
		start, end int
	}{
		{0, 100},                     // Start of file
		{len(data) - 100, len(data)}, // End of file
		{DefaultBlockSize - 50, DefaultBlockSize + 50}, // Cross block boundary
		{0, len(data)}, // Full file
		{1000, 2000},   // Middle of file
		{DefaultBlockSize * 2, DefaultBlockSize*2 + 1000}, // Later block
	}

	// Configure buffered reader
	SetBufferedFileReaderConfig(BufferedFileReaderConfig{
		CacheSize: DefaultCacheSize,
		BlockSize: DefaultBlockSize,
	})

	bufferedReader, err := OpenBufferedFileReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bufferedReader.Close()

	for _, tr := range testRanges {
		bufferedResult := bufferedReader.Range(tr.start, tr.end)
		expected := data[tr.start:tr.end]
		if !bytes.Equal(expected, bufferedResult) {
			t.Errorf("range [%d:%d] doesn't match original data", tr.start, tr.end)
		}
	}

	// Verify cache was used
	hits, misses, _, _ := GlobalCacheStats()
	t.Logf("Cache stats: hits=%d, misses=%d", hits, misses)
	if hits+misses == 0 {
		t.Error("expected cache to be used")
	}
}

// TestBufferedReaderMemoryControl verifies that the cache respects size limits.
func TestBufferedReaderMemoryControl(t *testing.T) {
	dir := t.TempDir()

	// Create multiple files
	numFiles := 10
	fileSize := DefaultBlockSize * 10 // 10 blocks per file
	paths := make([]string, numFiles)

	for i := 0; i < numFiles; i++ {
		path := filepath.Join(dir, "test"+string(rune('0'+i))+".bin")
		data := make([]byte, fileSize)
		for j := range data {
			data[j] = byte(i + j%256)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		paths[i] = path
	}

	// Configure cache to hold only a fraction of total data
	maxCacheSize := int64(DefaultBlockSize * 20) // Only 20 blocks (2 files worth)
	SetBufferedFileReaderConfig(BufferedFileReaderConfig{
		CacheSize: maxCacheSize,
		BlockSize: DefaultBlockSize,
	})

	// Open all files and read from them
	readers := make([]*BufferedFile, numFiles)
	for i, path := range paths {
		r, err := OpenBufferedFileReader(path)
		if err != nil {
			t.Fatal(err)
		}
		readers[i] = r
		// Read the full file to populate cache
		_ = r.Bytes()
	}

	// Check cache size doesn't exceed limit
	cache := GetGlobalCache()
	if cache == nil {
		t.Fatal("expected cache to be initialized")
	}

	if cache.Size() > maxCacheSize {
		t.Errorf("cache size %d exceeds max %d", cache.Size(), maxCacheSize)
	}

	// Clean up
	for _, r := range readers {
		r.Close()
	}
}

// TestCacheInvalidationOnClose verifies that closing a file clears its cache entries.
func TestCacheInvalidationOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	data := make([]byte, DefaultBlockSize*3)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	SetBufferedFileReaderConfig(BufferedFileReaderConfig{
		CacheSize: DefaultCacheSize,
		BlockSize: DefaultBlockSize,
	})

	// Clear any previous cache state
	ClearGlobalCache()

	reader, err := OpenBufferedFileReader(path)
	if err != nil {
		t.Fatal(err)
	}

	// Read to populate cache
	_ = reader.Bytes()

	cache := GetGlobalCache()
	sizeBeforeClose := cache.Size()
	if sizeBeforeClose == 0 {
		t.Error("expected cache to have data before close")
	}

	// Close should invalidate cache
	reader.Close()

	sizeAfterClose := cache.Size()
	if sizeAfterClose != 0 {
		t.Errorf("expected cache to be cleared after close, got size %d", sizeAfterClose)
	}
}
