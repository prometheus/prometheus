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
	"sync"

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
}

// UniqueAttrNameReader is optionally implemented by Reader implementations
// that maintain a cached set of unique resource attribute names. Checking
// this via type assertion avoids O(N_series) full scans.
type UniqueAttrNameReader interface {
	UniqueResourceAttrNames() map[string]struct{}
}

// MemSeriesMetadata is an in-memory implementation of series metadata storage.
// It wraps per-kind stores accessible both generically (via IterKind/KindLen)
// and type-safely (via ResourceStore).
type MemSeriesMetadata struct {
	resourceStore *MemStore[*ResourceVersion]
	labelsMap     map[uint64]labels.Labels // labelsHash → labels.Labels

	// uniqueAttrNames is a grow-only cache of all resource attribute names
	// seen across all resource versions. Cardinality is typically tiny (<100 names).
	// Used by promql info() to discover which OTel attribute names are present.
	uniqueAttrNames   map[string]struct{}
	uniqueAttrNamesMu sync.RWMutex
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

// UniqueResourceAttrNames returns a snapshot of all resource attribute names
// that have been seen. The returned map must not be modified by the caller.
// This is O(1) — no iteration required.
func (m *MemSeriesMetadata) UniqueResourceAttrNames() map[string]struct{} {
	m.uniqueAttrNamesMu.RLock()
	defer m.uniqueAttrNamesMu.RUnlock()
	return m.uniqueAttrNames
}

// TrackResourceAttrNames adds the attribute names from cur to the grow-only
// uniqueAttrNames cache. Safe to call concurrently.
func (m *MemSeriesMetadata) TrackResourceAttrNames(cur *VersionedResource) {
	if cur == nil {
		return
	}
	m.uniqueAttrNamesMu.Lock()
	if m.uniqueAttrNames == nil {
		m.uniqueAttrNames = make(map[string]struct{})
	}
	for _, rv := range cur.Versions {
		collectAttrNames(m.uniqueAttrNames, rv)
	}
	m.uniqueAttrNamesMu.Unlock()
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

// collectAttrNames adds all attribute names from a resource version to the name set.
func collectAttrNames(names map[string]struct{}, rv *ResourceVersion) {
	for k := range rv.Identifying {
		names[k] = struct{}{}
	}
	for k := range rv.Descriptive {
		names[k] = struct{}{}
	}
}
