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
