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

func TestBufferedFile_BasicRead(t *testing.T) {
	// Create a temp file with known content
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	data := make([]byte, 10000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Test without cache
	bf, err := OpenBufferedFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bf.Close()

	if bf.Len() != len(data) {
		t.Errorf("expected len %d, got %d", len(data), bf.Len())
	}

	// Read full content
	got := bf.Bytes()
	if !bytes.Equal(got, data) {
		t.Error("full content mismatch")
	}

	// Read range
	got = bf.Range(100, 200)
	if !bytes.Equal(got, data[100:200]) {
		t.Error("range content mismatch")
	}
}

func TestBufferedFile_WithCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	// Create file larger than block size to test multiple blocks
	data := make([]byte, DefaultBlockSize*3+1000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cache := NewFileCache(FileCacheOptions{
		MaxSize:   DefaultCacheSize,
		BlockSize: DefaultBlockSize,
	})

	bf, err := OpenBufferedFile(path, cache)
	if err != nil {
		t.Fatal(err)
	}
	defer bf.Close()

	// First read - should populate cache
	got := bf.Range(0, 100)
	if !bytes.Equal(got, data[:100]) {
		t.Error("first read mismatch")
	}

	// Check cache stats
	requests, misses, _, _, _, _ := cache.Stats()
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
	if requests != 1 {
		t.Errorf("expected 1 request, got %d", requests)
	}

	// Second read from same block - should hit cache
	got = bf.Range(50, 150)
	if !bytes.Equal(got, data[50:150]) {
		t.Error("second read mismatch")
	}

	requests, misses, _, _, _, _ = cache.Stats()
	// requests should be 2, misses still 1 (so 1 hit)
	if requests != 2 {
		t.Errorf("expected 2 requests, got %d", requests)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss still, got %d", misses)
	}

	// Read spanning multiple blocks
	start := DefaultBlockSize - 100
	end := DefaultBlockSize + 100
	got = bf.Range(start, end)
	if !bytes.Equal(got, data[start:end]) {
		t.Error("cross-block read mismatch")
	}
}

func TestBufferedFile_ReadAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	data := []byte("Hello, World! This is test data.")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	bf, err := OpenBufferedFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bf.Close()

	buf := make([]byte, 5)
	n, err := bf.ReadAt(buf, 7)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("expected to read 5 bytes, got %d", n)
	}
	if string(buf) != "World" {
		t.Errorf("expected 'World', got '%s'", string(buf))
	}
}

func TestBufferedFile_Close(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	data := []byte("test data")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cache := NewFileCache(FileCacheOptions{
		MaxSize:   1024 * 1024,
		BlockSize: 1024,
	})

	bf, err := OpenBufferedFile(path, cache)
	if err != nil {
		t.Fatal(err)
	}

	// Read to populate cache
	bf.Range(0, 5)

	// Close should invalidate cache
	bf.Close()

	// Verify cache is cleared for this file
	// (Size should be 0 if only one file was cached)
	if cache.Size() != 0 {
		t.Errorf("expected cache to be cleared after close, size: %d", cache.Size())
	}

	// Reading after close should return nil
	got := bf.Range(0, 5)
	if got != nil {
		t.Error("expected nil after close")
	}
}

func TestBufferedFile_EdgeCases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	data := []byte("short")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	bf, err := OpenBufferedFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bf.Close()

	// Empty range
	got := bf.Range(2, 2)
	if len(got) != 0 {
		t.Error("expected empty slice for equal start/end")
	}

	// Invalid range (start > end)
	got = bf.Range(3, 2)
	if got != nil {
		t.Error("expected nil for invalid range")
	}

	// Range beyond file size
	got = bf.Range(0, 100)
	if len(got) != len(data) {
		t.Errorf("expected range clamped to file size, got %d bytes", len(got))
	}
}

func TestBufferedFileReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	data := []byte("test data for buffered reader")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Configure the buffered reader
	SetBufferedFileReaderConfig(BufferedFileReaderConfig{
		CacheSize: 1024 * 1024,
		BlockSize: 1024,
	})

	reader, err := OpenBufferedFileReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if reader.Len() != len(data) {
		t.Errorf("expected len %d, got %d", len(data), reader.Len())
	}

	got := reader.Range(0, 4)
	if string(got) != "test" {
		t.Errorf("expected 'test', got '%s'", string(got))
	}

	// Check that we're using the cache
	requests, _, _, size, _, _ := GlobalCacheStats()
	if requests == 0 {
		t.Error("expected cache to be used")
	}
	if size == 0 {
		t.Error("expected cache to have data")
	}
}

func TestBufferedFileReaderConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	data := []byte("test data")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Set custom config
	SetBufferedFileReaderConfig(BufferedFileReaderConfig{
		CacheSize: 512 * 1024,
		BlockSize: 4096,
	})

	reader, err := OpenBufferedFileReader(path)
	if err != nil {
		t.Fatal(err)
	}
	reader.Close()

	// Verify config was applied
	cfg := GetBufferedFileReaderConfig()
	if cfg.CacheSize != 512*1024 {
		t.Errorf("expected CacheSize 512KB, got %d", cfg.CacheSize)
	}
	if cfg.BlockSize != 4096 {
		t.Errorf("expected BlockSize 4096, got %d", cfg.BlockSize)
	}
}
