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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestLayeredReader_LabelsForHash(t *testing.T) {
	base := NewMemSeriesMetadata()
	top := NewMemSeriesMetadata()

	baseLbls := labels.FromStrings("__name__", "base_metric", "job", "base")
	topLbls := labels.FromStrings("__name__", "top_metric", "job", "top")
	sharedLbls := labels.FromStrings("__name__", "shared_metric", "job", "shared")

	baseHash := uint64(1)
	topHash := uint64(2)
	sharedHash := uint64(3)

	base.SetLabels(baseHash, baseLbls)
	base.SetLabels(sharedHash, labels.FromStrings("__name__", "shared_metric", "job", "old"))

	top.SetLabels(topHash, topLbls)
	top.SetLabels(sharedHash, sharedLbls)

	lr := NewLayeredReader(base, top)
	defer lr.Close()

	// Base-only hash.
	lset, ok := lr.LabelsForHash(baseHash)
	require.True(t, ok)
	require.Equal(t, baseLbls, lset)

	// Top-only hash.
	lset, ok = lr.LabelsForHash(topHash)
	require.True(t, ok)
	require.Equal(t, topLbls, lset)

	// Shared hash — top wins.
	lset, ok = lr.LabelsForHash(sharedHash)
	require.True(t, ok)
	require.Equal(t, sharedLbls, lset)

	// Unknown hash.
	_, ok = lr.LabelsForHash(999)
	require.False(t, ok)
}

func TestLayeredReader_ResourceMerge(t *testing.T) {
	base := NewMemSeriesMetadata()
	top := NewMemSeriesMetadata()

	hash := uint64(42)

	// Base has an older version.
	base.SetVersionedResource(hash, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(
				map[string]string{"service.name": "svc-a"},
				map[string]string{"host.name": "host-1"},
				nil, 1000, 2000,
			),
		},
	})

	// Top has a newer version.
	top.SetVersionedResource(hash, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(
				map[string]string{"service.name": "svc-a"},
				map[string]string{"host.name": "host-2"},
				nil, 3000, 4000,
			),
		},
	})

	lr := NewLayeredReader(base, top)
	defer lr.Close()

	// GetVersionedResource should merge both.
	vr, ok := lr.GetVersionedResource(hash)
	require.True(t, ok)
	require.Len(t, vr.Versions, 2)
	require.Equal(t, "host-1", vr.Versions[0].Descriptive["host.name"])
	require.Equal(t, "host-2", vr.Versions[1].Descriptive["host.name"])
}

func TestLayeredReader_IterDedup(t *testing.T) {
	base := NewMemSeriesMetadata()
	top := NewMemSeriesMetadata()

	hash1 := uint64(1)
	hash2 := uint64(2)
	hash3 := uint64(3)

	// hash1 in base only.
	base.SetVersionedResource(hash1, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"service.name": "svc-1"}, nil, nil, 100, 200),
		},
	})

	// hash2 in both — should appear once with merged versions.
	base.SetVersionedResource(hash2, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"service.name": "svc-2"}, nil, nil, 100, 200),
		},
	})
	top.SetVersionedResource(hash2, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"service.name": "svc-2"}, map[string]string{"k": "v"}, nil, 300, 400),
		},
	})

	// hash3 in top only.
	top.SetVersionedResource(hash3, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"service.name": "svc-3"}, nil, nil, 500, 600),
		},
	})

	lr := NewLayeredReader(base, top)
	defer lr.Close()

	seen := make(map[uint64]int)
	err := lr.IterVersionedResources(context.Background(), func(labelsHash uint64, resources *VersionedResource) error {
		seen[labelsHash]++
		if labelsHash == hash2 {
			// Merged: should have 2 versions.
			require.Len(t, resources.Versions, 2)
		}
		return nil
	})
	require.NoError(t, err)

	require.Len(t, seen, 3)
	require.Equal(t, 1, seen[hash1])
	require.Equal(t, 1, seen[hash2]) // Deduped, not 2.
	require.Equal(t, 1, seen[hash3])
}

func TestLayeredReader_LookupResourceAttrUnion(t *testing.T) {
	base := NewMemSeriesMetadata()
	top := NewMemSeriesMetadata()

	baseHash := uint64(10)
	topHash := uint64(20)
	sharedHash := uint64(30)

	base.SetVersionedResource(baseHash, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"region": "us-west"}, nil, nil, 100, 200),
		},
	})
	base.SetVersionedResource(sharedHash, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"region": "us-west"}, nil, nil, 100, 200),
		},
	})
	base.BuildResourceAttrIndex()

	top.InitResourceAttrIndex()
	top.SetVersionedResource(topHash, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"region": "us-west"}, nil, nil, 300, 400),
		},
	})
	top.SetVersionedResource(sharedHash, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"region": "us-west"}, nil, nil, 300, 400),
		},
	})
	// Simulate incremental index update.
	top.UpdateResourceAttrIndex(topHash, nil, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"region": "us-west"}, nil, nil, 300, 400),
		},
	})
	top.UpdateResourceAttrIndex(sharedHash, nil, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"region": "us-west"}, nil, nil, 300, 400),
		},
	})

	lr := NewLayeredReader(base, top)
	defer lr.Close()

	// Union should contain all three hashes.
	result := lr.LookupResourceAttr("region", "us-west")
	require.NotNil(t, result)
	require.Len(t, result, 3)
	require.Contains(t, result, baseHash)
	require.Contains(t, result, topHash)
	require.Contains(t, result, sharedHash)

	// Unknown key should return nil.
	result = lr.LookupResourceAttr("unknown", "value")
	require.Nil(t, result)
}

func TestSelectiveResourceAttrIndexing(t *testing.T) {
	m := NewMemSeriesMetadata()

	hash1 := uint64(100)
	hash2 := uint64(200)

	// hash1: identifying=service.name, descriptive=k8s.namespace.name + host.name
	m.SetVersionedResource(hash1, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(
				map[string]string{"service.name": "frontend"},
				map[string]string{"k8s.namespace.name": "prod", "host.name": "node-1"},
				nil, 100, 200,
			),
		},
	})
	// hash2: identifying=service.name, descriptive=k8s.namespace.name + cloud.region
	m.SetVersionedResource(hash2, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(
				map[string]string{"service.name": "backend"},
				map[string]string{"k8s.namespace.name": "staging", "cloud.region": "us-east-1"},
				nil, 300, 400,
			),
		},
	})

	t.Run("no extra indexed attrs", func(t *testing.T) {
		// Only identifying attrs should be indexed.
		m.SetIndexedResourceAttrs(nil)
		m.resourceAttrIndex = nil // force rebuild
		m.BuildResourceAttrIndex()

		// Identifying attrs are always indexed.
		result := m.LookupResourceAttr("service.name", "frontend")
		require.NotNil(t, result)
		require.Contains(t, result, hash1)

		result = m.LookupResourceAttr("service.name", "backend")
		require.NotNil(t, result)
		require.Contains(t, result, hash2)

		// Descriptive attrs should NOT be indexed.
		result = m.LookupResourceAttr("k8s.namespace.name", "prod")
		require.Nil(t, result)
		result = m.LookupResourceAttr("host.name", "node-1")
		require.Nil(t, result)
		result = m.LookupResourceAttr("cloud.region", "us-east-1")
		require.Nil(t, result)
	})

	t.Run("with extra indexed attrs", func(t *testing.T) {
		// Index k8s.namespace.name but not host.name or cloud.region.
		m.SetIndexedResourceAttrs(map[string]struct{}{"k8s.namespace.name": {}})
		m.resourceAttrIndex = nil // force rebuild
		m.BuildResourceAttrIndex()

		// Identifying attrs still indexed.
		result := m.LookupResourceAttr("service.name", "frontend")
		require.NotNil(t, result)
		require.Contains(t, result, hash1)

		// k8s.namespace.name is now indexed.
		result = m.LookupResourceAttr("k8s.namespace.name", "prod")
		require.NotNil(t, result)
		require.Contains(t, result, hash1)

		result = m.LookupResourceAttr("k8s.namespace.name", "staging")
		require.NotNil(t, result)
		require.Contains(t, result, hash2)

		// host.name and cloud.region should NOT be indexed.
		result = m.LookupResourceAttr("host.name", "node-1")
		require.Nil(t, result)
		result = m.LookupResourceAttr("cloud.region", "us-east-1")
		require.Nil(t, result)
	})

	t.Run("incremental update respects filter", func(t *testing.T) {
		m2 := NewMemSeriesMetadata()
		m2.SetIndexedResourceAttrs(map[string]struct{}{"k8s.namespace.name": {}})
		m2.InitResourceAttrIndex()

		hash3 := uint64(300)
		vr := &VersionedResource{
			Versions: []*ResourceVersion{
				NewResourceVersion(
					map[string]string{"service.name": "api"},
					map[string]string{"k8s.namespace.name": "default", "host.name": "node-2"},
					nil, 500, 600,
				),
			},
		}
		m2.SetVersionedResource(hash3, vr)
		m2.UpdateResourceAttrIndex(hash3, nil, vr)

		// Identifying and configured descriptive attrs should be indexed.
		require.NotNil(t, m2.LookupResourceAttr("service.name", "api"))
		require.NotNil(t, m2.LookupResourceAttr("k8s.namespace.name", "default"))

		// Unconfigured descriptive attrs should NOT be indexed.
		require.Nil(t, m2.LookupResourceAttr("host.name", "node-2"))
	})
}

func TestLayeredReader_ScopeMerge(t *testing.T) {
	base := NewMemSeriesMetadata()
	top := NewMemSeriesMetadata()

	hash := uint64(42)

	base.SetVersionedScope(hash, &VersionedScope{
		Versions: []*ScopeVersion{
			NewScopeVersion("otel-go", "1.0", "", nil, 1000, 2000),
		},
	})
	top.SetVersionedScope(hash, &VersionedScope{
		Versions: []*ScopeVersion{
			NewScopeVersion("otel-go", "2.0", "", nil, 3000, 4000),
		},
	})

	lr := NewLayeredReader(base, top)
	defer lr.Close()

	vs, ok := lr.GetVersionedScope(hash)
	require.True(t, ok)
	require.Len(t, vs.Versions, 2)
	require.Equal(t, "1.0", vs.Versions[0].Version)
	require.Equal(t, "2.0", vs.Versions[1].Version)
}

func TestLayeredReader_KindLen(t *testing.T) {
	base := NewMemSeriesMetadata()
	top := NewMemSeriesMetadata()

	base.SetVersionedResource(1, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"k": "v"}, nil, nil, 100, 200),
		},
	})
	top.SetVersionedResource(2, &VersionedResource{
		Versions: []*ResourceVersion{
			NewResourceVersion(map[string]string{"k": "v"}, nil, nil, 300, 400),
		},
	})

	lr := NewLayeredReader(base, top)
	defer lr.Close()

	// KindLen is approximate (sum of both, may overcount if shared).
	require.Equal(t, 2, lr.KindLen(KindResource))
}
