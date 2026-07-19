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
//   - rcfModelStore is the process-global singleton (rcfModels).
//   - In-memory: an LRU cache of *rcfEntry keyed by series fingerprint.
//     Capacity is RCFStoreCacheSize (default 1024 models).
//   - On-disk: one gob file per model under RCFStorePath/<fingerprint>.rcf.
//     Empty RCFStorePath disables persistence (memory-only mode).
//   - Lifecycle: create → warm-up (ingest) → checkpoint → restore → prune/delete.
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

// ── configuration ─────────────────────────────────────────────────────────────

// RCFStorePath is the directory used for on-disk model persistence.
// Set to a non-empty path before the first rcf() evaluation to enable
// restart-safe persistence. An empty string disables disk I/O.
var RCFStorePath = ""

// RCFStoreCacheSize is the maximum number of models kept in memory at once.
// Models evicted from the LRU are checkpointed to disk if RCFStorePath is set.
var RCFStoreCacheSize = 1024

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
	mu    sync.Mutex
	cache map[uint64]*rcfEntry
	lru   *list.List // front = most recently used
}

// rcfModels is the process-global model store.
var rcfModels = &rcfModelStore{
	cache: make(map[uint64]*rcfEntry),
	lru:   list.New(),
}

// forest returns the in-memory entry for fingerprint, loading from disk or
// creating a new forest if necessary. The caller must hold entry.mu while
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

	// Not in cache: try disk, then create fresh.
	e := &rcfEntry{fingerprint: fingerprint}
	if f := s.loadFromDisk(fingerprint); f != nil && f.NumTrees == numTrees && f.SampleSize == sampleSize {
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

// Checkpoint writes all dirty models to disk.
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
		s.saveToDisk(e)
		e.dirty = false
		e.mu.Unlock()
	}
}

// Delete removes a model from memory and disk.
func (s *rcfModelStore) Delete(fingerprint uint64) {
	s.mu.Lock()
	if e, ok := s.cache[fingerprint]; ok {
		s.lru.Remove(e.elem)
		delete(s.cache, fingerprint)
	}
	s.mu.Unlock()
	if RCFStorePath != "" {
		_ = os.Remove(rcfDiskPath(fingerprint))
	}
}

// evictIfNeeded removes the least-recently-used model when the cache is full.
// Must be called with s.mu held.
func (s *rcfModelStore) evictIfNeeded() {
	for s.lru.Len() > RCFStoreCacheSize {
		back := s.lru.Back()
		if back == nil {
			break
		}
		e := back.Value.(*rcfEntry)
		s.lru.Remove(back)
		delete(s.cache, e.fingerprint)
		if e.dirty {
			e.mu.Lock()
			s.saveToDisk(e)
			e.mu.Unlock()
		}
	}
}

// ── disk I/O ──────────────────────────────────────────────────────────────────

func rcfDiskPath(fingerprint uint64) string {
	return filepath.Join(RCFStorePath, fmt.Sprintf("%016x.rcf", fingerprint))
}

func (s *rcfModelStore) loadFromDisk(fingerprint uint64) *rcfForest {
	if RCFStorePath == "" {
		return nil
	}
	f, err := os.Open(rcfDiskPath(fingerprint))
	if err != nil {
		return nil
	}
	defer f.Close()
	var snap rcfDiskModel
	if err := gob.NewDecoder(f).Decode(&snap); err != nil {
		return nil
	}
	return forestFromSnapshot(&snap)
}

func (s *rcfModelStore) saveToDisk(e *rcfEntry) {
	if RCFStorePath == "" {
		return
	}
	if err := os.MkdirAll(RCFStorePath, 0o755); err != nil {
		return
	}
	snap := forestToSnapshot(e.forest)
	tmp := rcfDiskPath(e.fingerprint) + ".tmp"
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
	_ = os.Rename(tmp, rcfDiskPath(e.fingerprint))
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
