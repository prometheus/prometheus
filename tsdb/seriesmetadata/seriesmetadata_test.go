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

// Tests for resource attributes functionality

func TestResourceAttributesBasicOperations(t *testing.T) {
	mem := NewMemResourceAttributes()

	attrs := map[string]string{
		"service.name":        "my-service",
		"service.namespace":   "production",
		"service.instance.id": "instance-123",
		"deployment.env":      "prod",
		"host.name":           "host-1",
	}

	ra := NewResourceAttributes(attrs, 1000, 2000)

	// Verify identifying attributes were extracted
	require.Equal(t, "my-service", ra.ServiceName)
	require.Equal(t, "production", ra.ServiceNamespace)
	require.Equal(t, "instance-123", ra.ServiceInstanceID)
	require.Equal(t, int64(1000), ra.MinTime)
	require.Equal(t, int64(2000), ra.MaxTime)

	// Store and retrieve
	mem.SetResourceAttributes(123, ra)

	got, found := mem.GetResourceAttributes(123)
	require.True(t, found)
	require.Equal(t, ra.ServiceName, got.ServiceName)
	require.Equal(t, ra.Attributes["deployment.env"], got.Attributes["deployment.env"])

	// Test TotalResourceAttributes
	require.Equal(t, uint64(1), mem.TotalResourceAttributes())

	// Test delete
	mem.DeleteResourceAttributes(123)
	_, found = mem.GetResourceAttributes(123)
	require.False(t, found)
}

func TestResourceAttributesIdentifying(t *testing.T) {
	attrs := map[string]string{
		"service.name":        "my-service",
		"service.namespace":   "production",
		"service.instance.id": "instance-123",
		"deployment.env":      "prod",
	}

	ra := NewResourceAttributes(attrs, 1000, 2000)

	// Test IdentifyingAttributes
	identifying := ra.IdentifyingAttributes()
	require.Len(t, identifying, 3)
	require.Equal(t, "my-service", identifying["service.name"])
	require.Equal(t, "production", identifying["service.namespace"])
	require.Equal(t, "instance-123", identifying["service.instance.id"])

	// Test NonIdentifyingAttributes
	nonIdentifying := ra.NonIdentifyingAttributes()
	require.Len(t, nonIdentifying, 1)
	require.Equal(t, "prod", nonIdentifying["deployment.env"])

	// Test IsIdentifyingAttribute
	require.True(t, IsIdentifyingAttribute("service.name"))
	require.True(t, IsIdentifyingAttribute("service.namespace"))
	require.True(t, IsIdentifyingAttribute("service.instance.id"))
	require.False(t, IsIdentifyingAttribute("deployment.env"))
	require.False(t, IsIdentifyingAttribute("host.name"))
}

func TestResourceAttributesTimeRangeUpdate(t *testing.T) {
	attrs := map[string]string{"service.name": "my-service"}
	ra := NewResourceAttributes(attrs, 1000, 2000)

	// Update with wider range
	ra.UpdateTimeRange(500, 3000)
	require.Equal(t, int64(500), ra.MinTime)
	require.Equal(t, int64(3000), ra.MaxTime)

	// Update with narrower range - should NOT change
	ra.UpdateTimeRange(700, 2500)
	require.Equal(t, int64(500), ra.MinTime)
	require.Equal(t, int64(3000), ra.MaxTime)
}

func TestResourceAttributesVersioningOnSet(t *testing.T) {
	mem := NewMemResourceAttributes()

	// First set
	attrs1 := map[string]string{
		"service.name":   "my-service",
		"deployment.env": "prod",
	}
	ra1 := NewResourceAttributes(attrs1, 1000, 2000)
	mem.SetResourceAttributes(123, ra1)

	// Second set for same hash with different attributes - should create new version
	attrs2 := map[string]string{
		"service.name": "my-service",
		"host.name":    "host-1",
	}
	ra2 := NewResourceAttributes(attrs2, 3000, 4000)
	mem.SetResourceAttributes(123, ra2)

	// GetResourceAttributes returns the current (latest) version
	got, found := mem.GetResourceAttributes(123)
	require.True(t, found)
	require.Equal(t, "my-service", got.Attributes["service.name"])
	require.Equal(t, "host-1", got.Attributes["host.name"])
	// Latest version doesn't have deployment.env
	require.Empty(t, got.Attributes["deployment.env"])

	// GetVersionedResourceAttributes returns all versions
	vattrs, found := mem.GetVersionedResourceAttributes(123)
	require.True(t, found)
	require.Len(t, vattrs.Versions, 2)

	// First version (oldest)
	require.Equal(t, "my-service", vattrs.Versions[0].Attributes["service.name"])
	require.Equal(t, "prod", vattrs.Versions[0].Attributes["deployment.env"])
	require.Equal(t, int64(1000), vattrs.Versions[0].MinTime)
	require.Equal(t, int64(2000), vattrs.Versions[0].MaxTime)

	// Second version (current/latest)
	require.Equal(t, "my-service", vattrs.Versions[1].Attributes["service.name"])
	require.Equal(t, "host-1", vattrs.Versions[1].Attributes["host.name"])
	require.Equal(t, int64(3000), vattrs.Versions[1].MinTime)
	require.Equal(t, int64(4000), vattrs.Versions[1].MaxTime)
}

func TestResourceAttributesSameAttributesExtendTimeRange(t *testing.T) {
	mem := NewMemResourceAttributes()

	// First set
	attrs := map[string]string{
		"service.name": "my-service",
	}
	ra1 := NewResourceAttributes(attrs, 1000, 2000)
	mem.SetResourceAttributes(123, ra1)

	// Second set with same attributes - should extend time range, not create new version
	ra2 := NewResourceAttributes(attrs, 3000, 4000)
	mem.SetResourceAttributes(123, ra2)

	// Should still have only one version
	vattrs, found := mem.GetVersionedResourceAttributes(123)
	require.True(t, found)
	require.Len(t, vattrs.Versions, 1)

	// Time range should be extended
	require.Equal(t, int64(1000), vattrs.Versions[0].MinTime)
	require.Equal(t, int64(4000), vattrs.Versions[0].MaxTime)
}

func TestResourceAttributesIter(t *testing.T) {
	mem := NewMemResourceAttributes()

	// Add multiple entries
	for i := uint64(1); i <= 5; i++ {
		attrs := map[string]string{
			"service.name": fmt.Sprintf("service-%d", i),
		}
		mem.SetResourceAttributes(i, NewResourceAttributes(attrs, int64(i*1000), int64(i*2000)))
	}

	// Iterate and collect
	collected := make(map[uint64]*ResourceAttributes)
	err := mem.IterResourceAttributes(func(labelsHash uint64, attrs *ResourceAttributes) error {
		collected[labelsHash] = attrs
		return nil
	})
	require.NoError(t, err)
	require.Len(t, collected, 5)

	// Verify content
	for i := uint64(1); i <= 5; i++ {
		ra, ok := collected[i]
		require.True(t, ok)
		require.Equal(t, fmt.Sprintf("service-%d", i), ra.ServiceName)
	}
}

func TestWriteAndReadbackResourceAttributes(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add metric metadata
	mem.Set("http_requests_total", 1, metadata.Metadata{
		Type: model.MetricTypeCounter,
		Help: "Total HTTP requests",
	})

	// Add resource attributes for two different series
	attrs1 := map[string]string{
		"service.name":        "frontend",
		"service.namespace":   "production",
		"service.instance.id": "frontend-1",
		"deployment.env":      "prod",
	}
	mem.SetResourceAttributes(100, NewResourceAttributes(attrs1, 1000, 2000))

	attrs2 := map[string]string{
		"service.name":        "backend",
		"service.namespace":   "production",
		"service.instance.id": "backend-1",
		"host.region":         "us-west-2",
	}
	mem.SetResourceAttributes(200, NewResourceAttributes(attrs2, 1500, 2500))

	// Write to file
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size)

	// Read back
	reader, readSize, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	defer reader.Close()
	require.Equal(t, size, readSize)

	// Verify metric metadata
	meta, found := reader.GetByMetricName("http_requests_total")
	require.True(t, found)
	require.Equal(t, model.MetricTypeCounter, meta.Type)

	// Verify resource attributes for series 100
	ra1, found := reader.GetResourceAttributes(100)
	require.True(t, found)
	require.Equal(t, "frontend", ra1.ServiceName)
	require.Equal(t, "production", ra1.ServiceNamespace)
	require.Equal(t, "frontend-1", ra1.ServiceInstanceID)
	require.Equal(t, "prod", ra1.Attributes["deployment.env"])
	require.Equal(t, int64(1000), ra1.MinTime)
	require.Equal(t, int64(2000), ra1.MaxTime)

	// Verify resource attributes for series 200
	ra2, found := reader.GetResourceAttributes(200)
	require.True(t, found)
	require.Equal(t, "backend", ra2.ServiceName)
	require.Equal(t, "us-west-2", ra2.Attributes["host.region"])
	require.Equal(t, int64(1500), ra2.MinTime)
	require.Equal(t, int64(2500), ra2.MaxTime)

	// Verify totals
	require.Equal(t, uint64(1), reader.Total())
	require.Equal(t, uint64(2), reader.TotalResourceAttributes())
}

func TestResourceAttributesIterInReader(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add multiple resource attributes
	for i := uint64(1); i <= 3; i++ {
		attrs := map[string]string{
			"service.name": fmt.Sprintf("service-%d", i),
			"host.name":    fmt.Sprintf("host-%d", i),
		}
		mem.SetResourceAttributes(i*100, NewResourceAttributes(attrs, int64(i*1000), int64(i*2000)))
	}

	// Write and read back
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// Iterate and verify
	collected := make(map[uint64]*ResourceAttributes)
	err = reader.IterResourceAttributes(func(labelsHash uint64, attrs *ResourceAttributes) error {
		collected[labelsHash] = attrs
		return nil
	})
	require.NoError(t, err)
	require.Len(t, collected, 3)

	// Verify content
	for i := uint64(1); i <= 3; i++ {
		ra, ok := collected[i*100]
		require.True(t, ok, "missing resource attributes for hash %d", i*100)
		require.Equal(t, fmt.Sprintf("service-%d", i), ra.ServiceName)
		require.Equal(t, fmt.Sprintf("host-%d", i), ra.Attributes["host.name"])
	}
}

func TestMixedMetadataAndResourceAttributes(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add various metric metadata
	mem.Set("requests_total", 1, metadata.Metadata{Type: model.MetricTypeCounter, Help: "Total requests"})
	mem.Set("temperature_celsius", 2, metadata.Metadata{Type: model.MetricTypeGauge, Unit: "celsius"})
	mem.Set("latency_seconds", 3, metadata.Metadata{Type: model.MetricTypeHistogram, Unit: "seconds"})

	// Add resource attributes for multiple series
	mem.SetResourceAttributes(100, NewResourceAttributes(map[string]string{
		"service.name":        "api-gateway",
		"service.instance.id": "gw-1",
	}, 1000, 5000))

	mem.SetResourceAttributes(200, NewResourceAttributes(map[string]string{
		"service.name":        "database",
		"service.instance.id": "db-1",
		"db.system":           "postgresql",
	}, 2000, 6000))

	// Write and read back
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	// Verify all metric metadata
	require.Equal(t, uint64(3), reader.Total())
	meta, found := reader.GetByMetricName("requests_total")
	require.True(t, found)
	require.Equal(t, model.MetricTypeCounter, meta.Type)

	meta, found = reader.GetByMetricName("temperature_celsius")
	require.True(t, found)
	require.Equal(t, "celsius", meta.Unit)

	// Verify all resource attributes
	require.Equal(t, uint64(2), reader.TotalResourceAttributes())

	ra, found := reader.GetResourceAttributes(100)
	require.True(t, found)
	require.Equal(t, "api-gateway", ra.ServiceName)

	ra, found = reader.GetResourceAttributes(200)
	require.True(t, found)
	require.Equal(t, "database", ra.ServiceName)
	require.Equal(t, "postgresql", ra.Attributes["db.system"])
}
