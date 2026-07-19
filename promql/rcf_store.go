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

// RCF model store.
//
// Architecture
// ────────────
//   - rcfModelStore is an LRU cache of *rcfEntry keyed by series fingerprint.
//   - Persistence is delegated to a RCFStore implementation.
//     Use newDiskRCFStore for restart-safe persistence, or newMemoryRCFStore
//     (the default) for in-process-only storage.
//   - Concurrency: the store-level mutex protects the LRU; each model has its
//     own mutex so concurrent PromQL evaluations on different series do not
//     block each other.

import (
	"container/list"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ── persistence interface ─────────────────────────────────────────────────────

// RCFStore is the persistence back-end for RCF models.
// Implementations must be safe for concurrent use.
type RCFStore interface {
	// Load returns the persisted forest for the given fingerprint, or nil if
	// none exists or the stored model is incompatible with numTrees/sampleSize.
	Load(fingerprint uint64, numTrees, sampleSize int) *rcfForest
	// Save persists the forest for the given fingerprint.
	Save(fingerprint uint64, forest *rcfForest)
	// Delete removes any persisted model for the given fingerprint.
	Delete(fingerprint uint64)
}

// ── memory-only store ─────────────────────────────────────────────────────────

type memoryRCFStore struct{}

func newMemoryRCFStore() RCFStore                          { return memoryRCFStore{} }
func (memoryRCFStore) Load(uint64, int, int) *rcfForest    { return nil }
func (memoryRCFStore) Save(uint64, *rcfForest)             {}
func (memoryRCFStore) Delete(uint64)                       {}

// ── disk store ────────────────────────────────────────────────────────────────

type diskRCFStore struct{ dir string }

func NewDiskRCFStore(dir string) RCFStore { return &diskRCFStore{dir: dir} }

func (s *diskRCFStore) path(fingerprint uint64) string {
	return filepath.Join(s.dir, fmt.Sprintf("%016x.rcf", fingerprint))
}

func (s *diskRCFStore) Load(fingerprint uint64, numTrees, sampleSize int) *rcfForest {
	f, err := os.Open(s.path(fingerprint))
	if err != nil {
		return nil
	}
	defer f.Close()
	var snap rcfDiskModel
	if err := gob.NewDecoder(f).Decode(&snap); err != nil {
		return nil
	}
	forest := forestFromSnapshot(&snap)
	if forest.NumTrees != numTrees || forest.SampleSize != sampleSize {
		return nil
	}
	return forest
}

func (s *diskRCFStore) Save(fingerprint uint64, forest *rcfForest) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return
	}
	snap := forestToSnapshot(forest)
	tmp := s.path(fingerprint) + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	if err := gob.NewEncoder(f).Encode(snap); err != nil {
		f.Close()
		os.Remove(tmp)
		return
	}
	f.Close()
	// Atomic rename for restart-safety.
	_ = os.Rename(tmp, s.path(fingerprint))
}

func (s *diskRCFStore) Delete(fingerprint uint64) {
	_ = os.Remove(s.path(fingerprint))
}

// ── model entry ───────────────────────────────────────────────────────────────

// rcfEntry is one cached model.
type rcfEntry struct {
	mu          sync.Mutex
	forest      *rcfForest
	fingerprint uint64
	dirty       bool // needs checkpoint
	elem        *list.Element
}

// rcfDiskModel is the gob-serialisable snapshot of a model.
type rcfDiskModel struct {
	NumTrees   int
	SampleSize int
	LastTS     int64
	Reservoir  rcfReservoir
	Trees      []rcfTreeSnapshot
}

// rcfTreeSnapshot is the gob-serialisable form of one tree.
// We flatten the tree into a slice of nodes using pre-order traversal.
type rcfTreeSnapshot struct {
	Rng   uint64
	Nodes []rcfNodeSnapshot
}

type rcfNodeSnapshot struct {
	Box    rcfBox
	Mass   int
	CutDim int
	CutVal float64
	IsLeaf bool
	Point  [rcfDims]float64
	Count  int
	// child indices into the Nodes slice; -1 = nil
	LeftIdx  int
	RightIdx int
}

// ── store ─────────────────────────────────────────────────────────────────────

type rcfModelStore struct {
	mu        sync.Mutex
	cache     map[uint64]*rcfEntry
	lru       *list.List // front = most recently used
	backend   RCFStore
	cacheSize int
}

func newRCFModelStore(backend RCFStore, cacheSize int) *rcfModelStore {
	if backend == nil {
		backend = newMemoryRCFStore()
	}
	if cacheSize <= 0 {
		cacheSize = 1024
	}
	return &rcfModelStore{
		cache:     make(map[uint64]*rcfEntry),
		lru:       list.New(),
		backend:   backend,
		cacheSize: cacheSize,
	}
}

// forest returns the in-memory entry for fingerprint, loading from the backend
// or creating a new forest if necessary. The caller must hold entry.mu while
// using entry.forest.
func (s *rcfModelStore) forest(fingerprint uint64, numTrees, sampleSize int) *rcfEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.cache[fingerprint]; ok {
		s.lru.MoveToFront(e.elem)
		// Rebuild if config changed.
		e.mu.Lock()
		if e.forest.NumTrees != numTrees || e.forest.SampleSize != sampleSize {
			e.forest = newRCFForest(numTrees, sampleSize, rcfSeedFromString("")^fingerprint)
			e.dirty = true
		}
		e.mu.Unlock()
		return e
	}

	// Not in cache: try backend, then create fresh.
	e := &rcfEntry{fingerprint: fingerprint}
	if f := s.backend.Load(fingerprint, numTrees, sampleSize); f != nil {
		e.forest = f
	} else {
		e.forest = newRCFForest(numTrees, sampleSize, rcfSeedFromString("")^fingerprint)
	}

	e.elem = s.lru.PushFront(e)
	s.cache[fingerprint] = e
	s.evictIfNeeded()
	return e
}

// markDirty flags a model for checkpointing on the next eviction or explicit flush.
func (s *rcfModelStore) markDirty(fingerprint uint64) {
	s.mu.Lock()
	if e, ok := s.cache[fingerprint]; ok {
		e.dirty = true
	}
	s.mu.Unlock()
}

// Checkpoint writes all dirty models to the backend.
func (s *rcfModelStore) Checkpoint() {
	s.mu.Lock()
	dirty := make([]*rcfEntry, 0)
	for _, e := range s.cache {
		if e.dirty {
			dirty = append(dirty, e)
		}
	}
	s.mu.Unlock()
	for _, e := range dirty {
		e.mu.Lock()
		s.backend.Save(e.fingerprint, e.forest)
		e.dirty = false
		e.mu.Unlock()
	}
}

// Delete removes a model from memory and the backend.
func (s *rcfModelStore) Delete(fingerprint uint64) {
	s.mu.Lock()
	if e, ok := s.cache[fingerprint]; ok {
		s.lru.Remove(e.elem)
		delete(s.cache, fingerprint)
	}
	s.mu.Unlock()
	s.backend.Delete(fingerprint)
}

// evictIfNeeded removes the least-recently-used model when the cache is full.
// Must be called with s.mu held.
func (s *rcfModelStore) evictIfNeeded() {
	for s.lru.Len() > s.cacheSize {
		back := s.lru.Back()
		if back == nil {
			break
		}
		e := back.Value.(*rcfEntry)
		s.lru.Remove(back)
		delete(s.cache, e.fingerprint)
		if e.dirty {
			e.mu.Lock()
			s.backend.Save(e.fingerprint, e.forest)
			e.mu.Unlock()
		}
	}
}

// ── snapshot serialisation ────────────────────────────────────────────────────

func forestToSnapshot(f *rcfForest) rcfDiskModel {
	snap := rcfDiskModel{
		NumTrees:   f.NumTrees,
		SampleSize: f.SampleSize,
		LastTS:     f.LastTS,
		Reservoir:  *f.Reservoir,
		Trees:      make([]rcfTreeSnapshot, len(f.Trees)),
	}
	for i, t := range f.Trees {
		snap.Trees[i] = treeToSnapshot(t)
	}
	return snap
}

func treeToSnapshot(t *rcfTree) rcfTreeSnapshot {
	snap := rcfTreeSnapshot{Rng: t.Rng}
	if t.Root == nil {
		return snap
	}
	// Pre-order traversal; record child indices.
	type frame struct {
		node      *rcfNode
		selfIdx   int
		parentIdx int
		isLeft    bool
	}
	stack := []frame{{node: t.Root, selfIdx: 0, parentIdx: -1}}
	snap.Nodes = make([]rcfNodeSnapshot, 0, 64)
	for len(stack) > 0 {
		fr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := fr.node
		idx := len(snap.Nodes)
		ns := rcfNodeSnapshot{
			Box:      n.Box,
			Mass:     n.Mass,
			CutDim:   n.CutDim,
			CutVal:   n.CutVal,
			IsLeaf:   n.isLeaf(),
			Point:    n.Point,
			Count:    n.Count,
			LeftIdx:  -1,
			RightIdx: -1,
		}
		snap.Nodes = append(snap.Nodes, ns)
		// Patch parent's child pointer.
		if fr.parentIdx >= 0 {
			if fr.isLeft {
				snap.Nodes[fr.parentIdx].LeftIdx = idx
			} else {
				snap.Nodes[fr.parentIdx].RightIdx = idx
			}
		}
		if !n.isLeaf() {
			if n.Right != nil {
				stack = append(stack, frame{node: n.Right, selfIdx: idx + 1, parentIdx: idx, isLeft: false})
			}
			if n.Left != nil {
				stack = append(stack, frame{node: n.Left, selfIdx: idx + 1, parentIdx: idx, isLeft: true})
			}
		}
	}
	return snap
}

func forestFromSnapshot(snap *rcfDiskModel) *rcfForest {
	res := snap.Reservoir
	f := &rcfForest{
		NumTrees:   snap.NumTrees,
		SampleSize: snap.SampleSize,
		LastTS:     snap.LastTS,
		Reservoir:  &res,
		Trees:      make([]*rcfTree, len(snap.Trees)),
	}
	for i, ts := range snap.Trees {
		f.Trees[i] = treeFromSnapshot(&ts)
	}
	return f
}

func treeFromSnapshot(snap *rcfTreeSnapshot) *rcfTree {
	t := &rcfTree{Rng: snap.Rng}
	if len(snap.Nodes) == 0 {
		return t
	}
	nodes := make([]*rcfNode, len(snap.Nodes))
	for i, ns := range snap.Nodes {
		n := &rcfNode{
			Box:    ns.Box,
			Mass:   ns.Mass,
			CutDim: ns.CutDim,
			CutVal: ns.CutVal,
			Point:  ns.Point,
			Count:  ns.Count,
		}
		nodes[i] = n
	}
	for i, ns := range snap.Nodes {
		if ns.LeftIdx >= 0 {
			nodes[i].Left = nodes[ns.LeftIdx]
			nodes[ns.LeftIdx].Parent = nodes[i]
		}
		if ns.RightIdx >= 0 {
			nodes[i].Right = nodes[ns.RightIdx]
			nodes[ns.RightIdx].Parent = nodes[i]
		}
	}
	t.Root = nodes[0]
	return t
}
