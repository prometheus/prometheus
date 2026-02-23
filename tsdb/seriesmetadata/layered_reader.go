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

	"github.com/prometheus/prometheus/model/labels"
)

// layeredMetadataReader combines a base reader (blocks-merged, cached
// indefinitely) with a top reader (head, always live). Lookups check
// top first, then base. Iteration deduplicates by labelsHash.
type layeredMetadataReader struct {
	base Reader // blocks-merged (immutable, cached)
	top  Reader // head (live, always current)
}

// NewLayeredReader creates a Reader that layers top on base.
// Close only closes base; the caller manages top's lifecycle.
func NewLayeredReader(base, top Reader) Reader {
	return &layeredMetadataReader{base: base, top: top}
}

func (r *layeredMetadataReader) Close() error {
	// Only close base (blocks). Head reader's Close is a no-op and
	// the head manages its own lifecycle.
	return r.base.Close()
}

// --- VersionedResourceReader ---

func (r *layeredMetadataReader) GetResource(labelsHash uint64) (*ResourceVersion, bool) {
	if v, ok := r.top.GetResource(labelsHash); ok {
		return v, true
	}
	return r.base.GetResource(labelsHash)
}

func (r *layeredMetadataReader) GetVersionedResource(labelsHash uint64) (*VersionedResource, bool) {
	topVR, topOK := r.top.GetVersionedResource(labelsHash)
	baseVR, baseOK := r.base.GetVersionedResource(labelsHash)
	switch {
	case topOK && baseOK:
		return MergeVersionedResources(baseVR, topVR), true
	case topOK:
		return topVR, true
	case baseOK:
		return baseVR, true
	default:
		return nil, false
	}
}

func (r *layeredMetadataReader) GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool) {
	if v, ok := r.top.GetResourceAt(labelsHash, timestamp); ok {
		return v, true
	}
	return r.base.GetResourceAt(labelsHash, timestamp)
}

func (r *layeredMetadataReader) IterResources(ctx context.Context, f func(labelsHash uint64, resource *ResourceVersion) error) error {
	seen := make(map[uint64]struct{})
	// Top (head) first — has the most current data.
	if err := r.top.IterResources(ctx, func(labelsHash uint64, resource *ResourceVersion) error {
		seen[labelsHash] = struct{}{}
		return f(labelsHash, resource)
	}); err != nil {
		return err
	}
	// Base — skip anything already seen.
	return r.base.IterResources(ctx, func(labelsHash uint64, resource *ResourceVersion) error {
		if _, ok := seen[labelsHash]; ok {
			return nil
		}
		return f(labelsHash, resource)
	})
}

func (r *layeredMetadataReader) IterVersionedResources(ctx context.Context, f func(labelsHash uint64, resources *VersionedResource) error) error {
	// Phase 1: Collect only the hash set from top (head). This uses
	// map[uint64]struct{} (~16 bytes/entry) instead of storing full
	// *VersionedResource pointers (~56+ bytes/entry), saving ~40% memory.
	topHashes := make(map[uint64]struct{})
	if err := r.top.IterVersionedResources(ctx, func(labelsHash uint64, _ *VersionedResource) error {
		topHashes[labelsHash] = struct{}{}
		return nil
	}); err != nil {
		return err
	}

	// Phase 2: Iterate base. For shared hashes, merge via point lookup on top.
	if err := r.base.IterVersionedResources(ctx, func(labelsHash uint64, baseVR *VersionedResource) error {
		if _, shared := topHashes[labelsHash]; shared {
			delete(topHashes, labelsHash)
			topVR, _ := r.top.GetVersionedResource(labelsHash)
			return f(labelsHash, MergeVersionedResources(baseVR, topVR))
		}
		return f(labelsHash, baseVR)
	}); err != nil {
		return err
	}

	// Phase 3: Emit remaining top-only entries via point lookups.
	for labelsHash := range topHashes {
		if err := ctx.Err(); err != nil {
			return err
		}
		topVR, _ := r.top.GetVersionedResource(labelsHash)
		if topVR != nil {
			if err := f(labelsHash, topVR); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *layeredMetadataReader) TotalResources() uint64 {
	// Approximate: sum may overcount shared hashes (acceptable).
	return r.base.TotalResources() + r.top.TotalResources()
}

func (r *layeredMetadataReader) TotalResourceVersions() uint64 {
	return r.base.TotalResourceVersions() + r.top.TotalResourceVersions()
}

// --- VersionedScopeReader ---

func (r *layeredMetadataReader) GetVersionedScope(labelsHash uint64) (*VersionedScope, bool) {
	topVS, topOK := r.top.GetVersionedScope(labelsHash)
	baseVS, baseOK := r.base.GetVersionedScope(labelsHash)
	switch {
	case topOK && baseOK:
		return MergeVersionedScopes(baseVS, topVS), true
	case topOK:
		return topVS, true
	case baseOK:
		return baseVS, true
	default:
		return nil, false
	}
}

func (r *layeredMetadataReader) IterVersionedScopes(ctx context.Context, f func(labelsHash uint64, scopes *VersionedScope) error) error {
	topHashes := make(map[uint64]struct{})
	if err := r.top.IterVersionedScopes(ctx, func(labelsHash uint64, _ *VersionedScope) error {
		topHashes[labelsHash] = struct{}{}
		return nil
	}); err != nil {
		return err
	}

	if err := r.base.IterVersionedScopes(ctx, func(labelsHash uint64, baseVS *VersionedScope) error {
		if _, shared := topHashes[labelsHash]; shared {
			delete(topHashes, labelsHash)
			topVS, _ := r.top.GetVersionedScope(labelsHash)
			return f(labelsHash, MergeVersionedScopes(baseVS, topVS))
		}
		return f(labelsHash, baseVS)
	}); err != nil {
		return err
	}

	for labelsHash := range topHashes {
		if err := ctx.Err(); err != nil {
			return err
		}
		topVS, _ := r.top.GetVersionedScope(labelsHash)
		if topVS != nil {
			if err := f(labelsHash, topVS); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *layeredMetadataReader) TotalScopes() uint64 {
	return r.base.TotalScopes() + r.top.TotalScopes()
}

func (r *layeredMetadataReader) TotalScopeVersions() uint64 {
	return r.base.TotalScopeVersions() + r.top.TotalScopeVersions()
}

// --- Generic Reader methods ---

func (r *layeredMetadataReader) IterKind(ctx context.Context, id KindID, f func(labelsHash uint64, versioned any) error) error {
	seen := make(map[uint64]struct{})
	if err := r.top.IterKind(ctx, id, func(labelsHash uint64, versioned any) error {
		seen[labelsHash] = struct{}{}
		return f(labelsHash, versioned)
	}); err != nil {
		return err
	}
	return r.base.IterKind(ctx, id, func(labelsHash uint64, versioned any) error {
		if _, ok := seen[labelsHash]; ok {
			return nil
		}
		return f(labelsHash, versioned)
	})
}

func (r *layeredMetadataReader) KindLen(id KindID) int {
	return r.base.KindLen(id) + r.top.KindLen(id)
}

func (r *layeredMetadataReader) LabelsForHash(labelsHash uint64) (labels.Labels, bool) {
	// Top (head) has live series — check first.
	if lset, ok := r.top.LabelsForHash(labelsHash); ok {
		return lset, true
	}
	return r.base.LabelsForHash(labelsHash)
}

func (r *layeredMetadataReader) LookupResourceAttr(key, value string) []uint64 {
	baseSet := r.base.LookupResourceAttr(key, value)
	topSet := r.top.LookupResourceAttr(key, value)

	if baseSet == nil && topSet == nil {
		return nil
	}
	if baseSet == nil {
		return topSet
	}
	if topSet == nil {
		return baseSet
	}

	return MergeSortedUnique(baseSet, topSet)
}

// MergeSortedUnique merges two sorted uint64 slices into a new sorted slice
// containing all unique values from both inputs. Both inputs must be sorted
// in ascending order.
func MergeSortedUnique(a, b []uint64) []uint64 {
	result := make([]uint64, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			result = append(result, a[i])
			i++
		case a[i] > b[j]:
			result = append(result, b[j])
			j++
		default:
			result = append(result, a[i])
			i++
			j++
		}
	}
	result = append(result, a[i:]...)
	result = append(result, b[j:]...)
	return result
}
