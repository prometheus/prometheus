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
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/RoaringBitmap/roaring"
	"github.com/cespare/xxhash/v2"
	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// SeriesMetadataFilename is the name of the series metadata file in a block directory.
const SeriesMetadataFilename = "series_metadata.parquet"

// schemaVersion is stored in the Parquet footer for future schema evolution.
const schemaVersion = "1"

// Reader provides read access to series metadata (OTel resources and scopes).
type Reader interface {
	// Close releases any resources associated with the reader.
	Close() error

	// VersionedResourceReader provides access to versioned OTel resources.
	VersionedResourceReader

	// VersionedScopeReader provides access to versioned OTel InstrumentationScope data.
	VersionedScopeReader

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

// LabelsPopulator allows post-construction population of the labels map.
type LabelsPopulator interface {
	SetLabels(labelsHash uint64, lset labels.Labels)
}

// UniqueAttrNameReader is optionally implemented by Reader implementations
// that maintain a cached set of unique resource attribute names. Checking
// this via type assertion avoids O(N_series) full scans.
type UniqueAttrNameReader interface {
	UniqueResourceAttrNames() map[string]struct{}
}

// InlineFlatResourceIterator is optionally implemented by Reader implementations
// that can iterate resource versions without allocating ThinCopy for single-version
// entries. When isInline is true, the versions slice contains the canonical directly
// and the caller should use inlineMinTime/inlineMaxTime for the time range.
type InlineFlatResourceIterator interface {
	IterVersionedResourcesFlatInline(ctx context.Context, f func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error) error
}

// InlineFlatScopeIterator is optionally implemented by Reader implementations
// that can iterate scope versions without allocating ThinCopy for single-version entries.
type InlineFlatScopeIterator interface {
	IterVersionedScopesFlatInline(ctx context.Context, f func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error) error
}

// iterResourcesFlatInline uses InlineFlatResourceIterator if available, otherwise
// falls back to IterVersionedResources with an adapter.
func iterResourcesFlatInline(ctx context.Context, mr Reader, f func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error) error {
	if inline, ok := mr.(InlineFlatResourceIterator); ok {
		return inline.IterVersionedResourcesFlatInline(ctx, f)
	}
	return mr.IterVersionedResources(ctx, func(labelsHash uint64, vr *VersionedResource) error {
		return f(labelsHash, vr.Versions, 0, 0, false)
	})
}

// iterScopesFlatInline uses InlineFlatScopeIterator if available, otherwise
// falls back to IterVersionedScopes with an adapter.
func iterScopesFlatInline(ctx context.Context, mr Reader, f func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error) error {
	if inline, ok := mr.(InlineFlatScopeIterator); ok {
		return inline.IterVersionedScopesFlatInline(ctx, f)
	}
	return mr.IterVersionedScopes(ctx, func(labelsHash uint64, vs *VersionedScope) error {
		return f(labelsHash, vs.Versions, 0, 0, false)
	})
}

// InlineFlatResourceIteratorWithContentHash extends InlineFlatResourceIterator
// with a cached contentHash parameter. For single-version inline entries, the
// MemStore's cached contentHash is passed directly (non-zero), avoiding
// recomputation during writes. Multi-version entries pass contentHash=0.
type InlineFlatResourceIteratorWithContentHash interface {
	IterVersionedResourcesFlatInlineWithContentHash(ctx context.Context, f func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool, contentHash uint64) error) error
}

// InlineFlatScopeIteratorWithContentHash is the scope equivalent.
type InlineFlatScopeIteratorWithContentHash interface {
	IterVersionedScopesFlatInlineWithContentHash(ctx context.Context, f func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool, contentHash uint64) error) error
}

// iterResourcesFlatInlineWithContentHash uses the WithContentHash variant if available,
// falls back to InlineFlatResourceIterator (contentHash=0), then IterVersionedResources.
func iterResourcesFlatInlineWithContentHash(ctx context.Context, mr Reader, f func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool, contentHash uint64) error) error {
	if ch, ok := mr.(InlineFlatResourceIteratorWithContentHash); ok {
		return ch.IterVersionedResourcesFlatInlineWithContentHash(ctx, f)
	}
	return iterResourcesFlatInline(ctx, mr, func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error {
		return f(labelsHash, versions, inlineMinTime, inlineMaxTime, isInline, 0)
	})
}

// iterScopesFlatInlineWithContentHash uses the WithContentHash variant if available,
// falls back to InlineFlatScopeIterator (contentHash=0), then IterVersionedScopes.
func iterScopesFlatInlineWithContentHash(ctx context.Context, mr Reader, f func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool, contentHash uint64) error) error {
	if ch, ok := mr.(InlineFlatScopeIteratorWithContentHash); ok {
		return ch.IterVersionedScopesFlatInlineWithContentHash(ctx, f)
	}
	return iterScopesFlatInline(ctx, mr, func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error {
		return f(labelsHash, versions, inlineMinTime, inlineMaxTime, isInline, 0)
	})
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
// and type-safely (via ResourceStore/ScopeStore).
type MemSeriesMetadata struct {
	stores    map[KindID]any           // each value is *MemStore[V] for the appropriate V
	labelsMap map[uint64]labels.Labels // labelsHash → labels.Labels

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
	m := &MemSeriesMetadata{
		stores:    make(map[KindID]any, len(allKindsRegistered)),
		labelsMap: make(map[uint64]labels.Labels),
	}
	for _, kind := range allKindsRegistered {
		m.stores[kind.ID()] = kind.NewStore()
	}
	return m
}

// ResourceStore returns the typed resource store.
func (m *MemSeriesMetadata) ResourceStore() *MemStore[*ResourceVersion] {
	return m.stores[KindResource].(*MemStore[*ResourceVersion])
}

// ScopeStore returns the typed scope store.
func (m *MemSeriesMetadata) ScopeStore() *MemStore[*ScopeVersion] {
	return m.stores[KindScope].(*MemStore[*ScopeVersion])
}

// StoreForKind returns the type-erased store for a kind.
func (m *MemSeriesMetadata) StoreForKind(id KindID) any {
	return m.stores[id]
}

// ResourceHasContentHash reports whether the series at labelsHash has a
// resource version with the given contentHash. Read-only, zero-allocation.
func (m *MemSeriesMetadata) ResourceHasContentHash(labelsHash, contentHash uint64) bool {
	return m.ResourceStore().HasContentHash(labelsHash, contentHash)
}

// ScopeHasContentHash reports whether the series at labelsHash has a
// scope version with the given contentHash. Read-only, zero-allocation.
func (m *MemSeriesMetadata) ScopeHasContentHash(labelsHash, contentHash uint64) bool {
	return m.ScopeStore().HasContentHash(labelsHash, contentHash)
}

// ResourceCount returns the number of unique series with resource data.
func (m *MemSeriesMetadata) ResourceCount() int { return m.ResourceStore().Len() }

// ScopeCount returns the number of unique series with scope data.
func (m *MemSeriesMetadata) ScopeCount() int { return m.ScopeStore().Len() }

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

// DeleteLabels removes the labels mapping for a given hash.
func (m *MemSeriesMetadata) DeleteLabels(labelsHash uint64) {
	delete(m.labelsMap, labelsHash)
}

// LabelsForHash returns the labels for a given labels hash, if available.
func (m *MemSeriesMetadata) LabelsForHash(labelsHash uint64) (labels.Labels, bool) {
	lset, ok := m.labelsMap[labelsHash]
	return lset, ok
}

// IterKind iterates all entries for a kind (type-erased).
// Uses IterVersionedResources/IterVersionedScopes under the hood.
func (m *MemSeriesMetadata) IterKind(ctx context.Context, id KindID, f func(labelsHash uint64, versioned any) error) error {
	switch id {
	case KindResource:
		return m.IterVersionedResources(ctx, func(labelsHash uint64, vr *VersionedResource) error {
			return f(labelsHash, vr)
		})
	case KindScope:
		return m.IterVersionedScopes(ctx, func(labelsHash uint64, vs *VersionedScope) error {
			return f(labelsHash, vs)
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
	case KindScope:
		return m.ScopeStore().IterHashes(ctx, f)
	default:
		return nil
	}
}

// KindLen returns the number of entries for a kind.
func (m *MemSeriesMetadata) KindLen(id KindID) int {
	kind, ok := KindByID(id)
	if !ok {
		return 0
	}
	store, ok := m.stores[id]
	if !ok {
		return 0
	}
	return kind.StoreLen(store)
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

// --- Scope type-safe accessors (VersionedScopeReader) ---

func (m *MemSeriesMetadata) GetVersionedScope(labelsHash uint64) (*VersionedScope, bool) {
	return m.ScopeStore().GetVersioned(labelsHash)
}

func (m *MemSeriesMetadata) SetVersionedScope(labelsHash uint64, scopes *VersionedScope) {
	m.ScopeStore().SetVersioned(labelsHash, scopes)
}

func (m *MemSeriesMetadata) IterVersionedScopes(ctx context.Context, f func(labelsHash uint64, scopes *VersionedScope) error) error {
	return m.ScopeStore().IterVersionedFlatInline(ctx, func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error {
		if isInline && len(versions) == 1 {
			thin := scopeOps{}.ThinCopy(versions[0], versions[0])
			thin.MinTime = inlineMinTime
			thin.MaxTime = inlineMaxTime
			return f(labelsHash, &Versioned[*ScopeVersion]{Versions: []*ScopeVersion{thin}})
		}
		return f(labelsHash, &Versioned[*ScopeVersion]{Versions: versions})
	})
}

func (m *MemSeriesMetadata) IterVersionedScopesFlatInline(ctx context.Context, f func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error) error {
	return m.ScopeStore().IterVersionedFlatInline(ctx, f)
}

func (m *MemSeriesMetadata) IterVersionedScopesFlatInlineWithContentHash(ctx context.Context, f func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool, contentHash uint64) error) error {
	return m.ScopeStore().IterVersionedFlatInlineWithContentHash(ctx, f)
}

func (m *MemSeriesMetadata) TotalScopes() uint64 {
	return m.ScopeStore().TotalEntries()
}

func (m *MemSeriesMetadata) TotalScopeVersions() uint64 {
	return m.ScopeStore().TotalVersions()
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

// addToAttrIndex adds attribute entries for a resource version to the sharded index.
// Identifying attributes are always indexed. Descriptive attributes are only
// indexed if their key is in extraIndexed.
// Uses in-place sorted insert through *[]uint64 pointers (copy-on-read for readers).
// Each key routes to a single stripe — no two stripe locks are held simultaneously.
// The buf is used to build index keys without allocating; string(buf.Bytes()) in
// map index expressions triggers Go's compiler optimization for zero-alloc lookups.
func addToAttrIndex(idx *shardedAttrIndex, labelsHash uint64, rv *ResourceVersion, extraIndexed map[string]struct{}, buf *bytes.Buffer) {
	for k, v := range rv.Identifying {
		buf.Reset()
		buf.WriteString(k)
		buf.WriteByte('\x00')
		buf.WriteString(v)
		addToAttrIndexEntry(idx, buf.Bytes(), labelsHash)
	}
	for k, v := range rv.Descriptive {
		if _, ok := extraIndexed[k]; !ok {
			continue
		}
		buf.Reset()
		buf.WriteString(k)
		buf.WriteByte('\x00')
		buf.WriteString(v)
		addToAttrIndexEntry(idx, buf.Bytes(), labelsHash)
	}
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
// Identifying attributes are always removed. Descriptive attributes are only
// removed if their key is in extraIndexed.
// Uses in-place sorted remove through *[]uint64 pointers (copy-on-read for readers).
// Each key routes to a single stripe — no two stripe locks are held simultaneously.
func removeFromAttrIndex(idx *shardedAttrIndex, labelsHash uint64, rv *ResourceVersion, extraIndexed map[string]struct{}, buf *bytes.Buffer) {
	for k, v := range rv.Identifying {
		buf.Reset()
		buf.WriteString(k)
		buf.WriteByte('\x00')
		buf.WriteString(v)
		removeFromAttrIndexEntry(idx, buf.Bytes(), labelsHash)
	}
	for k, v := range rv.Descriptive {
		if _, ok := extraIndexed[k]; !ok {
			continue
		}
		buf.Reset()
		buf.WriteString(k)
		buf.WriteByte('\x00')
		buf.WriteString(v)
		removeFromAttrIndexEntry(idx, buf.Bytes(), labelsHash)
	}
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
	for k, v := range rv.Identifying {
		buf.Reset()
		buf.WriteString(k)
		buf.WriteByte('\x00')
		buf.WriteString(v)
		bulkAddToAttrIndexEntry(idx, buf.Bytes(), labelsHash)
	}
	for k, v := range rv.Descriptive {
		if _, ok := extraIndexed[k]; !ok {
			continue
		}
		buf.Reset()
		buf.WriteString(k)
		buf.WriteByte('\x00')
		buf.WriteString(v)
		bulkAddToAttrIndexEntry(idx, buf.Bytes(), labelsHash)
	}
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

// parquetReader implements Reader by reading from a Parquet file.
type parquetReader struct {
	closer io.Closer // nil for ReaderAt-based readers (caller manages lifecycle)
	mem    *MemSeriesMetadata

	closeOnce sync.Once
	closeErr  error
}

// contentMapping pairs a series (by SeriesRef) with a content hash and time range.
type contentMapping struct {
	seriesRef   uint64
	contentHash uint64
	minTime     int64
	maxTime     int64
}

// kindDenormState holds per-kind state during row denormalization.
type kindDenormState struct {
	kind           KindDescriptor
	contentTable   map[uint64]any               // contentHash → version value (type-erased)
	mappings       []contentMapping             // series → content hash + time range
	versionsByHash map[uint64][]VersionWithTime // labelsHash → versions with time ranges
}

// denormalizeRows processes raw Parquet rows into in-memory lookup structures.
// It uses the kind registry to dispatch table/mapping rows generically.
func denormalizeRows(
	logger *slog.Logger,
	rows []metadataRow,
	mem *MemSeriesMetadata,
	refResolver func(seriesRef uint64) (labelsHash uint64, ok bool),
) {
	// Phase 1: Build content-addressed tables and collect mappings per kind.
	states := make(map[KindID]*kindDenormState)
	for _, kind := range AllKinds() {
		states[kind.ID()] = &kindDenormState{
			kind:           kind,
			contentTable:   make(map[uint64]any),
			versionsByHash: make(map[uint64][]VersionWithTime),
		}
	}

	for i := range rows {
		row := &rows[i]

		if kind, ok := KindByTableNS(row.Namespace); ok {
			state := states[kind.ID()]
			state.contentTable[row.ContentHash] = kind.ParseTableRow(logger, row)
		} else if kind, ok := KindByMappingNS(row.Namespace); ok {
			state := states[kind.ID()]
			state.mappings = append(state.mappings, contentMapping{
				seriesRef:   row.SeriesRef,
				contentHash: row.ContentHash,
				minTime:     row.MinTime,
				maxTime:     row.MaxTime,
			})
		}
	}

	// Phase 2: Resolve mappings by looking up content from tables.
	for _, state := range states {
		for _, m := range state.mappings {
			template, ok := state.contentTable[m.contentHash]
			if !ok {
				logger.Warn("Mapping references missing content hash",
					"kind", string(state.kind.ID()),
					"series_ref", m.seriesRef, "content_hash", m.contentHash)
				continue
			}
			labelsHash := m.seriesRef
			if refResolver != nil {
				lh, ok := refResolver(m.seriesRef)
				if !ok {
					logger.Warn("Mapping references unresolvable series ref",
						"kind", string(state.kind.ID()),
						"series_ref", m.seriesRef, "content_hash", m.contentHash)
					continue
				}
				labelsHash = lh
			}
			// Copy the template and set time range.
			// The kind descriptor's CopyVersioned works on *Versioned[V], but here
			// we have a single version. We'll wrap and use kind.SetVersioned.
			// For now, accumulate raw versions and build Versioned in phase 3.
			state.versionsByHash[labelsHash] = append(state.versionsByHash[labelsHash], VersionWithTime{
				Version: template,
				MinTime: m.minTime,
				MaxTime: m.maxTime,
			})
		}
	}

	// Phase 3: Sort versions by MinTime and populate stores (kind-generic).
	for _, kind := range AllKinds() {
		state := states[kind.ID()]
		store := mem.StoreForKind(kind.ID())
		for labelsHash, rawVersions := range state.versionsByHash {
			kind.DenormalizeIntoStore(store, labelsHash, rawVersions)
		}
	}

	// Phase 4: Process resource attribute inverted index rows.
	// Prefer dedicated AttrKey/AttrValue columns when non-empty (new files),
	// fall back to IdentifyingAttrs[0] for backward compatibility (old files).
	// Build into a local shardedAttrIndex, then assign atomically.
	var idx *shardedAttrIndex
	for i := range rows {
		row := &rows[i]
		if row.Namespace != NamespaceResourceAttrIndex {
			continue
		}
		if idx == nil {
			idx = newShardedAttrIndex()
		}

		var attrKey, attrValue string
		switch {
		case row.AttrKey != "":
			attrKey = row.AttrKey
			attrValue = row.AttrValue
		case len(row.IdentifyingAttrs) > 0:
			attrKey = row.IdentifyingAttrs[0].Key
			attrValue = row.IdentifyingAttrs[0].Value
		default:
			continue
		}

		labelsHash := row.SeriesRef
		if refResolver != nil {
			lh, ok := refResolver(row.SeriesRef)
			if !ok {
				continue
			}
			labelsHash = lh
		}
		key := attrKey + "\x00" + attrValue
		// Single-threaded during Parquet load — no stripe locking needed,
		// but use stripe routing for correct placement.
		st := idx.stripe(key)
		pl, exists := st.idx[key]
		if exists && pl.bitmap != nil {
			pl.bitmap.Add(idx.getOrAssignIDBulk(labelsHash))
		} else {
			var needPromo bool
			pl, needPromo = pl.addInline(labelsHash)
			if needPromo {
				pl = pl.promote(idx.getOrAssignIDBulk)
			}
			st.idx[key] = pl
		}
	}
	if idx != nil {
		mem.resourceAttrIndex = idx
	}
}

// VersionWithTime wraps a version value with its time range from a mapping row.
type VersionWithTime struct {
	Version any
	MinTime int64
	MaxTime int64
}

// newParquetReaderFromReaderAt creates a parquetReader from an io.ReaderAt.
func newParquetReaderFromReaderAt(logger *slog.Logger, r io.ReaderAt, size int64, opts ...ReaderOption) (*parquetReader, error) {
	var ropts readerOptions
	for _, o := range opts {
		o(&ropts)
	}

	pf, err := parquet.OpenFile(r, size)
	if err != nil {
		return nil, fmt.Errorf("open parquet file: %w", err)
	}
	if v, ok := pf.Lookup("schema_version"); ok {
		if v != schemaVersion {
			logger.Warn("Parquet metadata file has unexpected schema version; data may not load correctly",
				"expected", schemaVersion, "found", v)
		}
	} else {
		logger.Warn("Parquet metadata file missing schema_version in footer metadata")
	}

	mem := NewMemSeriesMetadata()

	if len(ropts.namespaceFilter) > 0 {
		nsColIdx := lookupColumnIndex(pf.Schema(), "namespace")
		var allRows []metadataRow
		for _, rg := range pf.RowGroups() {
			if nsColIdx >= 0 {
				if ns, ok := rowGroupSingleNamespace(rg, nsColIdx); ok {
					if _, match := ropts.namespaceFilter[ns]; !match {
						continue
					}
				}
			}

			rows, err := readRowGroup[metadataRow](rg)
			if err != nil {
				return nil, fmt.Errorf("read filtered row group: %w", err)
			}
			allRows = append(allRows, rows...)
		}
		denormalizeRows(logger, allRows, mem, ropts.refResolver)
	} else {
		rows, err := parquet.Read[metadataRow](r, size)
		if err != nil {
			return nil, fmt.Errorf("read parquet rows: %w", err)
		}
		denormalizeRows(logger, rows, mem, ropts.refResolver)
	}

	return &parquetReader{mem: mem}, nil
}

// lookupColumnIndex returns the index of the named column in the schema, or -1.
func lookupColumnIndex(schema *parquet.Schema, name string) int {
	for i, col := range schema.Columns() {
		if len(col) == 1 && col[0] == name {
			return i
		}
	}
	return -1
}

// rowGroupSingleNamespace checks whether a row group contains a single namespace.
func rowGroupSingleNamespace(rg parquet.RowGroup, nsColIdx int) (string, bool) {
	cc := rg.ColumnChunks()[nsColIdx]
	idx, err := cc.ColumnIndex()
	if err != nil || idx.NumPages() == 0 {
		return "", false
	}
	minVal := string(idx.MinValue(0).ByteArray())
	maxVal := string(idx.MaxValue(0).ByteArray())
	if minVal != maxVal {
		return "", false
	}
	for p := 1; p < idx.NumPages(); p++ {
		if string(idx.MinValue(p).ByteArray()) != minVal || string(idx.MaxValue(p).ByteArray()) != minVal {
			return "", false
		}
	}
	return minVal, true
}

// readRowGroup reads all rows from a single row group into a typed slice.
func readRowGroup[T any](rg parquet.RowGroup) ([]T, error) {
	n := rg.NumRows()
	rows := make([]T, n)
	reader := parquet.NewGenericRowGroupReader[T](rg)
	_, err := reader.Read(rows)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return rows, nil
}

// parseResourceContent converts a resource_table row into a ResourceVersion.
func parseResourceContent(logger *slog.Logger, row *metadataRow) *ResourceVersion {
	identifying := make(map[string]string, len(row.IdentifyingAttrs))
	for _, attr := range row.IdentifyingAttrs {
		identifying[attr.Key] = attr.Value
	}
	descriptive := make(map[string]string, len(row.DescriptiveAttrs))
	for _, attr := range row.DescriptiveAttrs {
		descriptive[attr.Key] = attr.Value
	}

	var entities []*Entity
	for _, entityRow := range row.Entities {
		entityID := make(map[string]string, len(entityRow.ID))
		for _, attr := range entityRow.ID {
			entityID[attr.Key] = attr.Value
		}
		entityDesc := make(map[string]string, len(entityRow.Description))
		for _, attr := range entityRow.Description {
			entityDesc[attr.Key] = attr.Value
		}
		entityType := entityRow.Type
		if entityType == "" {
			entityType = EntityTypeResource
		}
		e := &Entity{
			Type:        entityType,
			ID:          entityID,
			Description: entityDesc,
		}
		if err := e.Validate(); err != nil {
			logger.Warn("Skipping invalid entity during parquet read", "err", err, "type", entityRow.Type)
			continue
		}
		entities = append(entities, e)
	}
	slices.SortFunc(entities, func(a, b *Entity) int {
		return strings.Compare(a.Type, b.Type)
	})

	return &ResourceVersion{
		Identifying: identifying,
		Descriptive: descriptive,
		Entities:    entities,
	}
}

// parseScopeContent converts a scope_table row into a ScopeVersion.
func parseScopeContent(row *metadataRow) *ScopeVersion {
	attrs := make(map[string]string, len(row.ScopeAttrs))
	for _, attr := range row.ScopeAttrs {
		attrs[attr.Key] = attr.Value
	}
	return &ScopeVersion{
		Name:      row.ScopeName,
		Version:   row.ScopeVersionStr,
		SchemaURL: row.SchemaURL,
		Attrs:     attrs,
	}
}

// --- parquetReader type-safe accessors ---

func (r *parquetReader) GetResource(labelsHash uint64) (*ResourceVersion, bool) {
	return r.mem.GetResource(labelsHash)
}

func (r *parquetReader) GetVersionedResource(labelsHash uint64) (*VersionedResource, bool) {
	return r.mem.GetVersionedResource(labelsHash)
}

func (r *parquetReader) GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool) {
	return r.mem.GetResourceAt(labelsHash, timestamp)
}

func (r *parquetReader) IterResources(ctx context.Context, f func(labelsHash uint64, resource *ResourceVersion) error) error {
	return r.mem.IterResources(ctx, f)
}

func (r *parquetReader) IterVersionedResources(ctx context.Context, f func(labelsHash uint64, resources *VersionedResource) error) error {
	return r.mem.IterVersionedResources(ctx, f)
}

func (r *parquetReader) IterVersionedResourcesFlatInline(ctx context.Context, f func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error) error {
	return r.mem.IterVersionedResourcesFlatInline(ctx, f)
}

func (r *parquetReader) TotalResources() uint64 {
	return r.mem.TotalResources()
}

func (r *parquetReader) TotalResourceVersions() uint64 {
	return r.mem.TotalResourceVersions()
}

func (r *parquetReader) GetVersionedScope(labelsHash uint64) (*VersionedScope, bool) {
	return r.mem.GetVersionedScope(labelsHash)
}

func (r *parquetReader) IterVersionedScopes(ctx context.Context, f func(labelsHash uint64, scopes *VersionedScope) error) error {
	return r.mem.IterVersionedScopes(ctx, f)
}

func (r *parquetReader) IterVersionedScopesFlatInline(ctx context.Context, f func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error) error {
	return r.mem.IterVersionedScopesFlatInline(ctx, f)
}

func (r *parquetReader) TotalScopes() uint64 {
	return r.mem.TotalScopes()
}

func (r *parquetReader) TotalScopeVersions() uint64 {
	return r.mem.TotalScopeVersions()
}

func (r *parquetReader) IterKind(ctx context.Context, id KindID, f func(labelsHash uint64, versioned any) error) error {
	return r.mem.IterKind(ctx, id, f)
}

func (r *parquetReader) IterHashes(ctx context.Context, id KindID, f func(labelsHash uint64) error) error {
	return r.mem.IterHashes(ctx, id, f)
}

func (r *parquetReader) LabelsForHash(labelsHash uint64) (labels.Labels, bool) {
	return r.mem.LabelsForHash(labelsHash)
}

func (r *parquetReader) SetLabels(labelsHash uint64, lset labels.Labels) {
	r.mem.SetLabels(labelsHash, lset)
}

func (r *parquetReader) KindLen(id KindID) int {
	return r.mem.KindLen(id)
}

func (r *parquetReader) LookupResourceAttr(key, value string) []uint64 {
	return r.mem.LookupResourceAttr(key, value)
}

// Close releases resources associated with the reader.
func (r *parquetReader) Close() error {
	r.closeOnce.Do(func() {
		if r.closer != nil {
			r.closeErr = r.closer.Close()
		}
	})
	return r.closeErr
}

// sortAttrEntries sorts attribute entries by key for deterministic Parquet output.
func sortAttrEntries(entries []EntityAttributeEntry) {
	slices.SortFunc(entries, func(a, b EntityAttributeEntry) int {
		return cmp.Compare(a.Key, b.Key)
	})
}

// sortMetadataRows sorts rows for compression: group by namespace, then by
// series_ref, content_hash, MinTime.
func sortMetadataRows(rows []metadataRow) {
	slices.SortFunc(rows, func(a, b metadataRow) int {
		if c := strings.Compare(a.Namespace, b.Namespace); c != 0 {
			return c
		}
		if c := cmp.Compare(a.SeriesRef, b.SeriesRef); c != 0 {
			return c
		}
		if c := cmp.Compare(a.ContentHash, b.ContentHash); c != 0 {
			return c
		}
		return cmp.Compare(a.MinTime, b.MinTime)
	})
}

// WriteFile atomically writes series metadata to a Parquet file.
func WriteFile(logger *slog.Logger, dir string, mr Reader) (int64, error) {
	return WriteFileWithOptions(logger, dir, mr, WriterOptions{})
}

// WriteFileWithOptions writes series metadata using the kind registry for dispatch.
func WriteFileWithOptions(logger *slog.Logger, dir string, mr Reader, opts WriterOptions) (int64, error) {
	path := filepath.Join(dir, SeriesMetadataFilename)
	tmp := path + ".tmp"

	// Create temp file eagerly so we can stream rows incrementally.
	f, err := os.Create(tmp)
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				logger.Error("close temp file", "err", err.Error())
			}
		}
		if tmp != "" {
			if err := os.RemoveAll(tmp); err != nil {
				logger.Error("remove temp file", "err", err.Error())
			}
		}
	}()

	// Build writer options (counts are added via SetKeyValueMetadata after streaming).
	writerOpts := []parquet.WriterOption{
		parquet.Compression(&zstd.Codec{Level: zstd.SpeedBetterCompression}),
		parquet.KeyValueMetadata("schema_version", schemaVersion),
		parquet.KeyValueMetadata("row_group_layout", "namespace_partitioned"),
	}
	if opts.BloomFilterFormat == BloomFilterParquetNative {
		writerOpts = append(writerOpts,
			parquet.BloomFilters(
				parquet.SplitBlockFilter(10, "series_ref"),
				parquet.SplitBlockFilter(10, "content_hash"),
				parquet.SplitBlockFilter(10, "attr_key"),
				parquet.SplitBlockFilter(10, "attr_value"),
			),
		)
	}

	writer := parquet.NewGenericWriter[metadataRow](f, writerOpts...)
	totalRows := 0
	metadataCounts := make(map[string]int)

	// Stream rows per-kind using typed iteration to avoid interface boxing.
	// Each block builds table + mapping rows, writes them, then releases.

	// --- Resources ---
	{
		contentTable := make(map[uint64]metadataRow)
		var mappingRows []metadataRow
		var indexRows []metadataRow
		var keysBuf []string
		var seen map[uint64]struct{}
		if opts.EnableInvertedIndex {
			seen = make(map[uint64]struct{})
		}

		err := iterResourcesFlatInlineWithContentHash(context.Background(), mr, func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool, cachedContentHash uint64) error {
			if opts.HashFilter != nil && !opts.HashFilter(labelsHash) {
				return nil
			}
			seriesRef := labelsHash
			if opts.RefResolver != nil {
				ref, ok := opts.RefResolver(labelsHash)
				if !ok {
					return nil
				}
				seriesRef = ref
			}
			if seen != nil {
				clear(seen)
			}
			for i, rv := range versions {
				var contentHash uint64
				if isInline && cachedContentHash != 0 {
					contentHash = cachedContentHash
				} else {
					contentHash, keysBuf = hashResourceContentReusable(rv, keysBuf)
				}
				if _, exists := contentTable[contentHash]; !exists {
					contentTable[contentHash] = buildResourceTableRow(contentHash, rv)
				}
				minTime, maxTime := rv.MinTime, rv.MaxTime
				if isInline {
					minTime, maxTime = inlineMinTime, inlineMaxTime
				}
				mappingRows = append(mappingRows, metadataRow{
					Namespace:   NamespaceResourceMapping,
					SeriesRef:   seriesRef,
					ContentHash: contentHash,
					MinTime:     minTime,
					MaxTime:     maxTime,
				})
				// Build inverted index rows inline (Fix 1: avoids second iteration).
				if seen != nil {
					_ = i // suppress unused warning
					for k, v := range rv.Identifying {
						ch := attrKeyValueHash(k, v)
						if _, exists := seen[ch]; !exists {
							seen[ch] = struct{}{}
							indexRows = append(indexRows, metadataRow{
								Namespace:   NamespaceResourceAttrIndex,
								SeriesRef:   seriesRef,
								ContentHash: ch,
								AttrKey:     k,
								AttrValue:   v,
								IdentifyingAttrs: []EntityAttributeEntry{
									{Key: k, Value: v},
								},
							})
						}
					}
					for k, v := range rv.Descriptive {
						if _, ok := opts.IndexedResourceAttrs[k]; !ok {
							continue
						}
						ch := attrKeyValueHash(k, v)
						if _, exists := seen[ch]; !exists {
							seen[ch] = struct{}{}
							indexRows = append(indexRows, metadataRow{
								Namespace:   NamespaceResourceAttrIndex,
								SeriesRef:   seriesRef,
								ContentHash: ch,
								AttrKey:     k,
								AttrValue:   v,
								IdentifyingAttrs: []EntityAttributeEntry{
									{Key: k, Value: v},
								},
							})
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("iterate %s: %w", KindResource, err)
		}

		tableRows := make([]metadataRow, 0, len(contentTable))
		for _, row := range contentTable {
			tableRows = append(tableRows, row)
		}
		clear(contentTable)

		sortMetadataRows(tableRows)
		sortMetadataRows(mappingRows)

		metadataCounts[string(KindResource)+"_table_count"] = len(tableRows)
		metadataCounts[string(KindResource)+"_mapping_count"] = len(mappingRows)
		totalRows += len(tableRows) + len(mappingRows)

		if err := writeNamespaceRows(writer, tableRows, opts.MaxRowsPerRowGroup); err != nil {
			return 0, fmt.Errorf("write %s table rows: %w", KindResource, err)
		}
		if err := writeNamespaceRows(writer, mappingRows, opts.MaxRowsPerRowGroup); err != nil {
			return 0, fmt.Errorf("write %s mapping rows: %w", KindResource, err)
		}

		// Write inverted index rows built inline during the resource pass.
		if len(indexRows) > 0 {
			sortMetadataRows(indexRows)
			metadataCounts["resource_attr_index_count"] = len(indexRows)
			totalRows += len(indexRows)
			if err := writeNamespaceRows(writer, indexRows, opts.MaxRowsPerRowGroup); err != nil {
				return 0, fmt.Errorf("write resource attr index rows: %w", err)
			}
		}
	}

	// --- Scopes ---
	{
		contentTable := make(map[uint64]metadataRow)
		var mappingRows []metadataRow
		var keysBuf []string

		err := iterScopesFlatInlineWithContentHash(context.Background(), mr, func(labelsHash uint64, versions []*ScopeVersion, inlineMinTime, inlineMaxTime int64, isInline bool, cachedContentHash uint64) error {
			if opts.HashFilter != nil && !opts.HashFilter(labelsHash) {
				return nil
			}
			seriesRef := labelsHash
			if opts.RefResolver != nil {
				ref, ok := opts.RefResolver(labelsHash)
				if !ok {
					return nil
				}
				seriesRef = ref
			}
			for i, sv := range versions {
				var contentHash uint64
				if isInline && cachedContentHash != 0 {
					contentHash = cachedContentHash
				} else {
					contentHash, keysBuf = hashScopeContentReusable(sv, keysBuf)
				}
				_ = i // suppress unused warning
				if _, exists := contentTable[contentHash]; !exists {
					contentTable[contentHash] = buildScopeTableRow(contentHash, sv)
				}
				minTime, maxTime := sv.MinTime, sv.MaxTime
				if isInline {
					minTime, maxTime = inlineMinTime, inlineMaxTime
				}
				mappingRows = append(mappingRows, metadataRow{
					Namespace:   NamespaceScopeMapping,
					SeriesRef:   seriesRef,
					ContentHash: contentHash,
					MinTime:     minTime,
					MaxTime:     maxTime,
				})
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("iterate %s: %w", KindScope, err)
		}

		tableRows := make([]metadataRow, 0, len(contentTable))
		for _, row := range contentTable {
			tableRows = append(tableRows, row)
		}
		clear(contentTable)

		sortMetadataRows(tableRows)
		sortMetadataRows(mappingRows)

		metadataCounts[string(KindScope)+"_table_count"] = len(tableRows)
		metadataCounts[string(KindScope)+"_mapping_count"] = len(mappingRows)
		totalRows += len(tableRows) + len(mappingRows)

		if err := writeNamespaceRows(writer, tableRows, opts.MaxRowsPerRowGroup); err != nil {
			return 0, fmt.Errorf("write %s table rows: %w", KindScope, err)
		}
		if err := writeNamespaceRows(writer, mappingRows, opts.MaxRowsPerRowGroup); err != nil {
			return 0, fmt.Errorf("write %s mapping rows: %w", KindScope, err)
		}
	}

	if totalRows == 0 {
		// No rows written — remove the temp file and return.
		if err := writer.Close(); err != nil {
			return 0, fmt.Errorf("close empty parquet writer: %w", err)
		}
		return 0, nil
	}

	// Set metadata counts in Parquet footer (written on Close).
	for k, v := range metadataCounts {
		writer.SetKeyValueMetadata(k, strconv.Itoa(v))
	}

	if err := writer.Close(); err != nil {
		return 0, fmt.Errorf("close parquet writer: %w", err)
	}

	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("sync file: %w", err)
	}

	stat, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat file: %w", err)
	}
	size := stat.Size()

	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("close file: %w", err)
	}
	f = nil

	if err := fileutil.Replace(tmp, path); err != nil {
		return 0, fmt.Errorf("rename temp file: %w", err)
	}
	tmp = ""

	logArgs := []any{
		"resource_table", metadataCounts["resource_table_count"],
		"resource_mappings", metadataCounts["resource_mapping_count"],
		"scope_table", metadataCounts["scope_table_count"],
		"scope_mappings", metadataCounts["scope_mapping_count"],
		"size", size,
	}
	if cnt, ok := metadataCounts["resource_attr_index_count"]; ok {
		logArgs = append(logArgs, "resource_attr_index", cnt)
	}
	logger.Info("Series metadata written", logArgs...)

	// Populate write stats if requested.
	if opts.WriteStats != nil {
		opts.WriteStats.NamespaceRowCounts = metadataCounts
	}

	return size, nil
}

// buildResourceTableRow converts a ResourceVersion into a content-addressed table row.
func buildResourceTableRow(contentHash uint64, rv *ResourceVersion) metadataRow {
	idAttrs := make([]EntityAttributeEntry, 0, len(rv.Identifying))
	for k, v := range rv.Identifying {
		idAttrs = append(idAttrs, EntityAttributeEntry{Key: k, Value: v})
	}
	sortAttrEntries(idAttrs)

	descAttrs := make([]EntityAttributeEntry, 0, len(rv.Descriptive))
	for k, v := range rv.Descriptive {
		descAttrs = append(descAttrs, EntityAttributeEntry{Key: k, Value: v})
	}
	sortAttrEntries(descAttrs)

	entityRows := make([]EntityRow, 0, len(rv.Entities))
	for _, entity := range rv.Entities {
		entityIDAttrs := make([]EntityAttributeEntry, 0, len(entity.ID))
		for k, v := range entity.ID {
			entityIDAttrs = append(entityIDAttrs, EntityAttributeEntry{Key: k, Value: v})
		}
		sortAttrEntries(entityIDAttrs)

		entityDescAttrs := make([]EntityAttributeEntry, 0, len(entity.Description))
		for k, v := range entity.Description {
			entityDescAttrs = append(entityDescAttrs, EntityAttributeEntry{Key: k, Value: v})
		}
		sortAttrEntries(entityDescAttrs)

		entityRows = append(entityRows, EntityRow{
			Type:        entity.Type,
			ID:          entityIDAttrs,
			Description: entityDescAttrs,
		})
	}

	return metadataRow{
		Namespace:        NamespaceResourceTable,
		ContentHash:      contentHash,
		IdentifyingAttrs: idAttrs,
		DescriptiveAttrs: descAttrs,
		Entities:         entityRows,
	}
}

// buildResourceAttrIndexRows builds inverted index rows for Parquet from all
// resource versions. Each unique (key, value, seriesRef) tuple produces one row.
// Identifying attributes are always indexed. Descriptive attributes are only
// indexed if their key is in indexedResourceAttrs.
//
// Uses a per-series seen set (keyed by contentHash only, since seriesRef is
// constant within a single callback invocation) instead of a global seen map,
// so memory scales with max-attrs-per-series rather than total-attrs×series.
func buildResourceAttrIndexRows(mr Reader, refResolver func(labelsHash uint64) (uint64, bool), indexedResourceAttrs map[string]struct{}, hashFilter func(uint64) bool) []metadataRow {
	var rows []metadataRow
	// Per-series dedup: seriesRef is constant within a callback invocation,
	// so we only key by the attr hash. Hoisted outside the closure and
	// clear()ed per series to avoid per-callback map allocation.
	seen := make(map[uint64]struct{})

	_ = iterResourcesFlatInline(context.Background(), mr, func(labelsHash uint64, versions []*ResourceVersion, _, _ int64, _ bool) error {
		if hashFilter != nil && !hashFilter(labelsHash) {
			return nil
		}
		seriesRef := labelsHash
		if refResolver != nil {
			ref, ok := refResolver(labelsHash)
			if !ok {
				return nil
			}
			seriesRef = ref
		}

		clear(seen)

		addEntry := func(k, v string) {
			ch := attrKeyValueHash(k, v)
			if _, exists := seen[ch]; exists {
				return
			}
			seen[ch] = struct{}{}
			rows = append(rows, metadataRow{
				Namespace:   NamespaceResourceAttrIndex,
				SeriesRef:   seriesRef,
				ContentHash: ch,
				AttrKey:     k,
				AttrValue:   v,
				IdentifyingAttrs: []EntityAttributeEntry{
					{Key: k, Value: v},
				},
			})
		}

		for _, rv := range versions {
			for k, v := range rv.Identifying {
				addEntry(k, v)
			}
			for k, v := range rv.Descriptive {
				if _, ok := indexedResourceAttrs[k]; !ok {
					continue
				}
				addEntry(k, v)
			}
		}
		return nil
	})

	return rows
}

// attrKeyValueHash computes xxhash("key\x00value") for bloom filter skipability.
func attrKeyValueHash(key, value string) uint64 {
	var h xxhash.Digest
	_, _ = h.WriteString(key)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(value)
	return h.Sum64()
}

// ReadSeriesMetadata reads series metadata from a Parquet file in the given directory.
func ReadSeriesMetadata(logger *slog.Logger, dir string, opts ...ReaderOption) (Reader, int64, error) {
	path := filepath.Join(dir, SeriesMetadataFilename)

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return NewMemSeriesMetadata(), 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("open metadata file: %w", err)
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("stat metadata file: %w", err)
	}

	reader, err := newParquetReaderFromReaderAt(logger, f, stat.Size(), opts...)
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("create parquet reader: %w", err)
	}
	reader.closer = f

	return reader, stat.Size(), nil
}
