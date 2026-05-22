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
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
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

	rv := NewResourceVersion(identifying, descriptive, 1000, 2000)

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
	rv := NewResourceVersion(identifying, nil, 1000, 2000)

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
	rv1 := NewResourceVersion(identifying1, descriptive1, 1000, 2000)
	store.Set(123, rv1)

	// Second set for same hash with different attributes - should create new version
	identifying2 := map[string]string{"service.name": "my-service"}
	descriptive2 := map[string]string{"host.name": "host-1"}
	rv2 := NewResourceVersion(identifying2, descriptive2, 3000, 4000)
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
	rv1 := NewResourceVersion(identifying, nil, 1000, 2000)
	store.Set(123, rv1)

	rv2 := NewResourceVersion(identifying, nil, 3000, 4000)
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
		rv := NewResourceVersion(identifying, nil, int64(i*1000), int64(i*2000))
		store.Set(i, rv)
	}

	collected := make(map[uint64]*ResourceVersion)
	err := store.IterVersionedFlatInline(context.Background(), func(labelsHash uint64, versions []*ResourceVersion, _, _ int64, _ bool) error {
		if len(versions) > 0 {
			collected[labelsHash] = versions[len(versions)-1]
		}
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

func TestBuildResourceAttrIndex(t *testing.T) {
	mem := NewMemSeriesMetadata()

	// Series 1: payment-service in production
	rv1 := NewResourceVersion(
		map[string]string{"service.name": "payment-service", "service.namespace": "production"},
		map[string]string{"host.name": "host-1"}, 1000, 2000,
	)
	mem.SetVersionedResource(100, NewVersionedResource(rv1))

	// Series 2: payment-service in staging (same service.name, different namespace)
	rv2 := NewResourceVersion(
		map[string]string{"service.name": "payment-service", "service.namespace": "staging"},
		map[string]string{"host.name": "host-2"}, 1000, 2000,
	)
	mem.SetVersionedResource(200, NewVersionedResource(rv2))

	// Series 3: frontend-service in production
	rv3 := NewResourceVersion(
		map[string]string{"service.name": "frontend-service", "service.namespace": "production"},
		map[string]string{"host.name": "host-1"}, 1000, 2000,
	)
	mem.SetVersionedResource(300, NewVersionedResource(rv3))

	// Before building index, LookupResourceAttr should return nil.
	require.Nil(t, mem.LookupResourceAttr("service.name", "payment-service"))

	// Configure host.name as an extra indexed descriptive attribute.
	mem.SetIndexedResourceAttrs(map[string]struct{}{"host.name": {}})
	mem.BuildResourceAttrIndex()

	// Identifying attribute lookup: service.name=payment-service matches series 100 and 200
	hashes := mem.LookupResourceAttr("service.name", "payment-service")
	require.Len(t, hashes, 2)
	require.Contains(t, hashes, uint64(100))
	require.Contains(t, hashes, uint64(200))

	// Identifying attribute lookup: service.namespace=production matches series 100 and 300
	hashes = mem.LookupResourceAttr("service.namespace", "production")
	require.Len(t, hashes, 2)
	require.Contains(t, hashes, uint64(100))
	require.Contains(t, hashes, uint64(300))

	// Descriptive attribute lookup: host.name=host-1 matches series 100 and 300
	hashes = mem.LookupResourceAttr("host.name", "host-1")
	require.Len(t, hashes, 2)
	require.Contains(t, hashes, uint64(100))
	require.Contains(t, hashes, uint64(300))

	// Non-existent attribute returns empty (not nil since index is built)
	hashes = mem.LookupResourceAttr("service.name", "nonexistent")
	require.Empty(t, hashes)
}

func TestBuildResourceAttrIndex_MultipleVersions(t *testing.T) {
	mem := NewMemSeriesMetadata()

	// Series with two versions: different descriptive attrs across versions.
	rv1 := NewResourceVersion(
		map[string]string{"service.name": "my-service"},
		map[string]string{"host.name": "host-old"}, 1000, 2000,
	)
	rv2 := NewResourceVersion(
		map[string]string{"service.name": "my-service"},
		map[string]string{"host.name": "host-new"}, 2001, 3000,
	)
	vr := NewVersionedResource(rv1)
	vr.Versions = append(vr.Versions, rv2)
	mem.SetVersionedResource(100, vr)

	// Configure host.name as an extra indexed descriptive attribute.
	mem.SetIndexedResourceAttrs(map[string]struct{}{"host.name": {}})
	mem.BuildResourceAttrIndex()

	// Both host.name values should be indexed.
	hashes := mem.LookupResourceAttr("host.name", "host-old")
	require.Len(t, hashes, 1)
	require.Contains(t, hashes, uint64(100))

	hashes = mem.LookupResourceAttr("host.name", "host-new")
	require.Len(t, hashes, 1)
	require.Contains(t, hashes, uint64(100))

	// Identifying attr present in both versions is indexed once.
	hashes = mem.LookupResourceAttr("service.name", "my-service")
	require.Len(t, hashes, 1)
	require.Contains(t, hashes, uint64(100))
}

func TestMemStoreContentDedup(t *testing.T) {
	store := NewMemResourceStore()

	sharedIdentifying := map[string]string{
		"service.name":      "my-service",
		"service.namespace": "production",
	}
	sharedDescriptive := map[string]string{
		"deployment.env": "prod",
		"host.region":    "us-west-2",
	}

	// Insert 1000 entries with identical resource content but different labelsHash.
	const numEntries = 1000
	for i := uint64(1); i <= numEntries; i++ {
		rv := NewResourceVersion(sharedIdentifying, sharedDescriptive, 1000, 5000)
		store.Set(i, rv)
	}

	// All entries should share a single canonical.
	require.Equal(t, 1, store.TotalCanonical())

	// Verify all entries share the same map pointers via reflect.
	var canonicalIdentifying, canonicalDescriptive uintptr
	for i := uint64(1); i <= numEntries; i++ {
		got, ok := store.Get(i)
		require.True(t, ok)
		idPtr := reflect.ValueOf(got.Identifying).Pointer()
		descPtr := reflect.ValueOf(got.Descriptive).Pointer()
		if i == 1 {
			canonicalIdentifying = idPtr
			canonicalDescriptive = descPtr
		} else {
			require.Equal(t, canonicalIdentifying, idPtr, "series %d should share Identifying map pointer", i)
			require.Equal(t, canonicalDescriptive, descPtr, "series %d should share Descriptive map pointer", i)
		}
		// Values are correct.
		require.Equal(t, "my-service", got.Identifying["service.name"])
		require.Equal(t, "prod", got.Descriptive["deployment.env"])
	}

	// Insert entries with different content → should create a second canonical.
	for i := uint64(numEntries + 1); i <= numEntries+10; i++ {
		rv := NewResourceVersion(
			map[string]string{"service.name": "other-service"},
			map[string]string{"deployment.env": "staging"}, 2000, 6000,
		)
		store.Set(i, rv)
	}
	require.Equal(t, 2, store.TotalCanonical())

	// The new entries should share a different canonical.
	got1, _ := store.Get(numEntries + 1)
	got2, _ := store.Get(numEntries + 5)
	require.Equal(t, reflect.ValueOf(got1.Identifying).Pointer(), reflect.ValueOf(got2.Identifying).Pointer())
	// But different from the first group.
	require.NotEqual(t, canonicalIdentifying, reflect.ValueOf(got1.Identifying).Pointer())
}

func TestMemStoreContentDedupSetVersioned(t *testing.T) {
	store := NewMemResourceStore()

	// SetVersioned should also intern.
	for i := uint64(1); i <= 50; i++ {
		rv := NewResourceVersion(
			map[string]string{"service.name": "svc"},
			map[string]string{"env": "prod"}, 1000, 5000,
		)
		store.SetVersioned(i, &VersionedResource{Versions: []*ResourceVersion{rv}})
	}
	require.Equal(t, 1, store.TotalCanonical())

	// All share the same pointer.
	var firstPtr uintptr
	for i := uint64(1); i <= 50; i++ {
		got, ok := store.Get(i)
		require.True(t, ok)
		ptr := reflect.ValueOf(got.Identifying).Pointer()
		if i == 1 {
			firstPtr = ptr
		} else {
			require.Equal(t, firstPtr, ptr)
		}
	}
}

func TestMemStoreContentDedupTimeRangesIndependent(t *testing.T) {
	store := NewMemResourceStore()

	// Two entries with same content but different time ranges should share maps
	// but have independent time ranges.
	rv1 := NewResourceVersion(map[string]string{"service.name": "svc"}, nil, 1000, 2000)
	store.Set(1, rv1)

	rv2 := NewResourceVersion(map[string]string{"service.name": "svc"}, nil, 3000, 4000)
	store.Set(2, rv2)

	got1, _ := store.Get(1)
	got2, _ := store.Get(2)

	// Same content pointers.
	require.Equal(t, reflect.ValueOf(got1.Identifying).Pointer(), reflect.ValueOf(got2.Identifying).Pointer())

	// But independent time ranges.
	require.Equal(t, int64(1000), got1.MinTime)
	require.Equal(t, int64(2000), got1.MaxTime)
	require.Equal(t, int64(3000), got2.MinTime)
	require.Equal(t, int64(4000), got2.MaxTime)
}

func TestInvertedIndexDisabled(t *testing.T) {
	mem := NewMemSeriesMetadata()
	// Do NOT call InitResourceAttrIndex — simulates EnableResourceAttrIndex=false.

	rv1 := NewResourceVersion(
		map[string]string{"service.name": "payment-service", "service.namespace": "production"},
		map[string]string{"host.name": "host-1"}, 1000, 2000,
	)
	vr := NewVersionedResource(rv1)
	mem.SetVersionedResource(100, vr)

	// Incremental update with no index initialized — should still track attr names.
	mem.UpdateResourceAttrIndex(100, nil, vr)

	// LookupResourceAttr returns nil (index not built).
	require.Nil(t, mem.LookupResourceAttr("service.name", "payment-service"))

	// UniqueResourceAttrNames still returns attr names (decoupled from index).
	names := mem.UniqueResourceAttrNames()
	require.NotNil(t, names)
	require.Contains(t, names, "service.name")
	require.Contains(t, names, "service.namespace")
	require.Contains(t, names, "host.name")

	// Now build the full index from scratch — verify it works.
	// BuildResourceAttrIndex creates and populates the index in one step.
	mem.BuildResourceAttrIndex()

	hashes := mem.LookupResourceAttr("service.name", "payment-service")
	require.Len(t, hashes, 1)
	require.Contains(t, hashes, uint64(100))
}

func TestPostingList(t *testing.T) {
	// Helper: add to inline, promoting via idx if needed.
	addToList := func(idx *shardedAttrIndex, pl postingList, v uint64) postingList {
		var needPromo bool
		pl, needPromo = pl.addInline(v)
		if needPromo {
			pl = pl.promote(idx.getOrAssignIDBulk)
		}
		return pl
	}

	t.Run("add to empty", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		pl = addToList(idx, pl, 42)
		require.False(t, pl.isEmpty())
		require.Nil(t, pl.bitmap)
		require.Equal(t, []uint64{42}, pl.toArray(nil))
	})

	t.Run("sorted insert", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		pl = addToList(idx, pl, 30)
		pl = addToList(idx, pl, 10)
		pl = addToList(idx, pl, 20)
		require.Equal(t, []uint64{10, 20, 30}, pl.toArray(nil))
	})

	t.Run("dedup", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		pl = addToList(idx, pl, 10)
		pl = addToList(idx, pl, 10)
		require.Equal(t, []uint64{10}, pl.toArray(nil))
	})

	t.Run("promotion at threshold", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		for i := uint64(0); i <= postingListInlineThreshold; i++ {
			pl = addToList(idx, pl, i)
		}
		require.NotNil(t, pl.bitmap, "should be promoted to bitmap")
		require.Nil(t, pl.inline, "inline should be nil after promotion")
		require.Equal(t, uint64(postingListInlineThreshold+1), pl.bitmap.GetCardinality())
	})

	t.Run("at threshold no promotion", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		for i := range uint64(postingListInlineThreshold) {
			pl = addToList(idx, pl, i)
		}
		require.Nil(t, pl.bitmap, "should stay inline at threshold")
		require.Len(t, pl.inline, postingListInlineThreshold)
	})

	t.Run("remove from inline", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		pl = addToList(idx, pl, 10)
		pl = addToList(idx, pl, 20)
		pl = addToList(idx, pl, 30)
		pl = pl.removeInline(20)
		require.Equal(t, []uint64{10, 30}, pl.toArray(nil))
	})

	t.Run("remove nonexistent", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		pl = addToList(idx, pl, 10)
		pl = pl.removeInline(99)
		require.Equal(t, []uint64{10}, pl.toArray(nil))
	})

	t.Run("remove from bitmap", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		for i := uint64(0); i <= postingListInlineThreshold; i++ {
			pl = addToList(idx, pl, i)
		}
		require.NotNil(t, pl.bitmap)
		id, ok := idx.lookupID(5)
		require.True(t, ok)
		pl = pl.removeBitmap(id)
		require.Equal(t, uint64(postingListInlineThreshold), pl.bitmap.GetCardinality())
	})

	t.Run("isEmpty", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		require.True(t, pl.isEmpty())
		pl = addToList(idx, pl, 1)
		require.False(t, pl.isEmpty())
		pl = pl.removeInline(1)
		require.True(t, pl.isEmpty())
	})

	t.Run("toArray returns copy", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		pl = addToList(idx, pl, 10)
		pl = addToList(idx, pl, 20)
		arr := pl.toArray(nil)
		arr[0] = 999
		require.Equal(t, []uint64{10, 20}, pl.toArray(nil), "modifying returned slice must not affect posting list")
	})

	t.Run("runOptimize inline noop", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		pl = addToList(idx, pl, 10)
		pl = pl.runOptimize()
		require.Equal(t, []uint64{10}, pl.toArray(nil))
	})

	t.Run("runOptimize bitmap", func(t *testing.T) {
		idx := newShardedAttrIndex()
		var pl postingList
		for i := uint64(0); i <= postingListInlineThreshold; i++ {
			pl = addToList(idx, pl, i)
		}
		pl = pl.runOptimize()
		require.NotNil(t, pl.bitmap)
		// Bitmap stores compact IDs — toArray with reverse mapping recovers original hashes.
		result := pl.toArray(idx.reverse)
		require.Len(t, result, postingListInlineThreshold+1)
	})
}
