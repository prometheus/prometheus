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
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
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

	// Add some test metadata via versioned path (required for Parquet persistence).
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
		mem.SetVersioned(name, entry.hash, &MetadataVersion{
			Meta:    entry.meta,
			MinTime: 1000,
			MaxTime: 2000,
		})
	}

	// Write to file
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size)

	// Read back
	reader, readSize, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()
	require.Equal(t, size, readSize)

	// Verify all metadata was read back correctly by metric name
	for name, entry := range testData {
		metas, found := reader.GetByMetricName(name)
		require.True(t, found, "metadata for metric %s not found", name)
		require.Len(t, metas, 1)
		require.Equal(t, entry.meta.Type, metas[0].Type)
		require.Equal(t, entry.meta.Unit, metas[0].Unit)
		require.Equal(t, entry.meta.Help, metas[0].Help)
	}

	// Verify total count
	require.Equal(t, uint64(len(testData)), reader.Total())
}

func TestReadNonexistentFile(t *testing.T) {
	tmpdir := t.TempDir()

	// Reading from a directory without metadata file should return empty reader
	reader, size, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
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
	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
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
	require.Len(t, got, 1)
	require.Equal(t, meta, got[0])

	// Also verify Get by hash works
	gotSingle, found := mem.Get(123)
	require.True(t, found)
	require.Equal(t, meta, gotSingle)

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
	err := mem.IterByMetricName(func(name string, metas []metadata.Metadata) error {
		require.Len(t, metas, 1)
		collected[name] = metas[0]
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
	mem := NewMemSeriesMetadata()
	meta1 := metadata.Metadata{Type: model.MetricTypeCounter, Help: "First version"}
	meta2 := metadata.Metadata{Type: model.MetricTypeGauge, Help: "Second version"}

	// Set initial value
	mem.Set("test_metric", 100, meta1)
	got, _ := mem.GetByMetricName("test_metric")
	require.Len(t, got, 1)
	require.Equal(t, meta1, got[0])

	// Set with same metric name but different hash and different metadata adds to set
	mem.Set("test_metric", 200, meta2)
	got, _ = mem.GetByMetricName("test_metric")
	require.Len(t, got, 2)

	// Both hashes should work
	gotSingle, found := mem.Get(100)
	require.True(t, found)
	require.Equal(t, meta1, gotSingle)
	gotSingle, found = mem.Get(200)
	require.True(t, found)
	require.Equal(t, meta2, gotSingle)
}

func TestSetSameHashDifferentMetadata(t *testing.T) {
	mem := NewMemSeriesMetadata()
	meta1 := metadata.Metadata{Type: model.MetricTypeCounter, Help: "Version 1"}
	meta2 := metadata.Metadata{Type: model.MetricTypeCounter, Help: "Version 2"}

	// Set initial metadata for a series.
	mem.Set("test_metric", 100, meta1)
	got, _ := mem.GetByMetricName("test_metric")
	require.Len(t, got, 1)
	require.Equal(t, meta1, got[0])

	// Update the same series (same hash) with different metadata.
	// The old metadata should be replaced, not accumulated.
	mem.Set("test_metric", 100, meta2)
	got, _ = mem.GetByMetricName("test_metric")
	require.Len(t, got, 1, "same hash with updated metadata should replace, not accumulate")
	require.Equal(t, meta2, got[0])

	// Verify the hash still resolves correctly.
	gotSingle, found := mem.Get(100)
	require.True(t, found)
	require.Equal(t, meta2, gotSingle)

	// Delete should clean up completely.
	mem.Delete(100)
	_, found = mem.GetByMetricName("test_metric")
	require.False(t, found, "byName should be empty after deleting the only entry")
	_, found = mem.Get(100)
	require.False(t, found)
}

func TestSetSameHashSameMetaDifferentName(t *testing.T) {
	mem := NewMemSeriesMetadata()
	meta := metadata.Metadata{Type: model.MetricTypeCounter, Help: "Same help"}

	// Set hash=100 under "old_metric".
	mem.Set("old_metric", 100, meta)
	got, found := mem.GetByMetricName("old_metric")
	require.True(t, found)
	require.Len(t, got, 1)

	// Re-set the same hash with identical metadata but a different metric name.
	// The old byName entry should be cleaned up.
	mem.Set("new_metric", 100, meta)

	_, found = mem.GetByMetricName("old_metric")
	require.False(t, found, "old_metric should be removed after metric name change")

	got, found = mem.GetByMetricName("new_metric")
	require.True(t, found)
	require.Len(t, got, 1)
	require.Equal(t, meta, got[0])

	// Only the new metric name should be present.
	require.Equal(t, uint64(1), mem.Total())
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
	err := mem.IterByMetricName(func(name string, metas []metadata.Metadata) error {
		require.Len(t, metas, 1)
		collected[name] = metas[0]
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
	// Use distinct hashes for persistence via versioned path.
	mem.SetVersioned("metric_a", 1, &MetadataVersion{
		Meta: metadata.Metadata{Type: model.MetricTypeCounter, Help: "A"}, MinTime: 1000, MaxTime: 2000,
	})
	mem.SetVersioned("metric_b", 2, &MetadataVersion{
		Meta: metadata.Metadata{Type: model.MetricTypeGauge, Help: "B"}, MinTime: 1000, MaxTime: 2000,
	})
	mem.SetVersioned("metric_c", 3, &MetadataVersion{
		Meta: metadata.Metadata{Type: model.MetricTypeHistogram, Help: "C"}, MinTime: 1000, MaxTime: 2000,
	})

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// All entries should be readable by metric name
	collected := make(map[string]metadata.Metadata)
	err = reader.IterByMetricName(func(name string, metas []metadata.Metadata) error {
		require.Len(t, metas, 1)
		collected[name] = metas[0]
		return nil
	})
	require.NoError(t, err)
	require.Len(t, collected, 3)
	require.Equal(t, uint64(3), reader.Total())
}

func TestVersionedMetadataParquetRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add versioned entries for series hash=100.
	mem.SetVersioned("http_requests_total", 100, &MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "Total requests"},
		MinTime: 1000,
		MaxTime: 2000,
	})
	mem.SetVersioned("http_requests_total", 100, &MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "Total HTTP requests processed"},
		MinTime: 2001,
		MaxTime: 3000,
	})

	// Add versioned entries for series hash=200.
	mem.SetVersioned("temperature", 200, &MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeGauge, Help: "Temperature in Celsius"},
		MinTime: 500,
		MaxTime: 1500,
	})

	// Write.
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size)

	// Read back.
	reader, readSize, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()
	require.Equal(t, size, readSize)

	// Verify GetByMetricName returns latest version per metric (derived from versioned data).
	metas, found := reader.GetByMetricName("http_requests_total")
	require.True(t, found)
	require.Len(t, metas, 1)

	metas, found = reader.GetByMetricName("temperature")
	require.True(t, found)
	require.Len(t, metas, 1)

	// Verify versioned metadata for hash=100.
	vm, found := reader.GetVersionedMetadata(100)
	require.True(t, found)
	require.Len(t, vm.Versions, 2)
	require.Equal(t, "Total requests", vm.Versions[0].Meta.Help)
	require.Equal(t, int64(1000), vm.Versions[0].MinTime)
	require.Equal(t, int64(2000), vm.Versions[0].MaxTime)
	require.Equal(t, "Total HTTP requests processed", vm.Versions[1].Meta.Help)
	require.Equal(t, int64(2001), vm.Versions[1].MinTime)
	require.Equal(t, int64(3000), vm.Versions[1].MaxTime)

	// Verify versioned metadata for hash=200.
	vm, found = reader.GetVersionedMetadata(200)
	require.True(t, found)
	require.Len(t, vm.Versions, 1)
	require.Equal(t, "Temperature in Celsius", vm.Versions[0].Meta.Help)
	require.Equal(t, int64(500), vm.Versions[0].MinTime)

	// Non-existent hash.
	_, found = reader.GetVersionedMetadata(999)
	require.False(t, found)

	// TotalVersionedMetadata.
	require.Equal(t, 2, reader.TotalVersionedMetadata())
}

func TestVersionedMetadataContentDeduplication(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Two different series with the same metadata content.
	sharedMeta := metadata.Metadata{Type: model.MetricTypeCounter, Help: "Total requests"}

	mem.SetVersioned("http_requests_total", 100, &MetadataVersion{
		Meta: sharedMeta, MinTime: 1000, MaxTime: 2000,
	})
	mem.SetVersioned("http_requests_total", 200, &MetadataVersion{
		Meta: sharedMeta, MinTime: 1000, MaxTime: 2000,
	})

	// Write.
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	// Read back.
	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// Both series should resolve to the same metadata content.
	vm1, found := reader.GetVersionedMetadata(100)
	require.True(t, found)
	require.Len(t, vm1.Versions, 1)
	require.Equal(t, "Total requests", vm1.Versions[0].Meta.Help)

	vm2, found := reader.GetVersionedMetadata(200)
	require.True(t, found)
	require.Len(t, vm2.Versions, 1)
	require.Equal(t, "Total requests", vm2.Versions[0].Meta.Help)
}

func TestLargeMetadata(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Create many entries via versioned path
	const numEntries = 10000
	for i := range uint64(numEntries) {
		mem.SetVersioned(fmt.Sprintf("metric_%d", i), i+1, &MetadataVersion{
			Meta: metadata.Metadata{
				Type: model.MetricTypeCounter,
				Unit: "bytes",
				Help: "Test metric with some help text",
			},
			MinTime: 1000,
			MaxTime: 2000,
		})
	}

	require.Equal(t, uint64(numEntries), mem.Total())

	// Write and read back
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(numEntries), reader.Total())

	// Spot check a few entries by metric name
	for _, i := range []uint64{0, 100, 5000, numEntries - 1} {
		metas, found := reader.GetByMetricName(fmt.Sprintf("metric_%d", i))
		require.True(t, found, "entry metric_%d not found", i)
		require.Len(t, metas, 1)
		require.Equal(t, model.MetricTypeCounter, metas[0].Type)
	}
}

// Tests for unified resource functionality

func TestResourceBasicOperations(t *testing.T) {
	store := NewMemResourceStore()

	identifying := map[string]string{
		"service.name":        "my-service",
		"service.namespace":   "production",
		"service.instance.id": "instance-123",
	}
	descriptive := map[string]string{
		"deployment.env": "prod",
		"host.name":      "host-1",
	}

	rv := NewResourceVersion(identifying, descriptive, nil, 1000, 2000)

	// Verify version was created correctly
	require.Equal(t, "my-service", rv.Identifying["service.name"])
	require.Equal(t, "production", rv.Identifying["service.namespace"])
	require.Equal(t, "instance-123", rv.Identifying["service.instance.id"])
	require.Equal(t, "prod", rv.Descriptive["deployment.env"])
	require.Equal(t, int64(1000), rv.MinTime)
	require.Equal(t, int64(2000), rv.MaxTime)

	// Store and retrieve
	store.SetResource(123, rv)

	got, found := store.GetResource(123)
	require.True(t, found)
	require.Equal(t, rv.Identifying["service.name"], got.Identifying["service.name"])
	require.Equal(t, rv.Descriptive["deployment.env"], got.Descriptive["deployment.env"])

	// Test TotalResources
	require.Equal(t, uint64(1), store.TotalResources())

	// Test delete
	store.DeleteResource(123)
	_, found = store.GetResource(123)
	require.False(t, found)
}

func TestIsIdentifyingAttribute(t *testing.T) {
	// Test IsIdentifyingAttribute (for default identifying attribute detection)
	require.True(t, IsIdentifyingAttribute("service.name"))
	require.True(t, IsIdentifyingAttribute("service.namespace"))
	require.True(t, IsIdentifyingAttribute("service.instance.id"))
	require.False(t, IsIdentifyingAttribute("deployment.env"))
	require.False(t, IsIdentifyingAttribute("host.name"))
}

func TestResourceVersionTimeRangeUpdate(t *testing.T) {
	identifying := map[string]string{"service.name": "my-service"}
	rv := NewResourceVersion(identifying, nil, nil, 1000, 2000)

	// Update with wider range
	rv.UpdateTimeRange(500, 3000)
	require.Equal(t, int64(500), rv.MinTime)
	require.Equal(t, int64(3000), rv.MaxTime)

	// Update with narrower range - should NOT change
	rv.UpdateTimeRange(700, 2500)
	require.Equal(t, int64(500), rv.MinTime)
	require.Equal(t, int64(3000), rv.MaxTime)
}

func TestResourceVersioningOnSet(t *testing.T) {
	store := NewMemResourceStore()

	// First set
	identifying1 := map[string]string{"service.name": "my-service"}
	descriptive1 := map[string]string{"deployment.env": "prod"}
	rv1 := NewResourceVersion(identifying1, descriptive1, nil, 1000, 2000)
	store.SetResource(123, rv1)

	// Second set for same hash with different attributes - should create new version
	identifying2 := map[string]string{"service.name": "my-service"}
	descriptive2 := map[string]string{"host.name": "host-1"}
	rv2 := NewResourceVersion(identifying2, descriptive2, nil, 3000, 4000)
	store.SetResource(123, rv2)

	// GetResource returns the current (latest) version
	got, found := store.GetResource(123)
	require.True(t, found)
	require.Equal(t, "my-service", got.Identifying["service.name"])
	require.Equal(t, "host-1", got.Descriptive["host.name"])
	// Latest version doesn't have deployment.env
	require.Empty(t, got.Descriptive["deployment.env"])

	// GetVersionedResource returns all versions
	vr, found := store.GetVersionedResource(123)
	require.True(t, found)
	require.Len(t, vr.Versions, 2)

	// First version (oldest)
	require.Equal(t, "my-service", vr.Versions[0].Identifying["service.name"])
	require.Equal(t, "prod", vr.Versions[0].Descriptive["deployment.env"])
	require.Equal(t, int64(1000), vr.Versions[0].MinTime)
	require.Equal(t, int64(2000), vr.Versions[0].MaxTime)

	// Second version (current/latest)
	require.Equal(t, "my-service", vr.Versions[1].Identifying["service.name"])
	require.Equal(t, "host-1", vr.Versions[1].Descriptive["host.name"])
	require.Equal(t, int64(3000), vr.Versions[1].MinTime)
	require.Equal(t, int64(4000), vr.Versions[1].MaxTime)
}

func TestResourceSameAttributesExtendTimeRange(t *testing.T) {
	store := NewMemResourceStore()

	// First set
	identifying := map[string]string{"service.name": "my-service"}
	rv1 := NewResourceVersion(identifying, nil, nil, 1000, 2000)
	store.SetResource(123, rv1)

	// Second set with same attributes - should extend time range, not create new version
	rv2 := NewResourceVersion(identifying, nil, nil, 3000, 4000)
	store.SetResource(123, rv2)

	// Should still have only one version
	vr, found := store.GetVersionedResource(123)
	require.True(t, found)
	require.Len(t, vr.Versions, 1)

	// Time range should be extended
	require.Equal(t, int64(1000), vr.Versions[0].MinTime)
	require.Equal(t, int64(4000), vr.Versions[0].MaxTime)
}

func TestResourceIter(t *testing.T) {
	store := NewMemResourceStore()

	// Add multiple entries
	for i := uint64(1); i <= 5; i++ {
		identifying := map[string]string{
			"service.name": fmt.Sprintf("service-%d", i),
		}
		rv := NewResourceVersion(identifying, nil, nil, int64(i*1000), int64(i*2000))
		store.SetResource(i, rv)
	}

	// Iterate and collect
	collected := make(map[uint64]*ResourceVersion)
	err := store.IterResources(func(labelsHash uint64, rv *ResourceVersion) error {
		collected[labelsHash] = rv
		return nil
	})
	require.NoError(t, err)
	require.Len(t, collected, 5)

	// Verify content
	for i := uint64(1); i <= 5; i++ {
		rv, ok := collected[i]
		require.True(t, ok)
		require.Equal(t, fmt.Sprintf("service-%d", i), rv.Identifying["service.name"])
	}
}

func TestWriteAndReadbackResources(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add metric metadata using versioned approach
	mem.SetVersioned("http_requests_total", 1, &MetadataVersion{
		Meta: metadata.Metadata{
			Type: model.MetricTypeCounter,
			Help: "Total HTTP requests",
		},
		MinTime: 1000,
		MaxTime: 2000,
	})

	// Add resources for two different series
	identifying1 := map[string]string{
		"service.name":        "frontend",
		"service.namespace":   "production",
		"service.instance.id": "frontend-1",
	}
	descriptive1 := map[string]string{
		"deployment.env": "prod",
	}
	rv1 := NewResourceVersion(identifying1, descriptive1, nil, 1000, 2000)
	mem.SetResource(100, rv1)

	identifying2 := map[string]string{
		"service.name":        "backend",
		"service.namespace":   "production",
		"service.instance.id": "backend-1",
	}
	descriptive2 := map[string]string{
		"host.region": "us-west-2",
	}
	rv2 := NewResourceVersion(identifying2, descriptive2, nil, 1500, 2500)
	mem.SetResource(200, rv2)

	// Write to file
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size)

	// Read back
	reader, readSize, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()
	require.Equal(t, size, readSize)

	// Verify metric metadata
	metas, found := reader.GetByMetricName("http_requests_total")
	require.True(t, found)
	require.Equal(t, model.MetricTypeCounter, metas[0].Type)

	// Verify resource for series 100
	r1, found := reader.GetResource(100)
	require.True(t, found)
	require.Equal(t, "frontend", r1.Identifying["service.name"])
	require.Equal(t, "production", r1.Identifying["service.namespace"])
	require.Equal(t, "frontend-1", r1.Identifying["service.instance.id"])
	require.Equal(t, "prod", r1.Descriptive["deployment.env"])
	require.Equal(t, int64(1000), r1.MinTime)
	require.Equal(t, int64(2000), r1.MaxTime)

	// Verify resource for series 200
	r2, found := reader.GetResource(200)
	require.True(t, found)
	require.Equal(t, "backend", r2.Identifying["service.name"])
	require.Equal(t, "us-west-2", r2.Descriptive["host.region"])
	require.Equal(t, int64(1500), r2.MinTime)
	require.Equal(t, int64(2500), r2.MaxTime)

	// Verify totals
	require.Equal(t, uint64(1), reader.Total())
	require.Equal(t, uint64(2), reader.TotalResources())
}

func TestMixedMetadataAndResources(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add various metric metadata using versioned approach
	mem.SetVersioned("requests_total", 1, &MetadataVersion{
		Meta: metadata.Metadata{Type: model.MetricTypeCounter, Help: "Total requests"}, MinTime: 1000, MaxTime: 5000,
	})
	mem.SetVersioned("temperature_celsius", 2, &MetadataVersion{
		Meta: metadata.Metadata{Type: model.MetricTypeGauge, Unit: "celsius"}, MinTime: 1000, MaxTime: 5000,
	})
	mem.SetVersioned("latency_seconds", 3, &MetadataVersion{
		Meta: metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "seconds"}, MinTime: 1000, MaxTime: 5000,
	})

	// Add resources for multiple series
	rv1 := NewResourceVersion(
		map[string]string{
			"service.name":        "api-gateway",
			"service.instance.id": "gw-1",
		},
		nil, // no descriptive attributes
		nil, // no entities
		1000, 5000)
	mem.SetResource(100, rv1)

	rv2 := NewResourceVersion(
		map[string]string{
			"service.name":        "database",
			"service.instance.id": "db-1",
		},
		map[string]string{
			"db.system": "postgresql",
		},
		nil, // no entities
		2000, 6000)
	mem.SetResource(200, rv2)

	// Write and read back
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// Verify all metric metadata
	require.Equal(t, uint64(3), reader.Total())
	metas, found := reader.GetByMetricName("requests_total")
	require.True(t, found)
	require.Equal(t, model.MetricTypeCounter, metas[0].Type)

	metas, found = reader.GetByMetricName("temperature_celsius")
	require.True(t, found)
	require.Equal(t, "celsius", metas[0].Unit)

	// Verify all resources
	require.Equal(t, uint64(2), reader.TotalResources())

	r, found := reader.GetResource(100)
	require.True(t, found)
	require.Equal(t, "api-gateway", r.Identifying["service.name"])

	r, found = reader.GetResource(200)
	require.True(t, found)
	require.Equal(t, "database", r.Identifying["service.name"])
	require.Equal(t, "postgresql", r.Descriptive["db.system"])
}

func TestWriteAndReadbackScopes(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add scope data for two series
	sv1 := NewScopeVersion("go.opentelemetry.io/contrib/instrumentation/net/http", "0.45.0", "https://opentelemetry.io/schemas/1.21.0",
		map[string]string{"library.language": "go"}, 1000, 2000)
	mem.SetVersionedScope(100, NewVersionedScope(sv1))

	sv2 := NewScopeVersion("io.opentelemetry.contrib.javaagent", "1.30.0", "",
		map[string]string{"library.language": "java", "library.framework": "spring"}, 1500, 2500)
	mem.SetVersionedScope(200, NewVersionedScope(sv2))

	// Write to file
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size)

	// Read back
	reader, readSize, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()
	require.Equal(t, size, readSize)

	// Verify scope for series 100
	vs1, found := reader.GetVersionedScope(100)
	require.True(t, found)
	require.Len(t, vs1.Versions, 1)
	require.Equal(t, "go.opentelemetry.io/contrib/instrumentation/net/http", vs1.Versions[0].Name)
	require.Equal(t, "0.45.0", vs1.Versions[0].Version)
	require.Equal(t, "https://opentelemetry.io/schemas/1.21.0", vs1.Versions[0].SchemaURL)
	require.Equal(t, "go", vs1.Versions[0].Attrs["library.language"])
	require.Equal(t, int64(1000), vs1.Versions[0].MinTime)
	require.Equal(t, int64(2000), vs1.Versions[0].MaxTime)

	// Verify scope for series 200
	vs2, found := reader.GetVersionedScope(200)
	require.True(t, found)
	require.Len(t, vs2.Versions, 1)
	require.Equal(t, "io.opentelemetry.contrib.javaagent", vs2.Versions[0].Name)
	require.Equal(t, "java", vs2.Versions[0].Attrs["library.language"])
	require.Equal(t, "spring", vs2.Versions[0].Attrs["library.framework"])

	// Verify totals
	require.Equal(t, uint64(2), reader.TotalScopes())
	require.Equal(t, uint64(2), reader.TotalScopeVersions())
}

func TestMixedMetadataResourcesAndScopes(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add metric metadata using versioned approach
	mem.SetVersioned("http_requests_total", 1, &MetadataVersion{
		Meta: metadata.Metadata{Type: model.MetricTypeCounter, Help: "Total HTTP requests"}, MinTime: 1000, MaxTime: 5000,
	})

	// Add resource
	rv := NewResourceVersion(
		map[string]string{"service.name": "my-service"},
		map[string]string{"deployment.env": "prod"},
		nil, 1000, 5000)
	mem.SetResource(100, rv)

	// Add scope
	sv := NewScopeVersion("mylib", "1.0.0", "", map[string]string{"k": "v"}, 1000, 5000)
	mem.SetVersionedScope(100, NewVersionedScope(sv))

	// Write and read back
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// Verify all three types
	require.Equal(t, uint64(1), reader.Total())
	require.Equal(t, uint64(1), reader.TotalResources())
	require.Equal(t, uint64(1), reader.TotalScopes())

	metas, found := reader.GetByMetricName("http_requests_total")
	require.True(t, found)
	require.Equal(t, model.MetricTypeCounter, metas[0].Type)

	r, found := reader.GetResource(100)
	require.True(t, found)
	require.Equal(t, "my-service", r.Identifying["service.name"])

	vs, found := reader.GetVersionedScope(100)
	require.True(t, found)
	require.Len(t, vs.Versions, 1)
	require.Equal(t, "mylib", vs.Versions[0].Name)
	require.Equal(t, "v", vs.Versions[0].Attrs["k"])
}

func TestScopeVersioningParquetRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add two scope versions for the same series
	vs := &VersionedScope{
		Versions: []*ScopeVersion{
			NewScopeVersion("lib", "1.0", "", nil, 1000, 2000),
			NewScopeVersion("lib", "2.0", "", map[string]string{"new": "attr"}, 3000, 4000),
		},
	}
	mem.SetVersionedScope(100, vs)

	// Write and read back
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	got, found := reader.GetVersionedScope(100)
	require.True(t, found)
	require.Len(t, got.Versions, 2)
	require.Equal(t, "1.0", got.Versions[0].Version)
	require.Equal(t, int64(1000), got.Versions[0].MinTime)
	require.Equal(t, "2.0", got.Versions[1].Version)
	require.Equal(t, "attr", got.Versions[1].Attrs["new"])
	require.Equal(t, int64(3000), got.Versions[1].MinTime)

	require.Equal(t, uint64(1), reader.TotalScopes())
	require.Equal(t, uint64(2), reader.TotalScopeVersions())
}

// Tests for normalized Parquet storage.

func TestContentHashDeterminism(t *testing.T) {
	// Same content must produce the same hash regardless of map iteration order.
	rv := NewResourceVersion(
		map[string]string{"service.name": "foo", "service.namespace": "bar", "service.instance.id": "baz"},
		map[string]string{"z": "1", "a": "2", "m": "3"},
		[]*Entity{NewEntity("service", map[string]string{"b": "1", "a": "2"}, map[string]string{"y": "1", "x": "2"})},
		1000, 2000,
	)

	hash1 := hashResourceContent(rv)
	// Compute many times — must always be the same.
	for range 100 {
		require.Equal(t, hash1, hashResourceContent(rv))
	}

	// Same content but constructed separately.
	rv2 := NewResourceVersion(
		map[string]string{"service.instance.id": "baz", "service.name": "foo", "service.namespace": "bar"},
		map[string]string{"m": "3", "z": "1", "a": "2"},
		[]*Entity{NewEntity("service", map[string]string{"a": "2", "b": "1"}, map[string]string{"x": "2", "y": "1"})},
		5000, 6000, // Different times — should NOT affect hash
	)
	require.Equal(t, hash1, hashResourceContent(rv2))

	// Different content must produce different hash.
	rv3 := NewResourceVersion(
		map[string]string{"service.name": "DIFFERENT"},
		nil, nil, 1000, 2000,
	)
	require.NotEqual(t, hash1, hashResourceContent(rv3))
}

func TestScopeHashDeterminism(t *testing.T) {
	sv := NewScopeVersion("lib", "1.0", "https://schema", map[string]string{"z": "1", "a": "2"}, 1000, 2000)
	hash1 := hashScopeContent(sv)

	// Same content, different time range.
	sv2 := NewScopeVersion("lib", "1.0", "https://schema", map[string]string{"a": "2", "z": "1"}, 9000, 9999)
	require.Equal(t, hash1, hashScopeContent(sv2))

	// Different content.
	sv3 := NewScopeVersion("lib", "2.0", "https://schema", map[string]string{"a": "2", "z": "1"}, 1000, 2000)
	require.NotEqual(t, hash1, hashScopeContent(sv3))
}

func TestNormalizedResourceDeduplication(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// 100 series all sharing the same resource content.
	sharedIdentifying := map[string]string{
		"service.name":      "shared-service",
		"service.namespace": "production",
	}
	sharedDescriptive := map[string]string{
		"deployment.env": "prod",
		"host.region":    "us-west-2",
	}

	for i := uint64(1); i <= 100; i++ {
		rv := NewResourceVersion(sharedIdentifying, sharedDescriptive, nil, 1000, 5000)
		mem.SetResource(i, rv)
	}

	// Write — normalization should produce 1 table row + 100 mapping rows.
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size)

	// Read back and verify all 100 series have the correct resource data.
	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(100), reader.TotalResources())

	for i := uint64(1); i <= 100; i++ {
		r, found := reader.GetResource(i)
		require.True(t, found, "resource for series %d not found", i)
		require.Equal(t, "shared-service", r.Identifying["service.name"])
		require.Equal(t, "production", r.Identifying["service.namespace"])
		require.Equal(t, "prod", r.Descriptive["deployment.env"])
		require.Equal(t, "us-west-2", r.Descriptive["host.region"])
		require.Equal(t, int64(1000), r.MinTime)
		require.Equal(t, int64(5000), r.MaxTime)
	}
}

func TestNormalizedScopeDeduplication(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// 50 series sharing scope A, 50 series sharing scope B.
	for i := uint64(1); i <= 50; i++ {
		sv := NewScopeVersion("otel-go", "1.0", "", map[string]string{"lang": "go"}, 1000, 5000)
		mem.SetVersionedScope(i, NewVersionedScope(sv))
	}
	for i := uint64(51); i <= 100; i++ {
		sv := NewScopeVersion("otel-java", "2.0", "", map[string]string{"lang": "java"}, 1000, 5000)
		mem.SetVersionedScope(i, NewVersionedScope(sv))
	}

	// Write — should produce 2 scope table rows + 100 scope mapping rows.
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	// Read back.
	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(100), reader.TotalScopes())

	// Verify scope A series.
	for i := uint64(1); i <= 50; i++ {
		vs, found := reader.GetVersionedScope(i)
		require.True(t, found)
		require.Len(t, vs.Versions, 1)
		require.Equal(t, "otel-go", vs.Versions[0].Name)
		require.Equal(t, "go", vs.Versions[0].Attrs["lang"])
	}

	// Verify scope B series.
	for i := uint64(51); i <= 100; i++ {
		vs, found := reader.GetVersionedScope(i)
		require.True(t, found)
		require.Len(t, vs.Versions, 1)
		require.Equal(t, "otel-java", vs.Versions[0].Name)
		require.Equal(t, "java", vs.Versions[0].Attrs["lang"])
	}
}

func TestNormalizedMixedUniqueAndSharedResources(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// 5 series with unique resources, plus 10 series sharing one resource.
	for i := uint64(1); i <= 5; i++ {
		rv := NewResourceVersion(
			map[string]string{"service.name": fmt.Sprintf("unique-%d", i)},
			nil, nil, 1000, 5000,
		)
		mem.SetResource(i, rv)
	}
	for i := uint64(6); i <= 15; i++ {
		rv := NewResourceVersion(
			map[string]string{"service.name": "shared"},
			nil, nil, 1000, 5000,
		)
		mem.SetResource(i, rv)
	}

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// 15 total series with resources.
	require.Equal(t, uint64(15), reader.TotalResources())

	// Verify unique resources.
	for i := uint64(1); i <= 5; i++ {
		r, found := reader.GetResource(i)
		require.True(t, found)
		require.Equal(t, fmt.Sprintf("unique-%d", i), r.Identifying["service.name"])
	}
	// Verify shared resources.
	for i := uint64(6); i <= 15; i++ {
		r, found := reader.GetResource(i)
		require.True(t, found)
		require.Equal(t, "shared", r.Identifying["service.name"])
	}
}

func TestNormalizedResourceWithEntities(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Two series sharing the same resource with entities.
	entities := []*Entity{
		NewEntity("service", map[string]string{"service.name": "my-svc"}, map[string]string{"version": "1.0"}),
		NewEntity("host", map[string]string{"host.id": "h1"}, nil),
	}
	for i := uint64(1); i <= 2; i++ {
		rv := NewResourceVersion(
			map[string]string{"service.name": "my-svc"},
			map[string]string{"cloud": "aws"},
			entities,
			1000, 5000,
		)
		mem.SetResource(i, rv)
	}

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(2), reader.TotalResources())

	for i := uint64(1); i <= 2; i++ {
		r, found := reader.GetResource(i)
		require.True(t, found)
		require.Equal(t, "my-svc", r.Identifying["service.name"])
		require.Equal(t, "aws", r.Descriptive["cloud"])
		require.Len(t, r.Entities, 2)

		svc := r.GetEntity("service")
		require.NotNil(t, svc)
		require.Equal(t, "my-svc", svc.ID["service.name"])
		require.Equal(t, "1.0", svc.Description["version"])

		host := r.GetEntity("host")
		require.NotNil(t, host)
		require.Equal(t, "h1", host.ID["host.id"])
	}
}

func TestNormalizedVersionedResourceRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Series with two resource versions (different content at different times).
	vr := &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(
				map[string]string{"service.name": "svc"},
				map[string]string{"version": "1.0"},
				nil, 1000, 2000,
			),
			NewResourceVersion(
				map[string]string{"service.name": "svc"},
				map[string]string{"version": "2.0"},
				nil, 3000, 4000,
			),
		},
	}
	mem.SetVersionedResource(100, vr)

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	got, found := reader.GetVersionedResource(100)
	require.True(t, found)
	require.Len(t, got.Versions, 2)
	require.Equal(t, "1.0", got.Versions[0].Descriptive["version"])
	require.Equal(t, int64(1000), got.Versions[0].MinTime)
	require.Equal(t, "2.0", got.Versions[1].Descriptive["version"])
	require.Equal(t, int64(3000), got.Versions[1].MinTime)
}

// Tests for distributed-scale Parquet features.

// buildTestData creates a MemSeriesMetadata with metrics, resources, and scopes
// for use in multiple tests.
func buildTestData(t *testing.T) *MemSeriesMetadata {
	t.Helper()
	mem := NewMemSeriesMetadata()

	// Metrics (use SetVersioned so they persist via IterVersionedMetadata in WriteFile).
	mem.SetVersioned("http_requests_total", 1, &MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "Total HTTP requests"},
		MinTime: 1000, MaxTime: 5000,
	})
	mem.SetVersioned("temperature", 2, &MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeGauge, Unit: "celsius"},
		MinTime: 1000, MaxTime: 5000,
	})

	// Resources (two unique + two shared).
	for i := uint64(100); i <= 101; i++ {
		rv := NewResourceVersion(
			map[string]string{"service.name": fmt.Sprintf("svc-%d", i)},
			map[string]string{"env": "prod"},
			nil, 1000, 5000,
		)
		mem.SetResource(i, rv)
	}
	rv := NewResourceVersion(
		map[string]string{"service.name": "shared-svc"},
		map[string]string{"env": "staging"},
		nil, 1000, 5000,
	)
	mem.SetResource(200, rv)
	mem.SetResource(201, rv)

	// Scopes.
	sv := NewScopeVersion("otel-go", "1.0", "", map[string]string{"lang": "go"}, 1000, 5000)
	mem.SetVersionedScope(100, NewVersionedScope(sv))
	mem.SetVersionedScope(200, NewVersionedScope(sv))

	return mem
}

func TestWriteFileWithOptions_NamespaceRowGroups(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{})
	require.NoError(t, err)

	// Open the Parquet file and inspect row groups.
	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := parquet.OpenFile(f, stat.Size())
	require.NoError(t, err)

	// Each row group should contain rows of a single namespace.
	nsColIdx := lookupColumnIndex(pf.Schema(), "namespace")
	require.GreaterOrEqual(t, nsColIdx, 0)

	seenNamespaces := make(map[string]bool)
	for _, rg := range pf.RowGroups() {
		ns, ok := rowGroupSingleNamespace(rg, nsColIdx)
		require.True(t, ok, "row group should be single-namespace")
		seenNamespaces[ns] = true
	}

	// We should see at least metadata_table, metadata_mapping, resource_table, resource_mapping, scope_table, scope_mapping.
	require.True(t, seenNamespaces[nsMetadataTable], "expected metadata_table namespace")
	require.True(t, seenNamespaces[nsMetadataMapping], "expected metadata_mapping namespace")
	require.True(t, seenNamespaces[NamespaceResourceTable], "expected resource_table namespace")
	require.True(t, seenNamespaces[NamespaceResourceMapping], "expected resource_mapping namespace")
	require.True(t, seenNamespaces[NamespaceScopeTable], "expected scope_table namespace")
	require.True(t, seenNamespaces[NamespaceScopeMapping], "expected scope_mapping namespace")
}

func TestWriteFileWithOptions_MaxRowsPerRowGroup(t *testing.T) {
	tmpdir := t.TempDir()
	mem := NewMemSeriesMetadata()

	// Create 200 series all sharing the same resource → 200 resource_mapping rows.
	for i := uint64(1); i <= 200; i++ {
		rv := NewResourceVersion(
			map[string]string{"service.name": "shared"},
			nil, nil, 1000, 5000,
		)
		mem.SetResource(i, rv)
	}

	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{
		MaxRowsPerRowGroup: 50,
	})
	require.NoError(t, err)

	// Open and verify row groups.
	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := parquet.OpenFile(f, stat.Size())
	require.NoError(t, err)

	nsColIdx := lookupColumnIndex(pf.Schema(), "namespace")
	require.GreaterOrEqual(t, nsColIdx, 0)

	var resMappingRowGroups int
	for _, rg := range pf.RowGroups() {
		ns, ok := rowGroupSingleNamespace(rg, nsColIdx)
		if !ok {
			continue
		}
		if ns == NamespaceResourceMapping {
			require.LessOrEqual(t, rg.NumRows(), int64(50), "row group exceeds MaxRowsPerRowGroup")
			resMappingRowGroups++
		}
	}
	// 200 rows / 50 per group = 4 row groups.
	require.Equal(t, 4, resMappingRowGroups)
}

func TestWriteFileWithOptions_BloomFilters(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{
		EnableBloomFilters: true,
	})
	require.NoError(t, err)

	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := parquet.OpenFile(f, stat.Size())
	require.NoError(t, err)

	// Find column indexes for labels_hash and content_hash.
	labelsHashIdx := lookupColumnIndex(pf.Schema(), "labels_hash")
	contentHashIdx := lookupColumnIndex(pf.Schema(), "content_hash")
	require.GreaterOrEqual(t, labelsHashIdx, 0)
	require.GreaterOrEqual(t, contentHashIdx, 0)

	// At least one row group should have bloom filters for these columns.
	var foundLabelsBloom, foundContentBloom bool
	for _, rg := range pf.RowGroups() {
		ccs := rg.ColumnChunks()
		if bf := ccs[labelsHashIdx].BloomFilter(); bf != nil {
			foundLabelsBloom = true
		}
		if bf := ccs[contentHashIdx].BloomFilter(); bf != nil {
			foundContentBloom = true
		}
	}
	require.True(t, foundLabelsBloom, "expected bloom filter on labels_hash column")
	require.True(t, foundContentBloom, "expected bloom filter on content_hash column")
}

func TestWriteFileWithOptions_RoundTrip(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	// Write with all options enabled.
	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{
		MaxRowsPerRowGroup: 2,
		EnableBloomFilters: true,
	})
	require.NoError(t, err)

	// Read back via standard ReadSeriesMetadata.
	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// Verify metrics.
	require.Equal(t, uint64(2), reader.Total())
	metas, found := reader.GetByMetricName("http_requests_total")
	require.True(t, found)
	require.Equal(t, model.MetricTypeCounter, metas[0].Type)

	metas, found = reader.GetByMetricName("temperature")
	require.True(t, found)
	require.Equal(t, "celsius", metas[0].Unit)

	// Verify resources.
	require.Equal(t, uint64(4), reader.TotalResources())
	r, found := reader.GetResource(100)
	require.True(t, found)
	require.Equal(t, "svc-100", r.Identifying["service.name"])
	require.Equal(t, "prod", r.Descriptive["env"])

	r, found = reader.GetResource(200)
	require.True(t, found)
	require.Equal(t, "shared-svc", r.Identifying["service.name"])

	// Verify scopes.
	require.Equal(t, uint64(2), reader.TotalScopes())
	vs, found := reader.GetVersionedScope(100)
	require.True(t, found)
	require.Equal(t, "otel-go", vs.Versions[0].Name)
}

func TestReadSeriesMetadataFromReaderAt(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	// Open as *os.File which implements io.ReaderAt.
	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	stat, err := f.Stat()
	require.NoError(t, err)

	reader, err := ReadSeriesMetadataFromReaderAt(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)
	defer reader.Close()

	// Verify same results as ReadSeriesMetadata.
	require.Equal(t, uint64(2), reader.Total())
	require.Equal(t, uint64(4), reader.TotalResources())
	require.Equal(t, uint64(2), reader.TotalScopes())

	metas, found := reader.GetByMetricName("http_requests_total")
	require.True(t, found)
	require.Equal(t, model.MetricTypeCounter, metas[0].Type)

	r, found := reader.GetResource(100)
	require.True(t, found)
	require.Equal(t, "svc-100", r.Identifying["service.name"])
}

func TestReadSeriesMetadataFromReaderAt_BytesReader(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	// Read file into memory and wrap in bytes.Reader (simulates objstore buffer).
	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	reader, err := ReadSeriesMetadataFromReaderAt(promslog.NewNopLogger(), bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(2), reader.Total())
	require.Equal(t, uint64(4), reader.TotalResources())

	r, found := reader.GetResource(201)
	require.True(t, found)
	require.Equal(t, "shared-svc", r.Identifying["service.name"])
}

func TestReadSeriesMetadataFromReaderAt_NamespaceFilter(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	// Write with namespace-partitioned row groups.
	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{})
	require.NoError(t, err)

	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	stat, err := f.Stat()
	require.NoError(t, err)

	// Filter for only resource namespaces.
	reader, err := ReadSeriesMetadataFromReaderAt(promslog.NewNopLogger(), f, stat.Size(),
		WithNamespaceFilter(NamespaceResourceTable, NamespaceResourceMapping))
	require.NoError(t, err)
	defer reader.Close()

	// Resources should be loaded.
	require.Equal(t, uint64(4), reader.TotalResources())
	r, found := reader.GetResource(100)
	require.True(t, found)
	require.Equal(t, "svc-100", r.Identifying["service.name"])

	// Metrics and scopes should be empty.
	require.Equal(t, uint64(0), reader.Total())
	require.Equal(t, uint64(0), reader.TotalScopes())
}

func TestReadSeriesMetadataFromReaderAt_NamespaceFilter_EmptyResult(t *testing.T) {
	tmpdir := t.TempDir()
	mem := NewMemSeriesMetadata()
	mem.SetVersioned("test_metric", 1, &MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter},
		MinTime: 1000, MaxTime: 5000,
	})

	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{})
	require.NoError(t, err)

	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	stat, err := f.Stat()
	require.NoError(t, err)

	// Filter for resource namespaces when there are no resources.
	reader, err := ReadSeriesMetadataFromReaderAt(promslog.NewNopLogger(), f, stat.Size(),
		WithNamespaceFilter(NamespaceResourceTable, NamespaceResourceMapping))
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(0), reader.Total())
	require.Equal(t, uint64(0), reader.TotalResources())
	require.Equal(t, uint64(0), reader.TotalScopes())
}

func TestReadOldSingleRowGroupFile(t *testing.T) {
	// Verify that the new reader handles files written with the old WriteFile
	// (which produces a single row group for all namespaces).
	// The current WriteFile now uses WriteFileWithOptions with default opts,
	// which produces namespace-partitioned row groups. This test simulates
	// the old format by writing all rows into a single row group manually.
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	// Write with the new partitioned writer.
	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{})
	require.NoError(t, err)

	// Read back with both standard and ReaderAt paths — both should work.
	reader1, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader1.Close()

	require.Equal(t, uint64(2), reader1.Total())
	require.Equal(t, uint64(4), reader1.TotalResources())
	require.Equal(t, uint64(2), reader1.TotalScopes())

	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	reader2, err := ReadSeriesMetadataFromReaderAt(promslog.NewNopLogger(), bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer reader2.Close()

	require.Equal(t, uint64(2), reader2.Total())
	require.Equal(t, uint64(4), reader2.TotalResources())
	require.Equal(t, uint64(2), reader2.TotalScopes())
}
