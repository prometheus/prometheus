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
	"sync"
	"testing"
)

func TestFileCache_BasicOperations(t *testing.T) {
	cache := NewFileCache(FileCacheOptions{
		MaxSize:   1024 * 1024, // 1MB
		BlockSize: 1024,        // 1KB blocks
	})

	// Test put and get
	fileID := cache.NextFileID()
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	cache.Put(fileID, 0, data)

	got := cache.Get(fileID, 0)
	if got == nil {
		t.Fatal("expected to get cached data, got nil")
	}
	if !bytes.Equal(got, data) {
		t.Error("cached data doesn't match original")
	}

	// Test cache miss
	got = cache.Get(fileID, 1)
	if got != nil {
		t.Error("expected nil for uncached block")
	}

	// Test stats
	requests, misses, _, size, maxSize, _ := cache.Stats()
	if requests != 2 {
		t.Errorf("expected 2 requests, got %d", requests)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
	if size != 1024 {
		t.Errorf("expected size 1024, got %d", size)
	}
	if maxSize != 1024*1024 {
		t.Errorf("expected maxSize 1MB, got %d", maxSize)
	}
}

func TestFileCache_Eviction(t *testing.T) {
	// Create cache that can hold exactly 2 blocks
	cache := NewFileCache(FileCacheOptions{
		MaxSize:   2048,
		BlockSize: 1024,
	})

	fileID := cache.NextFileID()
	data := make([]byte, 1024)

	// Add 3 blocks - should evict the first one
	for i := 0; i < 3; i++ {
		for j := range data {
			data[j] = byte(i)
		}
		cache.Put(fileID, int64(i), data)
	}

	// Block 0 should be evicted (LRU)
	got := cache.Get(fileID, 0)
	if got != nil {
		t.Error("expected block 0 to be evicted")
	}

	// Blocks 1 and 2 should still be present
	got = cache.Get(fileID, 1)
	if got == nil {
		t.Error("expected block 1 to be present")
	}
	got = cache.Get(fileID, 2)
	if got == nil {
		t.Error("expected block 2 to be present")
	}
}

func TestFileCache_LRUOrder(t *testing.T) {
	cache := NewFileCache(FileCacheOptions{
		MaxSize:   3072, // 3 blocks
		BlockSize: 1024,
	})

	fileID := cache.NextFileID()
	data := make([]byte, 1024)

	// Add 3 blocks
	for i := 0; i < 3; i++ {
		cache.Put(fileID, int64(i), data)
	}

	// Access block 0 to make it most recently used
	cache.Get(fileID, 0)

	// Add block 3 - should evict block 1 (now LRU)
	cache.Put(fileID, 3, data)

	// Block 1 should be evicted
	if cache.Get(fileID, 1) != nil {
		t.Error("expected block 1 to be evicted")
	}
	// Blocks 0, 2, 3 should still be present
	if cache.Get(fileID, 0) == nil {
		t.Error("expected block 0 to be present")
	}
	if cache.Get(fileID, 2) == nil {
		t.Error("expected block 2 to be present")
	}
	if cache.Get(fileID, 3) == nil {
		t.Error("expected block 3 to be present")
	}
}

func TestFileCache_InvalidateFile(t *testing.T) {
	cache := NewFileCache(FileCacheOptions{
		MaxSize:   1024 * 1024,
		BlockSize: 1024,
	})

	fileID1 := cache.NextFileID()
	fileID2 := cache.NextFileID()
	data := make([]byte, 1024)

	// Add blocks for two files
	cache.Put(fileID1, 0, data)
	cache.Put(fileID1, 1, data)
	cache.Put(fileID2, 0, data)
	cache.Put(fileID2, 1, data)

	// Invalidate file 1
	cache.InvalidateFile(fileID1)

	// File 1 blocks should be gone
	if cache.Get(fileID1, 0) != nil {
		t.Error("expected file1 block 0 to be invalidated")
	}
	if cache.Get(fileID1, 1) != nil {
		t.Error("expected file1 block 1 to be invalidated")
	}

	// File 2 blocks should still be present
	if cache.Get(fileID2, 0) == nil {
		t.Error("expected file2 block 0 to be present")
	}
	if cache.Get(fileID2, 1) == nil {
		t.Error("expected file2 block 1 to be present")
	}
}

func TestFileCache_Clear(t *testing.T) {
	cache := NewFileCache(FileCacheOptions{
		MaxSize:   1024 * 1024,
		BlockSize: 1024,
	})

	fileID := cache.NextFileID()
	data := make([]byte, 1024)
	cache.Put(fileID, 0, data)
	cache.Put(fileID, 1, data)

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}
	if cache.Get(fileID, 0) != nil {
		t.Error("expected nil after clear")
	}
}

func TestFileCache_ConcurrentAccess(t *testing.T) {
	cache := NewFileCache(FileCacheOptions{
		MaxSize:   1024 * 1024,
		BlockSize: 1024,
	})

	fileID := cache.NextFileID()
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	var wg sync.WaitGroup
	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(block int64) {
			defer wg.Done()
			cache.Put(fileID, block, data)
		}(int64(i % 10))
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(block int64) {
			defer wg.Done()
			cache.Get(fileID, block)
		}(int64(i % 10))
	}

	wg.Wait()
}

func TestFileCache_SetMaxSize(t *testing.T) {
	cache := NewFileCache(FileCacheOptions{
		MaxSize:   4096, // 4 blocks
		BlockSize: 1024,
	})

	fileID := cache.NextFileID()
	data := make([]byte, 1024)

	// Fill cache with 4 blocks
	for i := 0; i < 4; i++ {
		cache.Put(fileID, int64(i), data)
	}

	if cache.Size() != 4096 {
		t.Errorf("expected size 4096, got %d", cache.Size())
	}

	// Reduce max size - should evict blocks
	cache.SetMaxSize(2048)

	if cache.Size() > 2048 {
		t.Errorf("expected size <= 2048, got %d", cache.Size())
	}
}
