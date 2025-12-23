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

package seriesmetadata

import (
	"fmt"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/model/metadata"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestWriteAndReadbackSeriesMetadata(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add some test metadata - map metric name to (hash, metadata)
	type testEntry struct {
		hash uint64
		meta metadata.Metadata
	}
	testData := map[string]testEntry{
		"http_requests_total":     {1, metadata.Metadata{Type: model.MetricTypeCounter, Unit: "bytes", Help: "Total bytes transferred"}},
		"current_time_seconds":    {2, metadata.Metadata{Type: model.MetricTypeGauge, Unit: "seconds", Help: "Current time"}},
		"request_latency":         {3, metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "", Help: "Request latency histogram"}},
		"request_latency_summary": {4, metadata.Metadata{Type: model.MetricTypeSummary, Unit: "requests", Help: "Summary of requests"}},
	}

	for name, entry := range testData {
		mem.Set(name, entry.hash, entry.meta)
	}

	// Write to file
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size)

	// Read back
	reader, readSize, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	defer reader.Close()
	require.Equal(t, size, readSize)

	// Verify all metadata was read back correctly by metric name
	for name, entry := range testData {
		meta, found := reader.GetByMetricName(name)
		require.True(t, found, "metadata for metric %s not found", name)
		require.Equal(t, entry.meta.Type, meta.Type)
		require.Equal(t, entry.meta.Unit, meta.Unit)
		require.Equal(t, entry.meta.Help, meta.Help)
	}

	// Verify total count
	require.Equal(t, uint64(len(testData)), reader.Total())
}

func TestReadNonexistentFile(t *testing.T) {
	tmpdir := t.TempDir()

	// Reading from a directory without metadata file should return empty reader
	reader, size, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	require.Equal(t, int64(0), size)
	require.Equal(t, uint64(0), reader.Total())

	// Get should return not found
	_, found := reader.GetByMetricName("nonexistent")
	require.False(t, found)

	require.NoError(t, reader.Close())
}

func TestWriteEmptyMetadata(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Write empty metadata
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size) // Even empty parquet has header

	// Read back
	reader, _, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(0), reader.Total())
}

func TestMemSeriesMetadataBasicOperations(t *testing.T) {
	mem := NewMemSeriesMetadata()

	// Test Set and Get by metric name
	meta := metadata.Metadata{
		Type: model.MetricTypeCounter,
		Unit: "bytes",
		Help: "Test metric",
	}
	mem.Set("test_metric", 123, meta)

	got, found := mem.GetByMetricName("test_metric")
	require.True(t, found)
	require.Equal(t, meta, got)

	// Also verify Get by hash works
	got, found = mem.Get(123)
	require.True(t, found)
	require.Equal(t, meta, got)

	// Test Get non-existent
	_, found = mem.GetByMetricName("nonexistent")
	require.False(t, found)
	_, found = mem.Get(999)
	require.False(t, found)

	// Test Delete
	mem.Delete(123)
	_, found = mem.Get(123)
	require.False(t, found)
	_, found = mem.GetByMetricName("test_metric")
	require.False(t, found)

	// Test Total
	mem.Set("metric_a", 1, meta)
	mem.Set("metric_b", 2, meta)
	mem.Set("metric_c", 3, meta)
	require.Equal(t, uint64(3), mem.Total())

	// Test Close (should be no-op)
	require.NoError(t, mem.Close())
}

func TestMemSeriesMetadataIter(t *testing.T) {
	mem := NewMemSeriesMetadata()

	testData := map[string]metadata.Metadata{
		"counter_metric":   {Type: model.MetricTypeCounter, Help: "Counter metric"},
		"gauge_metric":     {Type: model.MetricTypeGauge, Help: "Gauge metric"},
		"histogram_metric": {Type: model.MetricTypeHistogram, Help: "Histogram metric"},
	}

	hash := uint64(1)
	for name, meta := range testData {
		mem.Set(name, hash, meta)
		hash++
	}

	// Iterate by metric name and collect
	collected := make(map[string]metadata.Metadata)
	err := mem.IterByMetricName(func(name string, meta metadata.Metadata) error {
		collected[name] = meta
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, testData, collected)

	// Also test Iter by hash
	hashCount := 0
	err = mem.Iter(func(_ uint64, _ metadata.Metadata) error {
		hashCount++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, len(testData), hashCount)
}

func TestSeriesMetadataOverwrite(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()
	meta1 := metadata.Metadata{Type: model.MetricTypeCounter, Help: "First version"}
	meta2 := metadata.Metadata{Type: model.MetricTypeGauge, Help: "Second version"}

	// Set initial value
	mem.Set("test_metric", 100, meta1)
	got, _ := mem.GetByMetricName("test_metric")
	require.Equal(t, meta1, got)

	// Overwrite with same metric name but different hash
	mem.Set("test_metric", 200, meta2)
	got, _ = mem.GetByMetricName("test_metric")
	require.Equal(t, meta2, got)

	// Old hash should be gone, new hash should work
	_, found := mem.Get(100)
	require.False(t, found, "old hash should be removed")
	got, found = mem.Get(200)
	require.True(t, found)
	require.Equal(t, meta2, got)

	// Write and read back - verify overwrite persisted
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	got, found = reader.GetByMetricName("test_metric")
	require.True(t, found)
	require.Equal(t, meta2, got)
}

func TestSetWithZeroHash(t *testing.T) {
	mem := NewMemSeriesMetadata()

	// Simulate compaction/merge pattern: multiple metrics with hash=0
	meta1 := metadata.Metadata{Type: model.MetricTypeCounter, Help: "Counter metric"}
	meta2 := metadata.Metadata{Type: model.MetricTypeGauge, Help: "Gauge metric"}
	meta3 := metadata.Metadata{Type: model.MetricTypeHistogram, Help: "Histogram metric"}

	mem.Set("metric_a", 0, meta1)
	mem.Set("metric_b", 0, meta2)
	mem.Set("metric_c", 0, meta3)

	// All entries should be accessible via IterByMetricName
	collected := make(map[string]metadata.Metadata)
	err := mem.IterByMetricName(func(name string, meta metadata.Metadata) error {
		collected[name] = meta
		return nil
	})
	require.NoError(t, err)
	require.Len(t, collected, 3)
	require.Equal(t, meta1, collected["metric_a"])
	require.Equal(t, meta2, collected["metric_b"])
	require.Equal(t, meta3, collected["metric_c"])

	// Total should reflect all entries
	require.Equal(t, uint64(3), mem.Total())

	// Iter (by hash) should NOT include zero-hash entries
	hashCount := 0
	err = mem.Iter(func(_ uint64, _ metadata.Metadata) error {
		hashCount++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 0, hashCount)

	// Get(0) should return false — zero is not a valid hash lookup
	_, found := mem.Get(0)
	require.False(t, found)
}

func TestWriteAndReadbackZeroHash(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()
	mem.Set("metric_a", 0, metadata.Metadata{Type: model.MetricTypeCounter, Help: "A"})
	mem.Set("metric_b", 0, metadata.Metadata{Type: model.MetricTypeGauge, Help: "B"})
	mem.Set("metric_c", 0, metadata.Metadata{Type: model.MetricTypeHistogram, Help: "C"})

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// All entries should be readable by metric name
	collected := make(map[string]metadata.Metadata)
	err = reader.IterByMetricName(func(name string, meta metadata.Metadata) error {
		collected[name] = meta
		return nil
	})
	require.NoError(t, err)
	require.Len(t, collected, 3)
	require.Equal(t, uint64(3), reader.Total())
}

func TestLargeMetadata(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Create many entries
	const numEntries = 10000
	for i := range uint64(numEntries) {
		mem.Set(fmt.Sprintf("metric_%d", i), i, metadata.Metadata{
			Type: model.MetricTypeCounter,
			Unit: "bytes",
			Help: "Test metric with some help text",
		})
	}

	require.Equal(t, uint64(numEntries), mem.Total())

	// Write and read back
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(numEntries), reader.Total())

	// Spot check a few entries by metric name
	for _, i := range []uint64{0, 100, 5000, numEntries - 1} {
		meta, found := reader.GetByMetricName(fmt.Sprintf("metric_%d", i))
		require.True(t, found, "entry metric_%d not found", i)
		require.Equal(t, model.MetricTypeCounter, meta.Type)
	}
}
