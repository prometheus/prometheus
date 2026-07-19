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

package promql

import (
	"container/list"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// ── rcfBox ────────────────────────────────────────────────────────────────────

func TestRCFBoxExtendAndContains(t *testing.T) {
	p := [rcfDims]float64{1, 2, 3, 4, 5, 6}
	b := boxFromPoint(p)
	require.True(t, b.contains(p))

	q := [rcfDims]float64{0, 3, 2, 5, 4, 7}
	b.extendPoint(q)
	require.True(t, b.contains(p))
	require.True(t, b.contains(q))

	outside := [rcfDims]float64{-1, 0, 0, 0, 0, 0}
	require.False(t, b.contains(outside))
}

func TestRCFBoxSpan(t *testing.T) {
	var b rcfBox
	for d := range rcfDims {
		b.Lo[d] = 0
		b.Hi[d] = float64(d + 1)
	}
	// span = 1+2+3+4+5+6 = 21
	require.InDelta(t, 21.0, b.span(), 1e-9)
}

// ── rcfTree ───────────────────────────────────────────────────────────────────

func TestRCFTreeInsertAndScore(t *testing.T) {
	tree := newRCFTree(42)

	var p [rcfDims]float64
	require.Equal(t, 0.0, tree.score(p))

	for i := range 10 {
		var pt [rcfDims]float64
		pt[0] = float64(i)
		tree.insert(pt)
	}
	require.Equal(t, 10, tree.Root.Mass)

	var outlier [rcfDims]float64
	outlier[0] = 1000
	require.Greater(t, tree.score(outlier), 0.0)
}

func TestRCFTreeDeleteReducesMass(t *testing.T) {
	tree := newRCFTree(7)
	var p [rcfDims]float64
	p[0] = 1.0
	tree.insert(p)
	tree.insert(p)
	require.Equal(t, 2, tree.Root.Mass)

	tree.delete(p)
	require.Equal(t, 1, tree.Root.Mass)

	tree.delete(p)
	require.Nil(t, tree.Root)
}

func TestRCFTreeDeleteNonExistent(t *testing.T) {
	tree := newRCFTree(3)
	var p [rcfDims]float64
	p[0] = 5.0
	tree.insert(p)

	var q [rcfDims]float64
	q[0] = 99.0
	tree.delete(q)
	require.Equal(t, 1, tree.Root.Mass)
}

// ── rcfReservoir ─────────────────────────────────────────────────────────────

func TestRCFReservoirFillAndEvict(t *testing.T) {
	r := newReservoir(4, 1)
	for i := range 4 {
		var p [rcfDims]float64
		p[0] = float64(i)
		_, didEvict := r.add(int64(i), p)
		require.False(t, didEvict)
	}
	require.Len(t, r.Samples, 4)

	var p [rcfDims]float64
	p[0] = 99
	_, didEvict := r.add(4, p)
	require.True(t, didEvict)
	require.Len(t, r.Samples, 4)
}

// ── rcfForest ─────────────────────────────────────────────────────────────────

func TestRCFForestScoreRange(t *testing.T) {
	f := newRCFForest(10, 32, 123)
	for i := range 40 {
		var p [rcfDims]float64
		p[0] = float64(i % 5)
		f.update(int64(i*1000), p)
	}

	var normal [rcfDims]float64
	normal[0] = 2.0
	s := f.score(normal)
	require.GreaterOrEqual(t, s, 0.0)
	require.LessOrEqual(t, s, 1.0)

	var outlier [rcfDims]float64
	outlier[0] = 1e6
	sOut := f.score(outlier)
	require.GreaterOrEqual(t, sOut, 0.0)
	require.LessOrEqual(t, sOut, 1.0)
}

func TestRCFForestAttributionNonNegative(t *testing.T) {
	f := newRCFForest(5, 16, 77)
	for i := range 20 {
		var p [rcfDims]float64
		p[0] = float64(i)
		f.update(int64(i*1000), p)
	}

	var pt [rcfDims]float64
	pt[0] = 500
	attr := f.attribution(pt)
	for _, v := range attr {
		require.GreaterOrEqual(t, v, 0.0)
	}
}

// ── rcfModelStore ─────────────────────────────────────────────────────────────

func TestRCFModelStoreMemoryOnly(t *testing.T) {
	store := &rcfModelStore{
		cache: make(map[uint64]*rcfEntry),
		lru:   list.New(),
	}

	e1 := store.forest(1, 10, 32)
	require.NotNil(t, e1.forest)

	e2 := store.forest(1, 10, 32)
	require.Equal(t, e1, e2)

	store.markDirty(1)
	store.mu.Lock()
	require.True(t, store.cache[1].dirty)
	store.mu.Unlock()

	store.Delete(1)
	store.mu.Lock()
	_, ok := store.cache[1]
	store.mu.Unlock()
	require.False(t, ok)
}

func TestRCFModelStoreLRUEviction(t *testing.T) {
	store := &rcfModelStore{
		cache: make(map[uint64]*rcfEntry),
		lru:   list.New(),
	}
	origSize := RCFStoreCacheSize
	RCFStoreCacheSize = 2
	t.Cleanup(func() { RCFStoreCacheSize = origSize })

	store.forest(1, 5, 16)
	store.forest(2, 5, 16)
	store.forest(3, 5, 16)

	store.mu.Lock()
	_, has1 := store.cache[1]
	_, has2 := store.cache[2]
	_, has3 := store.cache[3]
	store.mu.Unlock()

	require.False(t, has1, "fingerprint 1 should have been evicted")
	require.True(t, has2)
	require.True(t, has3)
}

func TestRCFStoreCacheSizeEnvVar(t *testing.T) {
	// Verify that RCFStoreCacheSize is respected by eviction: setting it to 1
	// and inserting 2 models should evict the first.
	store := &rcfModelStore{
		cache: make(map[uint64]*rcfEntry),
		lru:   list.New(),
	}
	orig := RCFStoreCacheSize
	RCFStoreCacheSize = 1
	t.Cleanup(func() { RCFStoreCacheSize = orig })

	store.forest(1, 5, 16)
	store.forest(2, 5, 16)

	store.mu.Lock()
	_, has1 := store.cache[1]
	_, has2 := store.cache[2]
	store.mu.Unlock()

	require.False(t, has1, "fingerprint 1 should have been evicted")
	require.True(t, has2)
}

func TestRCFModelStoreDiskRoundtrip(t *testing.T) {
	dir := t.TempDir()

	origPath := RCFStorePath
	RCFStorePath = dir
	t.Cleanup(func() { RCFStorePath = origPath })

	store := &rcfModelStore{
		cache: make(map[uint64]*rcfEntry),
		lru:   list.New(),
	}

	fp := uint64(0xdeadbeef)
	e := store.forest(fp, 5, 16)
	e.mu.Lock()
	for i := range 20 {
		var p [rcfDims]float64
		p[0] = float64(i)
		e.forest.update(int64(i*1000), p)
	}
	wantLastTS := e.forest.LastTS
	e.mu.Unlock()
	store.markDirty(fp)
	store.Checkpoint()

	_, err := os.Stat(store.diskPath(fp))
	require.NoError(t, err)

	store2 := &rcfModelStore{
		cache: make(map[uint64]*rcfEntry),
		lru:   list.New(),
	}
	e2 := store2.forest(fp, 5, 16)
	require.Equal(t, wantLastTS, e2.forest.LastTS)
}

// ── snapshot round-trip ───────────────────────────────────────────────────────

func TestRCFSnapshotRoundtrip(t *testing.T) {
	f := newRCFForest(3, 16, 99)
	for i := range 20 {
		var p [rcfDims]float64
		p[0] = float64(i)
		p[1] = float64(i * 2)
		f.update(int64(i*1000), p)
	}

	snap := forestToSnapshot(f)
	f2 := forestFromSnapshot(&snap)

	require.Equal(t, f.NumTrees, f2.NumTrees)
	require.Equal(t, f.SampleSize, f2.SampleSize)
	require.Equal(t, f.LastTS, f2.LastTS)
	require.Len(t, f2.Trees, f.NumTrees)

	var pt [rcfDims]float64
	pt[0] = 500
	require.InDelta(t, f.score(pt), f2.score(pt), 1e-9)
}
