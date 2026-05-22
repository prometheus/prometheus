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
	"slices"
	"sync"

	"github.com/RoaringBitmap/roaring"
	"github.com/cespare/xxhash/v2"

	"github.com/prometheus/prometheus/model/labels"
)

// Reader provides read access to series metadata (OTel resources).
type Reader interface {
	// Close releases any resources associated with the reader.
	Close() error

	// VersionedResourceReader provides access to versioned OTel resources.
	VersionedResourceReader

	// IterKind iterates all entries for a kind (type-erased).
	IterKind(ctx context.Context, id KindID, f func(labelsHash uint64, versioned any) error) error

	// IterHashes iterates all labelsHashes for a kind without materializing
	// versions. Cheaper than IterKind when only the hash set is needed.
	IterHashes(ctx context.Context, id KindID, f func(labelsHash uint64) error) error

	// KindLen returns the number of entries for a kind.
	KindLen(id KindID) int

	// LabelsForHash returns the labels for a given labels hash, if available.
	LabelsForHash(labelsHash uint64) (labels.Labels, bool)

	// LookupResourceAttr returns sorted labelsHashes that have a resource version
	// with the given key:value in Identifying or Descriptive attributes.
	// Returns nil if the index has not been built. The returned slice must not
	// be modified by the caller.
	LookupResourceAttr(key, value string) []uint64
}

// UniqueAttrNameReader is optionally implemented by Reader implementations
// that maintain a cached set of unique resource attribute names. Checking
// this via type assertion avoids O(N_series) full scans.
type UniqueAttrNameReader interface {
	UniqueResourceAttrNames() map[string]struct{}
}

// numAttrIndexStripes is the number of shards in the inverted attribute index.
// Must be a power of two for fast modulo via bitmask.
const numAttrIndexStripes = 256

// postingListInlineThreshold is the maximum number of entries stored in a
// sorted []uint64 before promoting to a *roaring.Bitmap. 128 entries use
// 1024 bytes inline vs ~18000 bytes for a roaring bitmap with sparse random
// uint64-derived keys. Binary search insert is still fast at this size.
const postingListInlineThreshold = 128

// postingList is a hybrid posting list: small sets (≤128 entries) are stored
// as a sorted []uint64 inline, larger sets are promoted to *roaring.Bitmap
// using compact uint32 IDs assigned by the owning shardedAttrIndex.
// Stored by value in the map — mutations use read-modify-write.
type postingList struct {
	inline []uint64        // sorted labelsHashes; used when bitmap==nil
	bitmap *roaring.Bitmap // 32-bit compact IDs; non-nil when promoted past threshold
}

// addInline inserts v into the inline slice. Returns the updated postingList
// and whether promotion is needed (len exceeded threshold).
// Must only be called when bitmap==nil.
func (p postingList) addInline(v uint64) (postingList, bool) {
	i, found := slices.BinarySearch(p.inline, v)
	if found {
		return p, false
	}
	p.inline = slices.Insert(p.inline, i, v)
	return p, len(p.inline) > postingListInlineThreshold
}

// promote converts the inline slice to a roaring bitmap using compact IDs.
func (p postingList) promote(getID func(uint64) uint32) postingList {
	bm := roaring.New()
	for _, h := range p.inline {
		bm.Add(getID(h))
	}
	return postingList{bitmap: bm}
}

// removeInline removes v from the inline slice.
func (p postingList) removeInline(v uint64) postingList {
	i, found := slices.BinarySearch(p.inline, v)
	if found {
		p.inline = slices.Delete(p.inline, i, i+1)
	}
	return p
}

// removeBitmap removes the compact ID for v from the bitmap.
func (p postingList) removeBitmap(id uint32) postingList {
	if p.bitmap != nil {
		p.bitmap.Remove(id)
	}
	return p
}

func (p postingList) isEmpty() bool {
	return len(p.inline) == 0 && (p.bitmap == nil || p.bitmap.IsEmpty())
}

// toArray returns a copy of the posting list as a sorted []uint64.
// For bitmap posting lists, compact IDs are translated back via reverse.
// The returned slice is owned by the caller (safe after releasing locks).
func (p postingList) toArray(reverse []uint64) []uint64 {
	if p.bitmap != nil {
		compactIDs := p.bitmap.ToArray()
		result := make([]uint64, len(compactIDs))
		for i, id := range compactIDs {
			result[i] = reverse[id]
		}
		return result
	}
	return slices.Clone(p.inline)
}

func (p postingList) runOptimize() postingList {
	if p.bitmap != nil {
		p.bitmap.RunOptimize()
	}
	return p
}

// attrIndexStripe is a single shard of the inverted attribute index.
// Values are postingList: small sets use sorted inline slices,
// large sets use roaring bitmaps.
type attrIndexStripe struct {
	mtx sync.RWMutex
	idx map[string]postingList
	_   [40]byte // cache-line padding to prevent false sharing
}

// shardedAttrIndex is a 256-way sharded inverted index mapping
// "key\x00value" → sorted []uint64 of labelsHashes. Sharding by key hash
// eliminates the single-mutex bottleneck under high ingestion concurrency.
//
// Compact ID mapping: when a posting list is promoted from inline to bitmap,
// labelsHashes (uint64) are mapped to dense sequential uint32 compact IDs.
// Dense IDs share roaring containers efficiently (one bitmap container covers
// 65536 entries in 8 KB), dramatically reducing per-entry overhead.
type shardedAttrIndex struct {
	stripes [numAttrIndexStripes]attrIndexStripe

	// Compact ID mapping: labelsHash (uint64) ↔ dense uint32 ID for 32-bit roaring.
	idMu    sync.RWMutex
	forward map[uint64]uint32 // labelsHash → compactID
	reverse []uint64          // compactID → labelsHash (append-only)
}

func newShardedAttrIndex() *shardedAttrIndex {
	s := &shardedAttrIndex{
		forward: make(map[uint64]uint32),
	}
	for i := range s.stripes {
		s.stripes[i].idx = make(map[string]postingList)
	}
	return s
}

// getOrAssignID returns the compact uint32 ID for a labelsHash, assigning
// a new one if not yet mapped. Thread-safe.
func (s *shardedAttrIndex) getOrAssignID(labelsHash uint64) uint32 {
	s.idMu.RLock()
	if id, ok := s.forward[labelsHash]; ok {
		s.idMu.RUnlock()
		return id
	}
	s.idMu.RUnlock()

	s.idMu.Lock()
	defer s.idMu.Unlock()
	// Double-check after acquiring write lock.
	if id, ok := s.forward[labelsHash]; ok {
		return id
	}
	id := uint32(len(s.reverse))
	s.forward[labelsHash] = id
	s.reverse = append(s.reverse, labelsHash)
	return id
}

// getOrAssignIDBulk returns the compact ID, assigning if needed.
// NOT thread-safe — for single-threaded bulk build only.
func (s *shardedAttrIndex) getOrAssignIDBulk(labelsHash uint64) uint32 {
	if id, ok := s.forward[labelsHash]; ok {
		return id
	}
	id := uint32(len(s.reverse))
	s.forward[labelsHash] = id
	s.reverse = append(s.reverse, labelsHash)
	return id
}

// lookupID returns the compact ID for a labelsHash, if mapped. Thread-safe for reads.
func (s *shardedAttrIndex) lookupID(labelsHash uint64) (uint32, bool) {
	s.idMu.RLock()
	id, ok := s.forward[labelsHash]
	s.idMu.RUnlock()
	return id, ok
}

func (s *shardedAttrIndex) stripe(key string) *attrIndexStripe {
	h := xxhash.Sum64String(key)
	return &s.stripes[h&uint64(numAttrIndexStripes-1)]
}

func (s *shardedAttrIndex) stripeBytes(key []byte) *attrIndexStripe {
	h := xxhash.Sum64(key)
	return &s.stripes[h&uint64(numAttrIndexStripes-1)]
}

// lookup returns a sorted slice of labelsHashes for a given index key.
// The returned slice is owned by the caller.
func (s *shardedAttrIndex) lookup(key string) []uint64 {
	st := s.stripe(key)
	st.mtx.RLock()
	defer st.mtx.RUnlock()
	pl := st.idx[key]
	if pl.isEmpty() {
		return nil
	}
	s.idMu.RLock()
	result := pl.toArray(s.reverse)
	s.idMu.RUnlock()
	return result
}

// MemSeriesMetadata is an in-memory implementation of series metadata storage.
// It wraps per-kind stores accessible both generically (via IterKind/KindLen)
// and type-safely (via ResourceStore).
type MemSeriesMetadata struct {
	resourceStore *MemStore[*ResourceVersion]
	labelsMap     map[uint64]labels.Labels // labelsHash → labels.Labels

	// resourceAttrIndex is a 256-way sharded inverted index mapping
	// "key\x00value" → sorted []uint64 of labelsHashes.
	// Uses copy-on-write sorted slices for ~4x memory reduction vs maps and
	// zero-copy reads (readers holding old slices are safe).
	// Covers identifying attributes (always) and descriptive attributes
	// only when the key is in indexedResourceAttrs.
	// Built lazily via BuildResourceAttrIndex() or incrementally via
	// UpdateResourceAttrIndex(). nil until first build or incremental init.
	resourceAttrIndex *shardedAttrIndex // nil until first build/init

	// indexedResourceAttrs specifies additional descriptive resource attribute
	// names to include in the inverted index beyond identifying attributes
	// (which are always indexed). nil means index only identifying attributes.
	indexedResourceAttrs   map[string]struct{}
	indexedResourceAttrsMu sync.RWMutex // protects indexedResourceAttrs

	// uniqueAttrNames is a grow-only cache of all resource attribute names
	// seen across all resource versions. Updated incrementally in addToAttrIndex
	// and BuildResourceAttrIndex. Cardinality is typically tiny (<100 names).
	uniqueAttrNames   map[string]struct{}
	uniqueAttrNamesMu sync.RWMutex

	// Lazy attr index build: when attrIndexEnabled is true, the first call to
	// LookupResourceAttr triggers BuildResourceAttrIndex via sync.Once.
	buildAttrIndexOnce sync.Once
	attrIndexEnabled   bool
}

// NewMemSeriesMetadata creates a new in-memory series metadata store.
func NewMemSeriesMetadata() *MemSeriesMetadata {
	return &MemSeriesMetadata{
		resourceStore: NewMemStore[*ResourceVersion](ResourceOps),
		labelsMap:     make(map[uint64]labels.Labels),
	}
}

// ResourceStore returns the typed resource store.
func (m *MemSeriesMetadata) ResourceStore() *MemStore[*ResourceVersion] {
	return m.resourceStore
}

// ResourceHasContentHash reports whether the series at labelsHash has a
// resource version with the given contentHash. Read-only, zero-allocation.
func (m *MemSeriesMetadata) ResourceHasContentHash(labelsHash, contentHash uint64) bool {
	return m.ResourceStore().HasContentHash(labelsHash, contentHash)
}

// Close is a no-op for in-memory storage.
func (*MemSeriesMetadata) Close() error { return nil }

// SetIndexedResourceAttrs replaces the set of additional descriptive resource
// attribute names included in the inverted index. Identifying attributes
// are always indexed regardless of this setting.
// Thread-safe: uses the same mutex as index operations.
// The caller must not mutate the map after passing it — the store takes
// ownership. To change the set, call SetIndexedResourceAttrs with a new map;
// previously returned references from GetIndexedResourceAttrs remain valid
// and unchanged (replace-not-mutate semantics).
// Note: changing the indexed set does NOT retroactively rebuild the index —
// it only affects future updates. The caller should rebuild if needed.
func (m *MemSeriesMetadata) SetIndexedResourceAttrs(attrs map[string]struct{}) {
	m.indexedResourceAttrsMu.Lock()
	defer m.indexedResourceAttrsMu.Unlock()
	m.indexedResourceAttrs = attrs
}

// GetIndexedResourceAttrs returns the current set of additional descriptive
// resource attribute names included in the inverted index.
// The returned map must not be modified by the caller.
func (m *MemSeriesMetadata) GetIndexedResourceAttrs() map[string]struct{} {
	m.indexedResourceAttrsMu.RLock()
	defer m.indexedResourceAttrsMu.RUnlock()
	return m.indexedResourceAttrs
}

// UniqueResourceAttrNames returns a snapshot of all resource attribute names
// that have been seen. The returned map must not be modified by the caller.
// This is O(1) — no iteration required.
func (m *MemSeriesMetadata) UniqueResourceAttrNames() map[string]struct{} {
	m.uniqueAttrNamesMu.RLock()
	defer m.uniqueAttrNamesMu.RUnlock()
	return m.uniqueAttrNames
}

// SetLabels associates a labels set with a labels hash for later lookup.
func (m *MemSeriesMetadata) SetLabels(labelsHash uint64, lset labels.Labels) {
	m.labelsMap[labelsHash] = lset
}

// LabelsForHash returns the labels for a given labels hash, if available.
func (m *MemSeriesMetadata) LabelsForHash(labelsHash uint64) (labels.Labels, bool) {
	lset, ok := m.labelsMap[labelsHash]
	return lset, ok
}

// IterKind iterates all entries for a kind (type-erased).
// Uses IterVersionedResources under the hood.
func (m *MemSeriesMetadata) IterKind(ctx context.Context, id KindID, f func(labelsHash uint64, versioned any) error) error {
	switch id {
	case KindResource:
		return m.IterVersionedResources(ctx, func(labelsHash uint64, vr *VersionedResource) error {
			return f(labelsHash, vr)
		})
	default:
		return nil
	}
}

// IterHashes iterates labelsHashes for a kind without materializing versions.
func (m *MemSeriesMetadata) IterHashes(ctx context.Context, id KindID, f func(labelsHash uint64) error) error {
	switch id {
	case KindResource:
		return m.ResourceStore().IterHashes(ctx, f)
	default:
		return nil
	}
}

// KindLen returns the number of entries for a kind.
func (m *MemSeriesMetadata) KindLen(id KindID) int {
	switch id {
	case KindResource:
		return m.resourceStore.Len()
	default:
		return 0
	}
}

// --- Resource type-safe accessors (VersionedResourceReader) ---

func (m *MemSeriesMetadata) GetResource(labelsHash uint64) (*ResourceVersion, bool) {
	return m.ResourceStore().Get(labelsHash)
}

func (m *MemSeriesMetadata) GetVersionedResource(labelsHash uint64) (*VersionedResource, bool) {
	return m.ResourceStore().GetVersioned(labelsHash)
}

func (m *MemSeriesMetadata) GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool) {
	return m.ResourceStore().GetAt(labelsHash, timestamp)
}

func (m *MemSeriesMetadata) SetResource(labelsHash uint64, resource *ResourceVersion) {
	m.ResourceStore().Set(labelsHash, resource)
}

func (m *MemSeriesMetadata) SetVersionedResource(labelsHash uint64, resources *VersionedResource) {
	m.ResourceStore().SetVersioned(labelsHash, resources)
}

func (m *MemSeriesMetadata) DeleteResource(labelsHash uint64) {
	m.ResourceStore().Delete(labelsHash)
}

func (m *MemSeriesMetadata) IterResources(ctx context.Context, f func(labelsHash uint64, resource *ResourceVersion) error) error {
	return m.ResourceStore().IterVersionedFlatInline(ctx, func(labelsHash uint64, versions []*ResourceVersion, _, _ int64, _ bool) error {
		if len(versions) == 0 {
			return nil
		}
		return f(labelsHash, versions[len(versions)-1])
	})
}

func (m *MemSeriesMetadata) IterVersionedResources(ctx context.Context, f func(labelsHash uint64, resources *VersionedResource) error) error {
	return m.ResourceStore().IterVersionedFlatInline(ctx, func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error {
		if isInline && len(versions) == 1 {
			thin := resourceOps{}.ThinCopy(versions[0], versions[0])
			thin.MinTime = inlineMinTime
			thin.MaxTime = inlineMaxTime
			return f(labelsHash, &Versioned[*ResourceVersion]{Versions: []*ResourceVersion{thin}})
		}
		return f(labelsHash, &Versioned[*ResourceVersion]{Versions: versions})
	})
}

func (m *MemSeriesMetadata) IterVersionedResourcesFlatInline(ctx context.Context, f func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error) error {
	return m.ResourceStore().IterVersionedFlatInline(ctx, f)
}

func (m *MemSeriesMetadata) IterVersionedResourcesFlatInlineWithContentHash(ctx context.Context, f func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool, contentHash uint64) error) error {
	return m.ResourceStore().IterVersionedFlatInlineWithContentHash(ctx, f)
}

func (m *MemSeriesMetadata) TotalResources() uint64 {
	return m.ResourceStore().TotalEntries()
}

func (m *MemSeriesMetadata) TotalResourceVersions() uint64 {
	return m.ResourceStore().TotalVersions()
}

// SetAttrIndexEnabled marks that the attr index should be built on first query.
// Call this instead of BuildResourceAttrIndex to defer the expensive build
// until it's actually needed.
func (m *MemSeriesMetadata) SetAttrIndexEnabled(enabled bool) {
	m.attrIndexEnabled = enabled
}

// ensureResourceAttrIndex builds the attr index on first call if enabled.
func (m *MemSeriesMetadata) ensureResourceAttrIndex() {
	if !m.attrIndexEnabled {
		return
	}
	m.buildAttrIndexOnce.Do(m.BuildResourceAttrIndex)
}

// BuildResourceAttrIndex builds the inverted index from all resource versions.
// Called once after merge in mergeBlockMetadata. After this, LookupResourceAttr
// returns results in O(1) instead of requiring a full scan.
// Skips rebuilding if the index is already populated (e.g. from Parquet or
// incremental updates).
// Uses bulk append + sort instead of per-entry sortedInsert to avoid O(n²)
// cost when building from scratch.
func (m *MemSeriesMetadata) BuildResourceAttrIndex() {
	if m.resourceAttrIndex != nil {
		return
	}
	idx := newShardedAttrIndex()
	names := make(map[string]struct{})
	m.indexedResourceAttrsMu.RLock()
	extra := m.indexedResourceAttrs
	m.indexedResourceAttrsMu.RUnlock()
	var buf bytes.Buffer
	_ = m.ResourceStore().IterVersionedFlatInline(context.Background(), func(labelsHash uint64, versions []*ResourceVersion, _, _ int64, _ bool) error {
		for _, rv := range versions {
			bulkAddToAttrIndex(idx, labelsHash, rv, extra, &buf)
			collectAttrNames(names, rv)
		}
		return nil
	})
	finalizeBulkAttrIndex(idx)
	m.resourceAttrIndex = idx

	m.uniqueAttrNamesMu.Lock()
	m.uniqueAttrNames = names
	m.uniqueAttrNamesMu.Unlock()
}

// InitResourceAttrIndex initializes an empty inverted index, enabling
// incremental updates via UpdateResourceAttrIndex. This must be called
// before any incremental updates (e.g. on head startup).
func (m *MemSeriesMetadata) InitResourceAttrIndex() {
	if m.resourceAttrIndex == nil {
		m.resourceAttrIndex = newShardedAttrIndex()
	}
}

// UpdateResourceAttrIndex incrementally updates the inverted index when a
// resource version changes. Removes stale entries from old, adds new ones.
// old may be nil if this is the first insert for this labelsHash.
//
// Callers must only invoke this when content has actually changed (not for
// time-range-only extensions). This avoids the expensive remove+add cycle
// on the >99% hot path.
func (m *MemSeriesMetadata) UpdateResourceAttrIndex(
	labelsHash uint64,
	old *VersionedResource,
	cur *VersionedResource,
) {
	// Track new attr names from the current version (grow-only).
	// Always runs even without inverted index — used for autocomplete.
	if cur != nil {
		m.uniqueAttrNamesMu.Lock()
		if m.uniqueAttrNames == nil {
			m.uniqueAttrNames = make(map[string]struct{})
		}
		for _, rv := range cur.Versions {
			collectAttrNames(m.uniqueAttrNames, rv)
		}
		m.uniqueAttrNamesMu.Unlock()
	}

	if m.resourceAttrIndex == nil {
		return
	}
	m.indexedResourceAttrsMu.RLock()
	extra := m.indexedResourceAttrs
	m.indexedResourceAttrsMu.RUnlock()

	var buf bytes.Buffer
	// Remove old entries.
	if old != nil {
		for _, rv := range old.Versions {
			removeFromAttrIndex(m.resourceAttrIndex, labelsHash, rv, extra, &buf)
		}
	}
	// Add current entries.
	if cur != nil {
		for _, rv := range cur.Versions {
			addToAttrIndex(m.resourceAttrIndex, labelsHash, rv, extra, &buf)
		}
	}
}

// RemoveFromResourceAttrIndex removes all index entries for a labelsHash.
func (m *MemSeriesMetadata) RemoveFromResourceAttrIndex(labelsHash uint64, vr *VersionedResource) {
	if vr == nil {
		return
	}
	if m.resourceAttrIndex == nil {
		return
	}
	m.indexedResourceAttrsMu.RLock()
	extra := m.indexedResourceAttrs
	m.indexedResourceAttrsMu.RUnlock()
	var buf bytes.Buffer
	for _, rv := range vr.Versions {
		removeFromAttrIndex(m.resourceAttrIndex, labelsHash, rv, extra, &buf)
	}
}

// collectAttrNames adds all attribute names from a resource version to the name set.
func collectAttrNames(names map[string]struct{}, rv *ResourceVersion) {
	for k := range rv.Identifying {
		names[k] = struct{}{}
	}
	for k := range rv.Descriptive {
		names[k] = struct{}{}
	}
}

// forEachAttrKey iterates all indexable "key\x00value" entries for a resource version.
// Identifying attributes are always included. Descriptive attributes are only
// included if their key is in extraIndexed.
// The buf is reused to build index keys without allocating.
func forEachAttrKey(rv *ResourceVersion, extraIndexed map[string]struct{}, buf *bytes.Buffer, fn func(key []byte)) {
	for k, v := range rv.Identifying {
		buf.Reset()
		buf.WriteString(k)
		buf.WriteByte('\x00')
		buf.WriteString(v)
		fn(buf.Bytes())
	}
	for k, v := range rv.Descriptive {
		if _, ok := extraIndexed[k]; !ok {
			continue
		}
		buf.Reset()
		buf.WriteString(k)
		buf.WriteByte('\x00')
		buf.WriteString(v)
		fn(buf.Bytes())
	}
}

// addToAttrIndex adds attribute entries for a resource version to the sharded index.
// Uses in-place sorted insert through *[]uint64 pointers (copy-on-read for readers).
// Each key routes to a single stripe — no two stripe locks are held simultaneously.
// string(buf.Bytes()) in map index expressions triggers Go's compiler optimization
// for zero-alloc lookups.
func addToAttrIndex(idx *shardedAttrIndex, labelsHash uint64, rv *ResourceVersion, extraIndexed map[string]struct{}, buf *bytes.Buffer) {
	forEachAttrKey(rv, extraIndexed, buf, func(key []byte) {
		addToAttrIndexEntry(idx, key, labelsHash)
	})
}

// addToAttrIndexEntry adds a labelsHash to the posting list for the given key.
// When the posting list already has a bitmap, the bitmap is mutated through its
// pointer without a map write-back, avoiding Go's per-write string key allocation.
// Only structural changes (inline→bitmap promotion or first insert) write to the map.
func addToAttrIndexEntry(idx *shardedAttrIndex, key []byte, labelsHash uint64) {
	st := idx.stripeBytes(key)
	st.mtx.Lock()
	pl, exists := st.idx[string(key)]
	if exists && pl.bitmap != nil {
		// Bitmap mutation goes through the pointer — no map write needed.
		pl.bitmap.Add(idx.getOrAssignID(labelsHash))
	} else {
		// Inline or new: try inline add first.
		var needPromo bool
		pl, needPromo = pl.addInline(labelsHash)
		if needPromo {
			pl = pl.promote(idx.getOrAssignID)
		}
		st.idx[string(key)] = pl
	}
	st.mtx.Unlock()
}

// removeFromAttrIndex removes attribute entries for a resource version from the sharded index.
func removeFromAttrIndex(idx *shardedAttrIndex, labelsHash uint64, rv *ResourceVersion, extraIndexed map[string]struct{}, buf *bytes.Buffer) {
	forEachAttrKey(rv, extraIndexed, buf, func(key []byte) {
		removeFromAttrIndexEntry(idx, key, labelsHash)
	})
}

// removeFromAttrIndexEntry removes a labelsHash from the posting list for the given key.
// Deletes the map entry entirely if the posting list becomes empty.
func removeFromAttrIndexEntry(idx *shardedAttrIndex, key []byte, labelsHash uint64) {
	st := idx.stripeBytes(key)
	st.mtx.Lock()
	pl, exists := st.idx[string(key)]
	if !exists {
		st.mtx.Unlock()
		return
	}
	if pl.bitmap != nil {
		if id, ok := idx.lookupID(labelsHash); ok {
			pl.bitmap.Remove(id)
		}
		if pl.bitmap.IsEmpty() {
			delete(st.idx, string(key))
		}
	} else {
		pl = pl.removeInline(labelsHash)
		if pl.isEmpty() {
			delete(st.idx, string(key))
		} else {
			st.idx[string(key)] = pl
		}
	}
	st.mtx.Unlock()
}

// bulkAddToAttrIndex appends labelsHash to posting lists without maintaining sort order.
// Used during BuildResourceAttrIndex for O(n) build; finalizeBulkAttrIndex sorts afterward.
func bulkAddToAttrIndex(idx *shardedAttrIndex, labelsHash uint64, rv *ResourceVersion, extraIndexed map[string]struct{}, buf *bytes.Buffer) {
	forEachAttrKey(rv, extraIndexed, buf, func(key []byte) {
		bulkAddToAttrIndexEntry(idx, key, labelsHash)
	})
}

// bulkAddToAttrIndexEntry adds to a posting list (no lock, single-threaded bulk phase).
func bulkAddToAttrIndexEntry(idx *shardedAttrIndex, key []byte, labelsHash uint64) {
	st := idx.stripeBytes(key)
	pl, exists := st.idx[string(key)]
	if exists && pl.bitmap != nil {
		pl.bitmap.Add(idx.getOrAssignIDBulk(labelsHash))
	} else {
		var needPromo bool
		pl, needPromo = pl.addInline(labelsHash)
		if needPromo {
			pl = pl.promote(idx.getOrAssignIDBulk)
		}
		st.idx[string(key)] = pl
	}
}

// finalizeBulkAttrIndex optimizes all posting lists for memory after bulk insertion.
// RunOptimize applies run-length encoding where beneficial for promoted bitmaps.
func finalizeBulkAttrIndex(idx *shardedAttrIndex) {
	for i := range idx.stripes {
		st := &idx.stripes[i]
		for key, pl := range st.idx {
			if pl.isEmpty() {
				delete(st.idx, key)
			} else {
				st.idx[key] = pl.runOptimize()
			}
		}
	}
}

// AttrIndexKeyCount returns the total number of distinct keys across all stripes
// of the resource attribute inverted index. Returns 0 if the index has not been built.
func (m *MemSeriesMetadata) AttrIndexKeyCount() int {
	if m.resourceAttrIndex == nil {
		return 0
	}
	var total int
	for i := range m.resourceAttrIndex.stripes {
		st := &m.resourceAttrIndex.stripes[i]
		st.mtx.RLock()
		total += len(st.idx)
		st.mtx.RUnlock()
	}
	return total
}

// LookupResourceAttr returns sorted labelsHashes that have a resource version
// with the given key:value in Identifying or Descriptive attributes.
// Returns nil if the index has not been built and lazy build is not enabled.
// The returned slice is a copy, safe for use after the call returns.
func (m *MemSeriesMetadata) LookupResourceAttr(key, value string) []uint64 {
	m.ensureResourceAttrIndex()
	if m.resourceAttrIndex == nil {
		return nil
	}
	return m.resourceAttrIndex.lookup(key + "\x00" + value)
}
