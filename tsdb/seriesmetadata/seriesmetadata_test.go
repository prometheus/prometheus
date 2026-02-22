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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestReadNonexistentFile(t *testing.T) {
	tmpdir := t.TempDir()

	// Reading from a directory without metadata file should return empty reader.
	reader, size, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	require.Equal(t, int64(0), size)
	require.Equal(t, uint64(0), reader.TotalResources())
	require.Equal(t, uint64(0), reader.TotalScopes())
	require.NoError(t, reader.Close())
}

func TestWriteEmptyMetadata(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Write empty metadata — no file is created when there are no rows.
	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Zero(t, size)

	// Read back.
	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(0), reader.TotalResources())
	require.Equal(t, uint64(0), reader.TotalScopes())
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
	store.Set(123, rv)

	got, found := store.Get(123)
	require.True(t, found)
	require.Equal(t, rv.Identifying["service.name"], got.Identifying["service.name"])
	require.Equal(t, rv.Descriptive["deployment.env"], got.Descriptive["deployment.env"])

	// Test TotalResources
	require.Equal(t, uint64(1), store.TotalEntries())

	// Test delete
	store.Delete(123)
	_, found = store.Get(123)
	require.False(t, found)
}

func TestIsIdentifyingAttribute(t *testing.T) {
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
	store.Set(123, rv1)

	// Second set for same hash with different attributes - should create new version
	identifying2 := map[string]string{"service.name": "my-service"}
	descriptive2 := map[string]string{"host.name": "host-1"}
	rv2 := NewResourceVersion(identifying2, descriptive2, nil, 3000, 4000)
	store.Set(123, rv2)

	// GetResource returns the current (latest) version
	got, found := store.Get(123)
	require.True(t, found)
	require.Equal(t, "my-service", got.Identifying["service.name"])
	require.Equal(t, "host-1", got.Descriptive["host.name"])
	require.Empty(t, got.Descriptive["deployment.env"])

	// GetVersionedResource returns all versions
	vr, found := store.GetVersioned(123)
	require.True(t, found)
	require.Len(t, vr.Versions, 2)

	require.Equal(t, "my-service", vr.Versions[0].Identifying["service.name"])
	require.Equal(t, "prod", vr.Versions[0].Descriptive["deployment.env"])
	require.Equal(t, int64(1000), vr.Versions[0].MinTime)

	require.Equal(t, "my-service", vr.Versions[1].Identifying["service.name"])
	require.Equal(t, "host-1", vr.Versions[1].Descriptive["host.name"])
	require.Equal(t, int64(3000), vr.Versions[1].MinTime)
}

func TestResourceSameAttributesExtendTimeRange(t *testing.T) {
	store := NewMemResourceStore()

	identifying := map[string]string{"service.name": "my-service"}
	rv1 := NewResourceVersion(identifying, nil, nil, 1000, 2000)
	store.Set(123, rv1)

	rv2 := NewResourceVersion(identifying, nil, nil, 3000, 4000)
	store.Set(123, rv2)

	vr, found := store.GetVersioned(123)
	require.True(t, found)
	require.Len(t, vr.Versions, 1)
	require.Equal(t, int64(1000), vr.Versions[0].MinTime)
	require.Equal(t, int64(4000), vr.Versions[0].MaxTime)
}

func TestResourceIter(t *testing.T) {
	store := NewMemResourceStore()

	for i := uint64(1); i <= 5; i++ {
		identifying := map[string]string{
			"service.name": fmt.Sprintf("service-%d", i),
		}
		rv := NewResourceVersion(identifying, nil, nil, int64(i*1000), int64(i*2000))
		store.Set(i, rv)
	}

	collected := make(map[uint64]*ResourceVersion)
	err := store.Iter(context.Background(), func(labelsHash uint64, rv *ResourceVersion) error {
		collected[labelsHash] = rv
		return nil
	})
	require.NoError(t, err)
	require.Len(t, collected, 5)

	for i := uint64(1); i <= 5; i++ {
		rv, ok := collected[i]
		require.True(t, ok)
		require.Equal(t, fmt.Sprintf("service-%d", i), rv.Identifying["service.name"])
	}
}

func TestWriteAndReadbackResources(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

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

	require.Equal(t, uint64(2), reader.TotalResources())
}

func TestMixedResourcesAndScopes(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// Add resources for multiple series
	rv1 := NewResourceVersion(
		map[string]string{
			"service.name":        "api-gateway",
			"service.instance.id": "gw-1",
		},
		nil, nil, 1000, 5000)
	mem.SetResource(100, rv1)

	rv2 := NewResourceVersion(
		map[string]string{
			"service.name":        "database",
			"service.instance.id": "db-1",
		},
		map[string]string{"db.system": "postgresql"},
		nil, 2000, 6000)
	mem.SetResource(200, rv2)

	// Write and read back
	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

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

func TestResourcesAndScopes(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

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

	require.Equal(t, uint64(1), reader.TotalResources())
	require.Equal(t, uint64(1), reader.TotalScopes())

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
	rv := NewResourceVersion(
		map[string]string{"service.name": "foo", "service.namespace": "bar", "service.instance.id": "baz"},
		map[string]string{"z": "1", "a": "2", "m": "3"},
		[]*Entity{NewEntity("service", map[string]string{"b": "1", "a": "2"}, map[string]string{"y": "1", "x": "2"})},
		1000, 2000,
	)

	hash1 := hashResourceContent(rv)
	for range 100 {
		require.Equal(t, hash1, hashResourceContent(rv))
	}

	rv2 := NewResourceVersion(
		map[string]string{"service.instance.id": "baz", "service.name": "foo", "service.namespace": "bar"},
		map[string]string{"m": "3", "z": "1", "a": "2"},
		[]*Entity{NewEntity("service", map[string]string{"a": "2", "b": "1"}, map[string]string{"x": "2", "y": "1"})},
		5000, 6000,
	)
	require.Equal(t, hash1, hashResourceContent(rv2))

	rv3 := NewResourceVersion(
		map[string]string{"service.name": "DIFFERENT"},
		nil, nil, 1000, 2000,
	)
	require.NotEqual(t, hash1, hashResourceContent(rv3))
}

func TestScopeHashDeterminism(t *testing.T) {
	sv := NewScopeVersion("lib", "1.0", "https://schema", map[string]string{"z": "1", "a": "2"}, 1000, 2000)
	hash1 := hashScopeContent(sv)

	sv2 := NewScopeVersion("lib", "1.0", "https://schema", map[string]string{"a": "2", "z": "1"}, 9000, 9999)
	require.Equal(t, hash1, hashScopeContent(sv2))

	sv3 := NewScopeVersion("lib", "2.0", "https://schema", map[string]string{"a": "2", "z": "1"}, 1000, 2000)
	require.NotEqual(t, hash1, hashScopeContent(sv3))
}

func TestNormalizedResourceDeduplication(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

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

	size, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)
	require.Positive(t, size)

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

	for i := uint64(1); i <= 50; i++ {
		sv := NewScopeVersion("otel-go", "1.0", "", map[string]string{"lang": "go"}, 1000, 5000)
		mem.SetVersionedScope(i, NewVersionedScope(sv))
	}
	for i := uint64(51); i <= 100; i++ {
		sv := NewScopeVersion("otel-java", "2.0", "", map[string]string{"lang": "java"}, 1000, 5000)
		mem.SetVersionedScope(i, NewVersionedScope(sv))
	}

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(100), reader.TotalScopes())

	for i := uint64(1); i <= 50; i++ {
		vs, found := reader.GetVersionedScope(i)
		require.True(t, found)
		require.Len(t, vs.Versions, 1)
		require.Equal(t, "otel-go", vs.Versions[0].Name)
		require.Equal(t, "go", vs.Versions[0].Attrs["lang"])
	}

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

	require.Equal(t, uint64(15), reader.TotalResources())

	for i := uint64(1); i <= 5; i++ {
		r, found := reader.GetResource(i)
		require.True(t, found)
		require.Equal(t, fmt.Sprintf("unique-%d", i), r.Identifying["service.name"])
	}
	for i := uint64(6); i <= 15; i++ {
		r, found := reader.GetResource(i)
		require.True(t, found)
		require.Equal(t, "shared", r.Identifying["service.name"])
	}
}

func TestNormalizedResourceWithEntities(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

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

// buildTestData creates a MemSeriesMetadata with resources and scopes for use in multiple tests.
func buildTestData(t *testing.T) *MemSeriesMetadata {
	t.Helper()
	mem := NewMemSeriesMetadata()

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

	nsColIdx := lookupColumnIndex(pf.Schema(), "namespace")
	require.GreaterOrEqual(t, nsColIdx, 0)

	seenNamespaces := make(map[string]bool)
	for _, rg := range pf.RowGroups() {
		ns, ok := rowGroupSingleNamespace(rg, nsColIdx)
		require.True(t, ok, "row group should be single-namespace")
		seenNamespaces[ns] = true
	}

	require.True(t, seenNamespaces[NamespaceResourceTable], "expected resource_table namespace")
	require.True(t, seenNamespaces[NamespaceResourceMapping], "expected resource_mapping namespace")
	require.True(t, seenNamespaces[NamespaceScopeTable], "expected scope_table namespace")
	require.True(t, seenNamespaces[NamespaceScopeMapping], "expected scope_mapping namespace")
}

func TestWriteFileWithOptions_MaxRowsPerRowGroup(t *testing.T) {
	tmpdir := t.TempDir()
	mem := NewMemSeriesMetadata()

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

	seriesRefIdx := lookupColumnIndex(pf.Schema(), "series_ref")
	contentHashIdx := lookupColumnIndex(pf.Schema(), "content_hash")
	require.GreaterOrEqual(t, seriesRefIdx, 0)
	require.GreaterOrEqual(t, contentHashIdx, 0)

	var foundSeriesRefBloom, foundContentBloom bool
	for _, rg := range pf.RowGroups() {
		ccs := rg.ColumnChunks()
		if bf := ccs[seriesRefIdx].BloomFilter(); bf != nil {
			foundSeriesRefBloom = true
		}
		if bf := ccs[contentHashIdx].BloomFilter(); bf != nil {
			foundContentBloom = true
		}
	}
	require.True(t, foundSeriesRefBloom, "expected bloom filter on series_ref column")
	require.True(t, foundContentBloom, "expected bloom filter on content_hash column")
}

func TestWriteFileWithOptions_RoundTrip(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{
		MaxRowsPerRowGroup: 2,
		EnableBloomFilters: true,
	})
	require.NoError(t, err)

	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader.Close()

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

	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	stat, err := f.Stat()
	require.NoError(t, err)

	reader, err := ReadSeriesMetadataFromReaderAt(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(4), reader.TotalResources())
	require.Equal(t, uint64(2), reader.TotalScopes())

	r, found := reader.GetResource(100)
	require.True(t, found)
	require.Equal(t, "svc-100", r.Identifying["service.name"])
}

func TestReadSeriesMetadataFromReaderAt_BytesReader(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	_, err := WriteFile(promslog.NewNopLogger(), tmpdir, mem)
	require.NoError(t, err)

	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	reader, err := ReadSeriesMetadataFromReaderAt(promslog.NewNopLogger(), bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, uint64(4), reader.TotalResources())

	r, found := reader.GetResource(201)
	require.True(t, found)
	require.Equal(t, "shared-svc", r.Identifying["service.name"])
}

func TestReadSeriesMetadataFromReaderAt_NamespaceFilter(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

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

	// Scopes should be empty.
	require.Equal(t, uint64(0), reader.TotalScopes())
}

func TestReadSeriesMetadataFromReaderAt_NamespaceFilter_EmptyResult(t *testing.T) {
	tmpdir := t.TempDir()
	mem := NewMemSeriesMetadata()
	// Only add a scope, no resources.
	sv := NewScopeVersion("lib", "1.0", "", nil, 1000, 5000)
	mem.SetVersionedScope(1, NewVersionedScope(sv))

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

	require.Equal(t, uint64(0), reader.TotalResources())
	require.Equal(t, uint64(0), reader.TotalScopes())
}

func TestReadOldSingleRowGroupFile(t *testing.T) {
	tmpdir := t.TempDir()
	mem := buildTestData(t)

	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, WriterOptions{})
	require.NoError(t, err)

	reader1, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir)
	require.NoError(t, err)
	defer reader1.Close()

	require.Equal(t, uint64(4), reader1.TotalResources())
	require.Equal(t, uint64(2), reader1.TotalScopes())

	path := filepath.Join(tmpdir, SeriesMetadataFilename)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	reader2, err := ReadSeriesMetadataFromReaderAt(promslog.NewNopLogger(), bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer reader2.Close()

	require.Equal(t, uint64(4), reader2.TotalResources())
	require.Equal(t, uint64(2), reader2.TotalScopes())
}

func TestRefResolverRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()

	mem := NewMemSeriesMetadata()

	// labelsHash → seriesRef mapping (simulating a block index).
	labelsHashToRef := map[uint64]uint64{
		1000: 10, // labelsHash 1000 → seriesRef 10
		2000: 20, // labelsHash 2000 → seriesRef 20
	}
	refToLabelsHash := map[uint64]uint64{
		10: 1000,
		20: 2000,
	}

	// Add resources and scopes keyed by labelsHash.
	rv1 := NewResourceVersion(
		map[string]string{"service.name": "frontend"},
		map[string]string{"env": "prod"},
		nil, 1000, 5000,
	)
	mem.SetResource(1000, rv1)

	rv2 := NewResourceVersion(
		map[string]string{"service.name": "backend"},
		nil, nil, 2000, 6000,
	)
	mem.SetResource(2000, rv2)

	sv := NewScopeVersion("otel-go", "1.0", "", map[string]string{"lang": "go"}, 1000, 5000)
	mem.SetVersionedScope(1000, NewVersionedScope(sv))

	// Write with RefResolver (labelsHash → seriesRef).
	wopts := WriterOptions{
		RefResolver: func(labelsHash uint64) (uint64, bool) {
			ref, ok := labelsHashToRef[labelsHash]
			return ref, ok
		},
	}
	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, wopts)
	require.NoError(t, err)

	// Read back with RefResolver (seriesRef → labelsHash).
	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), tmpdir,
		WithRefResolver(func(seriesRef uint64) (uint64, bool) {
			lh, ok := refToLabelsHash[seriesRef]
			return lh, ok
		}),
	)
	require.NoError(t, err)
	defer reader.Close()

	// Verify data is accessible via the original labelsHash keys.
	require.Equal(t, uint64(2), reader.TotalResources())
	require.Equal(t, uint64(1), reader.TotalScopes())

	r1, found := reader.GetResource(1000)
	require.True(t, found)
	require.Equal(t, "frontend", r1.Identifying["service.name"])
	require.Equal(t, "prod", r1.Descriptive["env"])

	r2, found := reader.GetResource(2000)
	require.True(t, found)
	require.Equal(t, "backend", r2.Identifying["service.name"])

	vs, found := reader.GetVersionedScope(1000)
	require.True(t, found)
	require.Len(t, vs.Versions, 1)
	require.Equal(t, "otel-go", vs.Versions[0].Name)
}
