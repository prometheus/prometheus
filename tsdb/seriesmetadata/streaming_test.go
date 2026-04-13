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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

// writeTestFile writes test metadata to a Parquet file and returns the file path.
func writeTestFile(t *testing.T, mem *MemSeriesMetadata, wopts WriterOptions) string {
	t.Helper()
	tmpdir := t.TempDir()
	_, err := WriteFileWithOptions(promslog.NewNopLogger(), tmpdir, mem, wopts)
	require.NoError(t, err)
	return filepath.Join(tmpdir, SeriesMetadataFilename)
}

func openTestFile(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	return f
}

func TestOpenParquetFile(t *testing.T) {
	mem := buildTestData(t)
	path := writeTestFile(t, mem, WriterOptions{})

	f := openTestFile(t, path)
	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := OpenParquetFile(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)
	require.NotNil(t, pf)
}

func TestStreamVersionedResourcesFromFile(t *testing.T) {
	mem := buildTestData(t)
	path := writeTestFile(t, mem, WriterOptions{})

	f := openTestFile(t, path)
	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := OpenParquetFile(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)

	// Stream and collect all resources.
	collected := make(map[uint64]*VersionedResource)
	err = StreamVersionedResourcesFromFile(promslog.NewNopLogger(), pf,
		WithOnVersionedResource(func(labelsHash uint64, vr *VersionedResource) error {
			collected[labelsHash] = vr
			return nil
		}),
	)
	require.NoError(t, err)

	// Compare with batch reader.
	reader, _, err := ReadSeriesMetadata(promslog.NewNopLogger(), filepath.Dir(path))
	require.NoError(t, err)
	defer reader.Close()

	// Verify same count.
	require.Len(t, collected, int(reader.TotalResources()))

	// Verify each entry matches.
	err = reader.IterVersionedResources(context.Background(), func(labelsHash uint64, vr *VersionedResource) error {
		got, ok := collected[labelsHash]
		require.True(t, ok, "labelsHash %d missing from streaming result", labelsHash)
		require.Len(t, got.Versions, len(vr.Versions))
		for i, expected := range vr.Versions {
			actual := got.Versions[i]
			require.Equal(t, expected.Identifying, actual.Identifying)
			require.Equal(t, expected.Descriptive, actual.Descriptive)
			require.Equal(t, expected.MinTime, actual.MinTime)
			require.Equal(t, expected.MaxTime, actual.MaxTime)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestStreamVersionedResourcesFromFile_TimeRangeFilter(t *testing.T) {
	mem := NewMemSeriesMetadata()

	// Series with two versions at different time ranges.
	rv1 := NewResourceVersion(
		map[string]string{"service.name": "svc"},
		nil, 1000, 2000,
	)
	rv2 := NewResourceVersion(
		map[string]string{"service.name": "svc"},
		map[string]string{"env": "v2"},
		5000, 6000,
	)
	vr := NewVersionedResource(rv1)
	vr.Versions = append(vr.Versions, rv2)
	mem.SetVersionedResource(100, vr)

	path := writeTestFile(t, mem, WriterOptions{})
	f := openTestFile(t, path)
	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := OpenParquetFile(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)

	// Filter to only versions overlapping [4000, 7000] — should get only rv2.
	collected := make(map[uint64]*VersionedResource)
	err = StreamVersionedResourcesFromFile(promslog.NewNopLogger(), pf,
		WithTimeRange(4000, 7000),
		WithOnVersionedResource(func(labelsHash uint64, vr *VersionedResource) error {
			collected[labelsHash] = vr
			return nil
		}),
	)
	require.NoError(t, err)

	require.Len(t, collected, 1)
	got := collected[100]
	require.Len(t, got.Versions, 1)
	require.Equal(t, int64(5000), got.Versions[0].MinTime)
	require.Equal(t, "v2", got.Versions[0].Descriptive["env"])
}

func TestStreamVersionedResourcesFromFile_TimeRangeAtZero(t *testing.T) {
	mem := NewMemSeriesMetadata()

	// Version with MinTime=0 — must not be silently excluded by the filter.
	rv := NewResourceVersion(
		map[string]string{"service.name": "svc"},
		nil, 0, 1000,
	)
	mem.SetVersionedResource(100, NewVersionedResource(rv))

	path := writeTestFile(t, mem, WriterOptions{})
	f := openTestFile(t, path)
	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := OpenParquetFile(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)

	// Filter [0, 500] — should include the version since it overlaps.
	collected := make(map[uint64]*VersionedResource)
	err = StreamVersionedResourcesFromFile(promslog.NewNopLogger(), pf,
		WithTimeRange(0, 500),
		WithOnVersionedResource(func(labelsHash uint64, vr *VersionedResource) error {
			collected[labelsHash] = vr
			return nil
		}),
	)
	require.NoError(t, err)
	require.Len(t, collected, 1)
	require.Equal(t, int64(0), collected[100].Versions[0].MinTime)
}

func TestStreamVersionedResourcesFromFile_SeriesRefFilter(t *testing.T) {
	mem := NewMemSeriesMetadata()

	for i := uint64(1); i <= 4; i++ {
		rv := NewResourceVersion(
			map[string]string{"service.name": fmt.Sprintf("svc-%d", i)}, nil, 1000, 5000,
		)
		mem.SetResource(i, rv)
	}

	path := writeTestFile(t, mem, WriterOptions{})
	f := openTestFile(t, path)
	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := OpenParquetFile(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)

	// Only allow series 2 and 4 (note: without RefResolver, SeriesRef == labelsHash).
	allowed := map[uint64]bool{2: true, 4: true}
	collected := make(map[uint64]*VersionedResource)
	err = StreamVersionedResourcesFromFile(promslog.NewNopLogger(), pf,
		WithSeriesRefFilter(func(seriesRef uint64) bool { return allowed[seriesRef] }),
		WithOnVersionedResource(func(labelsHash uint64, vr *VersionedResource) error {
			collected[labelsHash] = vr
			return nil
		}),
	)
	require.NoError(t, err)

	require.Len(t, collected, 2)
	require.Contains(t, collected, uint64(2))
	require.Contains(t, collected, uint64(4))
	require.Equal(t, "svc-2", collected[2].Versions[0].Identifying["service.name"])
	require.Equal(t, "svc-4", collected[4].Versions[0].Identifying["service.name"])
}

func TestStreamResourceAttrIndexFromFile(t *testing.T) {
	mem := NewMemSeriesMetadata()

	rv1 := NewResourceVersion(
		map[string]string{"service.name": "svc-a"},
		map[string]string{"host.name": "host-1"},
		1000, 5000,
	)
	mem.SetVersionedResource(100, NewVersionedResource(rv1))

	rv2 := NewResourceVersion(
		map[string]string{"service.name": "svc-b"},
		map[string]string{"host.name": "host-1"},
		1000, 5000,
	)
	mem.SetVersionedResource(200, NewVersionedResource(rv2))

	path := writeTestFile(t, mem, WriterOptions{
		EnableInvertedIndex:  true,
		IndexedResourceAttrs: map[string]struct{}{"host.name": {}},
	})
	f := openTestFile(t, path)
	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := OpenParquetFile(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)

	// Stream index entries.
	type entry struct {
		key, value string
		seriesRef  uint64
	}
	var entries []entry
	err = StreamResourceAttrIndexFromFile(pf,
		WithOnAttrIndexEntry(func(attrKey, attrValue string, seriesRef uint64) error {
			entries = append(entries, entry{attrKey, attrValue, seriesRef})
			return nil
		}),
	)
	require.NoError(t, err)

	// Verify expected entries.
	expected := []entry{
		{"service.name", "svc-a", 100},
		{"service.name", "svc-b", 200},
		{"host.name", "host-1", 100},
		{"host.name", "host-1", 200},
	}
	require.ElementsMatch(t, expected, entries)
}

func TestOpenParquetFile_Reuse(t *testing.T) {
	mem := NewMemSeriesMetadata()
	rv := NewResourceVersion(
		map[string]string{"service.name": "svc"},
		map[string]string{"env": "prod"},
		1000, 5000,
	)
	mem.SetVersionedResource(100, NewVersionedResource(rv))

	path := writeTestFile(t, mem, WriterOptions{
		EnableInvertedIndex:  true,
		IndexedResourceAttrs: map[string]struct{}{"env": {}},
	})
	f := openTestFile(t, path)
	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := OpenParquetFile(promslog.NewNopLogger(), f, stat.Size())
	require.NoError(t, err)

	// Use the same *parquet.File for both streaming functions.
	var resourceCount int
	err = StreamVersionedResourcesFromFile(promslog.NewNopLogger(), pf,
		WithOnVersionedResource(func(_ uint64, _ *VersionedResource) error {
			resourceCount++
			return nil
		}),
	)
	require.NoError(t, err)
	require.Equal(t, 1, resourceCount)

	var indexCount int
	err = StreamResourceAttrIndexFromFile(pf,
		WithOnAttrIndexEntry(func(_, _ string, _ uint64) error {
			indexCount++
			return nil
		}),
	)
	require.NoError(t, err)
	require.Positive(t, indexCount)
}
