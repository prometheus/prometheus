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

	"github.com/stretchr/testify/require"
)

func TestScopeVersionEquality(t *testing.T) {
	a := NewScopeVersion("mylib", "1.0.0", "https://schema.example.com", map[string]string{"key": "val"}, 1000, 2000)
	b := NewScopeVersion("mylib", "1.0.0", "https://schema.example.com", map[string]string{"key": "val"}, 3000, 4000)
	require.True(t, ScopeVersionsEqual(a, b))

	// Different name
	c := NewScopeVersion("otherlib", "1.0.0", "https://schema.example.com", map[string]string{"key": "val"}, 1000, 2000)
	require.False(t, ScopeVersionsEqual(a, c))

	// Different version
	d := NewScopeVersion("mylib", "2.0.0", "https://schema.example.com", map[string]string{"key": "val"}, 1000, 2000)
	require.False(t, ScopeVersionsEqual(a, d))

	// Different schema URL
	e := NewScopeVersion("mylib", "1.0.0", "https://other.example.com", map[string]string{"key": "val"}, 1000, 2000)
	require.False(t, ScopeVersionsEqual(a, e))

	// Different attrs
	f := NewScopeVersion("mylib", "1.0.0", "https://schema.example.com", map[string]string{"key": "other"}, 1000, 2000)
	require.False(t, ScopeVersionsEqual(a, f))
}

func TestScopeVersionDeepCopy(t *testing.T) {
	original := NewScopeVersion("mylib", "1.0.0", "", map[string]string{"k": "v"}, 1000, 2000)
	copied := CopyScopeVersion(original)

	// Modify original attrs
	original.Attrs["k"] = "modified"
	require.Equal(t, "v", copied.Attrs["k"])
}

func TestVersionedScopeAddOrExtend(t *testing.T) {
	vs := &VersionedScope{}

	sv1 := NewScopeVersion("mylib", "1.0.0", "", map[string]string{"k": "v"}, 1000, 2000)
	vs.AddOrExtend(sv1)
	require.Len(t, vs.Versions, 1)
	require.Equal(t, int64(1000), vs.Versions[0].MinTime)
	require.Equal(t, int64(2000), vs.Versions[0].MaxTime)

	// Same scope, later time — should extend
	sv2 := NewScopeVersion("mylib", "1.0.0", "", map[string]string{"k": "v"}, 3000, 4000)
	vs.AddOrExtend(sv2)
	require.Len(t, vs.Versions, 1)
	require.Equal(t, int64(1000), vs.Versions[0].MinTime)
	require.Equal(t, int64(4000), vs.Versions[0].MaxTime)

	// Different scope — should add new version
	sv3 := NewScopeVersion("mylib", "2.0.0", "", map[string]string{"k": "v"}, 5000, 6000)
	vs.AddOrExtend(sv3)
	require.Len(t, vs.Versions, 2)
	require.Equal(t, "2.0.0", vs.Versions[1].Version)
}

func TestVersionedScopeVersionAt(t *testing.T) {
	vs := &VersionedScope{
		Versions: []*ScopeVersion{
			{Name: "lib", Version: "1.0", MinTime: 1000, MaxTime: 2000},
			{Name: "lib", Version: "2.0", MinTime: 3000, MaxTime: 4000},
		},
	}

	// Within first version
	v := vs.VersionAt(1500)
	require.Equal(t, "1.0", v.Version)

	// Within second version
	v = vs.VersionAt(3500)
	require.Equal(t, "2.0", v.Version)

	// Between versions — returns closest earlier
	v = vs.VersionAt(2500)
	require.Equal(t, "1.0", v.Version)

	// Before all versions — no version covers this timestamp
	v = vs.VersionAt(500)
	require.Nil(t, v)

	// After all versions
	v = vs.VersionAt(5000)
	require.Equal(t, "2.0", v.Version)
}

func TestMergeVersionedScopes(t *testing.T) {
	a := &VersionedScope{
		Versions: []*ScopeVersion{
			{Name: "lib", Version: "1.0", MinTime: 1000, MaxTime: 2000},
		},
	}
	b := &VersionedScope{
		Versions: []*ScopeVersion{
			{Name: "lib", Version: "1.0", MinTime: 2001, MaxTime: 3000},
		},
	}

	merged := MergeVersionedScopes(a, b)
	// Adjacent (within +1) with same content should merge
	require.Len(t, merged.Versions, 1)
	require.Equal(t, int64(1000), merged.Versions[0].MinTime)
	require.Equal(t, int64(3000), merged.Versions[0].MaxTime)

	// Nil cases
	require.Equal(t, b, MergeVersionedScopes(nil, b))
	require.Equal(t, a, MergeVersionedScopes(a, nil))
}

func TestMergeVersionedScopesDifferentVersions(t *testing.T) {
	a := &VersionedScope{
		Versions: []*ScopeVersion{
			{Name: "lib", Version: "1.0", MinTime: 1000, MaxTime: 2000},
		},
	}
	b := &VersionedScope{
		Versions: []*ScopeVersion{
			{Name: "lib", Version: "2.0", MinTime: 3000, MaxTime: 4000},
		},
	}

	merged := MergeVersionedScopes(a, b)
	require.Len(t, merged.Versions, 2)
	require.Equal(t, "1.0", merged.Versions[0].Version)
	require.Equal(t, "2.0", merged.Versions[1].Version)
}

func TestMemScopeStoreBasicOperations(t *testing.T) {
	store := NewMemScopeStore()

	sv := NewScopeVersion("mylib", "1.0.0", "https://schema.example.com", map[string]string{"key": "val"}, 1000, 2000)
	store.SetScope(123, sv)

	got, found := store.GetVersionedScope(123)
	require.True(t, found)
	require.Len(t, got.Versions, 1)
	require.Equal(t, "mylib", got.Versions[0].Name)
	require.Equal(t, "1.0.0", got.Versions[0].Version)

	require.Equal(t, uint64(1), store.TotalScopes())
	require.Equal(t, uint64(1), store.TotalScopeVersions())

	// Not found
	_, found = store.GetVersionedScope(999)
	require.False(t, found)

	// Delete
	store.DeleteScope(123)
	_, found = store.GetVersionedScope(123)
	require.False(t, found)
}

func TestMemScopeStoreVersioning(t *testing.T) {
	store := NewMemScopeStore()

	sv1 := NewScopeVersion("lib", "1.0", "", nil, 1000, 2000)
	store.SetScope(100, sv1)

	// Same scope — extend
	sv2 := NewScopeVersion("lib", "1.0", "", nil, 3000, 4000)
	store.SetScope(100, sv2)

	got, found := store.GetVersionedScope(100)
	require.True(t, found)
	require.Len(t, got.Versions, 1)
	require.Equal(t, int64(1000), got.Versions[0].MinTime)
	require.Equal(t, int64(4000), got.Versions[0].MaxTime)

	// Different scope — new version
	sv3 := NewScopeVersion("lib", "2.0", "", nil, 5000, 6000)
	store.SetScope(100, sv3)

	got, found = store.GetVersionedScope(100)
	require.True(t, found)
	require.Len(t, got.Versions, 2)
	require.Equal(t, uint64(2), store.TotalScopeVersions())
}

func TestMemScopeStoreIter(t *testing.T) {
	store := NewMemScopeStore()

	for i := uint64(1); i <= 3; i++ {
		sv := NewScopeVersion("lib", "1.0", "", nil, int64(i*1000), int64(i*2000))
		store.SetScope(i, sv)
	}

	collected := make(map[uint64]*VersionedScope)
	err := store.IterVersionedScopes(func(labelsHash uint64, scopes *VersionedScope) error {
		collected[labelsHash] = scopes
		return nil
	})
	require.NoError(t, err)
	require.Len(t, collected, 3)
}
