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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/metadata"
)

func TestAddOrExtend_NewVersion(t *testing.T) {
	vm := &VersionedMetadata{}
	v := &MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "total requests"},
		MinTime: 100,
		MaxTime: 200,
	}
	vm.AddOrExtend(v)

	require.Len(t, vm.Versions, 1)
	require.Equal(t, int64(100), vm.Versions[0].MinTime)
	require.Equal(t, int64(200), vm.Versions[0].MaxTime)
	require.Equal(t, "total requests", vm.Versions[0].Meta.Help)
}

func TestAddOrExtend_ExtendSameContent(t *testing.T) {
	vm := &VersionedMetadata{}
	meta := metadata.Metadata{Type: model.MetricTypeCounter, Help: "total requests"}

	vm.AddOrExtend(&MetadataVersion{Meta: meta, MinTime: 100, MaxTime: 200})
	vm.AddOrExtend(&MetadataVersion{Meta: meta, MinTime: 200, MaxTime: 300})

	require.Len(t, vm.Versions, 1)
	require.Equal(t, int64(100), vm.Versions[0].MinTime)
	require.Equal(t, int64(300), vm.Versions[0].MaxTime)
}

func TestAddOrExtend_NewVersionOnChange(t *testing.T) {
	vm := &VersionedMetadata{}

	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "total requests"},
		MinTime: 100,
		MaxTime: 200,
	})
	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "total HTTP requests processed"},
		MinTime: 201,
		MaxTime: 300,
	})

	require.Len(t, vm.Versions, 2)
	require.Equal(t, "total requests", vm.Versions[0].Meta.Help)
	require.Equal(t, "total HTTP requests processed", vm.Versions[1].Meta.Help)
}

func TestAddOrExtend_ExtendMinTime(t *testing.T) {
	vm := &VersionedMetadata{}
	meta := metadata.Metadata{Type: model.MetricTypeGauge, Help: "temperature"}

	vm.AddOrExtend(&MetadataVersion{Meta: meta, MinTime: 200, MaxTime: 300})
	vm.AddOrExtend(&MetadataVersion{Meta: meta, MinTime: 100, MaxTime: 250})

	require.Len(t, vm.Versions, 1)
	require.Equal(t, int64(100), vm.Versions[0].MinTime)
	require.Equal(t, int64(300), vm.Versions[0].MaxTime)
}

func TestCurrentVersion(t *testing.T) {
	vm := &VersionedMetadata{}
	require.Nil(t, vm.CurrentVersion())

	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "v1"},
		MinTime: 100,
		MaxTime: 200,
	})
	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeGauge, Help: "v2"},
		MinTime: 201,
		MaxTime: 300,
	})

	cur := vm.CurrentVersion()
	require.NotNil(t, cur)
	require.Equal(t, "v2", cur.Meta.Help)
	require.Equal(t, model.MetricTypeGauge, cur.Meta.Type)
}

func TestVersionAt(t *testing.T) {
	vm := &VersionedMetadata{}
	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "v1"},
		MinTime: 100,
		MaxTime: 200,
	})
	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeGauge, Help: "v2"},
		MinTime: 300,
		MaxTime: 400,
	})

	require.Nil(t, vm.VersionAt(50))                    // Before any version.
	require.Equal(t, "v1", vm.VersionAt(100).Meta.Help) // Exactly at v1 MinTime.
	require.Equal(t, "v1", vm.VersionAt(150).Meta.Help) // Within v1 range.
	require.Equal(t, "v1", vm.VersionAt(200).Meta.Help) // Exactly at v1 MaxTime.
	require.Equal(t, "v1", vm.VersionAt(250).Meta.Help) // In gap: v1 remains active until v2 starts.
	require.Equal(t, "v2", vm.VersionAt(300).Meta.Help) // Exactly at v2 MinTime.
	require.Equal(t, "v2", vm.VersionAt(400).Meta.Help) // Exactly at v2 MaxTime.
	require.Equal(t, "v2", vm.VersionAt(500).Meta.Help) // After last version: last known version persists.
}

func TestCopy(t *testing.T) {
	vm := &VersionedMetadata{}
	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "original"},
		MinTime: 100,
		MaxTime: 200,
	})

	cp := vm.Copy()
	require.Len(t, cp.Versions, 1)
	require.Equal(t, "original", cp.Versions[0].Meta.Help)

	// Mutate original, copy should be unaffected.
	vm.Versions[0].Meta.Help = "modified"
	require.Equal(t, "original", cp.Versions[0].Meta.Help)

	// Nil copy.
	var nilVM *VersionedMetadata
	require.Nil(t, nilVM.Copy())
}

func TestMergeVersionedMetadata(t *testing.T) {
	a := &VersionedMetadata{}
	a.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "v1"},
		MinTime: 100,
		MaxTime: 200,
	})
	a.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeGauge, Help: "v2"},
		MinTime: 300,
		MaxTime: 400,
	})

	b := &VersionedMetadata{}
	b.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "v1"},
		MinTime: 150,
		MaxTime: 250,
	})
	b.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeGauge, Help: "v2"},
		MinTime: 350,
		MaxTime: 500,
	})

	merged := MergeVersionedMetadata(a, b)
	require.Len(t, merged.Versions, 2)
	// v1 should be coalesced: min(100,150)=100, max(250,200)=250
	require.Equal(t, "v1", merged.Versions[0].Meta.Help)
	require.Equal(t, int64(100), merged.Versions[0].MinTime)
	require.Equal(t, int64(250), merged.Versions[0].MaxTime)
	// v2 should be coalesced: min(300,350)=300, max(400,500)=500
	require.Equal(t, "v2", merged.Versions[1].Meta.Help)
	require.Equal(t, int64(300), merged.Versions[1].MinTime)
	require.Equal(t, int64(500), merged.Versions[1].MaxTime)
}

func TestMergeVersionedMetadata_NilInputs(t *testing.T) {
	a := &VersionedMetadata{}
	a.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "v1"},
		MinTime: 100,
		MaxTime: 200,
	})

	require.Equal(t, a, MergeVersionedMetadata(a, nil))
	require.Equal(t, a, MergeVersionedMetadata(nil, a))
	require.Nil(t, MergeVersionedMetadata(nil, nil))
}

func TestMergeVersionedMetadata_Interleaved(t *testing.T) {
	a := &VersionedMetadata{}
	a.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "counter"},
		MinTime: 100,
		MaxTime: 200,
	})
	a.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeGauge, Help: "gauge"},
		MinTime: 300,
		MaxTime: 400,
	})

	b := &VersionedMetadata{}
	b.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeSummary, Help: "summary"},
		MinTime: 250,
		MaxTime: 350,
	})

	merged := MergeVersionedMetadata(a, b)
	require.Len(t, merged.Versions, 3)
	require.Equal(t, "counter", merged.Versions[0].Meta.Help)
	require.Equal(t, "summary", merged.Versions[1].Meta.Help)
	require.Equal(t, "gauge", merged.Versions[2].Meta.Help)
}

func TestHashMetadataContent(t *testing.T) {
	m1 := metadata.Metadata{Type: model.MetricTypeCounter, Help: "total requests", Unit: ""}
	m2 := metadata.Metadata{Type: model.MetricTypeCounter, Help: "total requests", Unit: ""}
	m3 := metadata.Metadata{Type: model.MetricTypeCounter, Help: "total requests", Unit: "bytes"}

	require.Equal(t, HashMetadataContent(m1), HashMetadataContent(m2))
	require.NotEqual(t, HashMetadataContent(m1), HashMetadataContent(m3))
}

func TestMetadataVersionsEqual(t *testing.T) {
	v1 := &MetadataVersion{Meta: metadata.Metadata{Type: model.MetricTypeCounter, Help: "a"}, MinTime: 1, MaxTime: 2}
	v2 := &MetadataVersion{Meta: metadata.Metadata{Type: model.MetricTypeCounter, Help: "a"}, MinTime: 10, MaxTime: 20}
	v3 := &MetadataVersion{Meta: metadata.Metadata{Type: model.MetricTypeGauge, Help: "a"}, MinTime: 1, MaxTime: 2}

	require.True(t, MetadataVersionsEqual(v1, v2))
	require.False(t, MetadataVersionsEqual(v1, v3))
	require.False(t, MetadataVersionsEqual(v1, nil))
	require.False(t, MetadataVersionsEqual(nil, v1))
	require.True(t, MetadataVersionsEqual(nil, nil))
}

func TestFilterVersionsByTimeRange(t *testing.T) {
	vm := &VersionedMetadata{}
	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "v1"},
		MinTime: 100,
		MaxTime: 200,
	})
	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeGauge, Help: "v2"},
		MinTime: 300,
		MaxTime: 400,
	})

	// Both overlap.
	filtered := FilterVersionsByTimeRange(vm, 150, 350)
	require.Len(t, filtered.Versions, 2)

	// Only first.
	filtered = FilterVersionsByTimeRange(vm, 50, 150)
	require.Len(t, filtered.Versions, 1)
	require.Equal(t, "v1", filtered.Versions[0].Meta.Help)

	// Only second.
	filtered = FilterVersionsByTimeRange(vm, 350, 500)
	require.Len(t, filtered.Versions, 1)
	require.Equal(t, "v2", filtered.Versions[0].Meta.Help)

	// No overlap.
	filtered = FilterVersionsByTimeRange(vm, 210, 290)
	require.Nil(t, filtered)

	// Nil input.
	require.Nil(t, FilterVersionsByTimeRange(nil, 0, 1000))
}

func TestCurrentMetadata(t *testing.T) {
	vm := &VersionedMetadata{}
	require.Nil(t, vm.CurrentMetadata())

	vm.AddOrExtend(&MetadataVersion{
		Meta:    metadata.Metadata{Type: model.MetricTypeCounter, Help: "hello"},
		MinTime: 1,
		MaxTime: 2,
	})

	m := vm.CurrentMetadata()
	require.NotNil(t, m)
	require.Equal(t, "hello", m.Help)
}
