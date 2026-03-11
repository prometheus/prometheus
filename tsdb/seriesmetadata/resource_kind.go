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
	"cmp"
	"context"
	"log/slog"
	"maps"
	"slices"

	"github.com/cespare/xxhash/v2"
)

func init() {
	RegisterKind(&resourceKindDescriptor{})
}

// resourceOps implements KindOps for *ResourceVersion.
type resourceOps struct{}

func (resourceOps) Equal(a, b *ResourceVersion) bool         { return ResourceVersionsEqual(a, b) }
func (resourceOps) Copy(v *ResourceVersion) *ResourceVersion { return copyResourceVersion(v) }

func (resourceOps) ContentHash(v *ResourceVersion) uint64 { return hashResourceContent(v) }
func (resourceOps) ThinCopy(canonical, v *ResourceVersion) *ResourceVersion {
	return &ResourceVersion{
		Identifying: canonical.Identifying,
		Descriptive: canonical.Descriptive,
		Entities:    canonical.Entities,
		MinTime:     v.MinTime,
		MaxTime:     v.MaxTime,
	}
}

func (resourceOps) IsInterned(canonical, v *ResourceVersion) bool {
	return mapSameUnderlying(canonical.Identifying, v.Identifying)
}

// ResourceOps is the shared KindOps instance for resources.
var ResourceOps KindOps[*ResourceVersion] = resourceOps{}

// hashResourceContent computes a deterministic xxhash for a ResourceVersion's content.
// The hash covers identifying attrs, descriptive attrs, and all entities.
// It does NOT include MinTime/MaxTime since those are per-mapping, not per-content.
func hashResourceContent(rv *ResourceVersion) uint64 {
	hash, _ := hashResourceContentReusable(rv, nil)
	return hash
}

// hashResourceContentReusable is like hashResourceContent but accepts and returns
// a reusable keys buffer to avoid per-call []string allocations on the write path.
func hashResourceContentReusable(rv *ResourceVersion, keysBuf []string) (uint64, []string) {
	var h xxhash.Digest

	keysBuf = hashAttrs(&h, rv.Identifying, keysBuf)
	_, _ = h.Write([]byte{1}) // section separator
	keysBuf = hashAttrs(&h, rv.Descriptive, keysBuf)
	_, _ = h.Write([]byte{1})

	// Entities must be sorted by Type (enforced by NewResourceVersion and parseResourceContent).
	for _, e := range rv.Entities {
		_, _ = h.WriteString(e.Type)
		_, _ = h.Write([]byte{0})
		keysBuf = hashAttrs(&h, e.ID, keysBuf)
		_, _ = h.Write([]byte{1})
		keysBuf = hashAttrs(&h, e.Description, keysBuf)
		_, _ = h.Write([]byte{1})
	}

	return h.Sum64(), keysBuf
}

// resourceKindDescriptor implements KindDescriptor for OTel resources.
type resourceKindDescriptor struct{}

func (*resourceKindDescriptor) ID() KindID                   { return KindResource }
func (*resourceKindDescriptor) WALRecordType() WALRecordType { return WALResourceUpdate }
func (*resourceKindDescriptor) TableNamespace() string       { return NamespaceResourceTable }
func (*resourceKindDescriptor) MappingNamespace() string     { return NamespaceResourceMapping }

// DecodeWAL and EncodeWAL are implemented in tsdb/head_wal_kind.go to avoid
// importing tsdb/record from this package (which would create an import cycle).
// The descriptor delegates to pluggable functions set during init.
func (*resourceKindDescriptor) DecodeWAL(rec []byte, into any) (any, error) {
	return ResourceDecodeWAL(rec, into)
}

func (*resourceKindDescriptor) EncodeWAL(records any, buf []byte) []byte {
	return ResourceEncodeWAL(records, buf)
}

// ResourceDecodeWAL is set by the tsdb package to break the import cycle.
var ResourceDecodeWAL func(rec []byte, into any) (any, error)

// ResourceEncodeWAL is set by the tsdb package to break the import cycle.
var ResourceEncodeWAL func(records any, buf []byte) []byte

func (*resourceKindDescriptor) ParseTableRow(logger *slog.Logger, row *metadataRow) any {
	return parseResourceContent(logger, row)
}

func (*resourceKindDescriptor) BuildTableRow(contentHash uint64, version any) metadataRow {
	return buildResourceTableRow(contentHash, version.(*ResourceVersion))
}

func (*resourceKindDescriptor) ContentHash(version any) uint64 {
	return hashResourceContent(version.(*ResourceVersion))
}

func (*resourceKindDescriptor) CommitToSeries(series, walRecord any) {
	CommitResourceDirect(series.(kindMetaAccessor), walRecord.(ResourceCommitData))
}

// ResourceEntityData is a lightweight struct for passing entity data
// from WAL records without importing tsdb/record.
type ResourceEntityData struct {
	Type        string
	ID          map[string]string
	Description map[string]string
}

// ResourceCommitData carries resource WAL record data without importing tsdb/record.
type ResourceCommitData struct {
	Identifying map[string]string
	Descriptive map[string]string
	Entities    []ResourceEntityData
	MinTime     int64
	MaxTime     int64
	// Owned indicates the caller guarantees exclusive ownership of the maps.
	// When true, buildResourceVersion takes the maps directly instead of cloning.
	// Set by the ingester push path where maps are freshly allocated.
	Owned bool
}

// CommitResourceDirect is the hot-path commit for resources.
// It constructs the ResourceVersion with exactly one deep copy of each map
// (from the caller's buffers into stored metadata) and takes ownership of
// the result — no further copies via AddOrExtend or copyResourceVersion.
// Called directly from headAppenderBase.commitResources and from
// CommitToSeries (cold path, WAL replay).
func CommitResourceDirect(accessor kindMetaAccessor, rcd ResourceCommitData) {
	entities := make([]*Entity, len(rcd.Entities))
	for j, e := range rcd.Entities {
		entityType := e.Type
		if entityType == "" {
			entityType = EntityTypeResource
		}
		entities[j] = &Entity{
			Type:        entityType,
			ID:          maps.Clone(e.ID),
			Description: maps.Clone(e.Description),
		}
	}
	slices.SortFunc(entities, func(a, b *Entity) int {
		return cmp.Compare(a.Type, b.Type)
	})

	rv := &ResourceVersion{
		Identifying: maps.Clone(rcd.Identifying),
		Descriptive: maps.Clone(rcd.Descriptive),
		Entities:    entities,
		MinTime:     rcd.MinTime,
		MaxTime:     rcd.MaxTime,
	}

	existing, _ := accessor.GetKindMeta(KindResource)
	if existing == nil {
		accessor.SetKindMeta(KindResource, &Versioned[*ResourceVersion]{
			Versions: []*ResourceVersion{rv},
		})
	} else {
		vr := existing.(*Versioned[*ResourceVersion])
		if len(vr.Versions) > 0 && ResourceOps.Equal(vr.Versions[len(vr.Versions)-1], rv) {
			vr.Versions[len(vr.Versions)-1].UpdateTimeRange(rv.MinTime, rv.MaxTime)
		} else {
			vr.Versions = append(vr.Versions, rv)
		}
	}
}

// hashResourceCommitData computes the content hash from raw ResourceCommitData
// without cloning any maps. The hash is identical to hashResourceContent for
// equivalent data, including the entity default-type normalization.
func hashResourceCommitData(rcd ResourceCommitData) uint64 {
	hash, _ := hashResourceCommitDataReusable(rcd, nil)
	return hash
}

// hashResourceCommitDataReusable is like hashResourceCommitData but accepts and
// returns a reusable keys buffer to avoid per-call []string allocations.
func hashResourceCommitDataReusable(rcd ResourceCommitData, keysBuf []string) (uint64, []string) {
	var h xxhash.Digest

	keysBuf = hashAttrs(&h, rcd.Identifying, keysBuf)
	_, _ = h.Write([]byte{1})
	keysBuf = hashAttrs(&h, rcd.Descriptive, keysBuf)
	_, _ = h.Write([]byte{1})

	// Sort entities by type for deterministic hashing (matching hashResourceContent).
	// Sort in-place since we'd sort anyway in the full-alloc path.
	slices.SortFunc(rcd.Entities, func(a, b ResourceEntityData) int {
		at, bt := a.Type, b.Type
		if at == "" {
			at = EntityTypeResource
		}
		if bt == "" {
			bt = EntityTypeResource
		}
		return cmp.Compare(at, bt)
	})
	for _, e := range rcd.Entities {
		entityType := e.Type
		if entityType == "" {
			entityType = EntityTypeResource
		}
		_, _ = h.WriteString(entityType)
		_, _ = h.Write([]byte{0})
		keysBuf = hashAttrs(&h, e.ID, keysBuf)
		_, _ = h.Write([]byte{1})
		keysBuf = hashAttrs(&h, e.Description, keysBuf)
		_, _ = h.Write([]byte{1})
	}

	return h.Sum64(), keysBuf
}

// CommitResourceToStore builds a ResourceVersion from ResourceCommitData and
// commits it directly to the MemStore, bypassing per-series storage entirely.
// Returns contentChanged=false when only the time range was extended (no WAL
// write or attr index update needed — the >99% hot path).
// Returns contentChanged=true with old/cur materialized when content changed.
//
// Uses InsertVersion to avoid deep-copying maps when a canonical already exists
// in the content dedup table (common during WAL replay where many series share
// the same resource). The buildFull callback is only invoked when no canonical
// exists yet.
func CommitResourceToStore(store *MemStore[*ResourceVersion], labelsHash uint64, rcd ResourceCommitData) (contentChanged bool, old, cur *VersionedResource) {
	// Compute content hash from raw data (no allocations needed).
	// hashResourceCommitData also sorts rcd.Entities in-place.
	contentHash := hashResourceCommitData(rcd)

	return store.InsertVersion(labelsHash, contentHash, rcd.MinTime, rcd.MaxTime, func() *ResourceVersion {
		return buildResourceVersion(rcd)
	})
}

// CommitResourceToStoreReusable is like CommitResourceToStore but accepts and
// returns a reusable keys buffer for hash computation, avoiding per-call
// []string allocations on the ingestion hot path.
func CommitResourceToStoreReusable(store *MemStore[*ResourceVersion], labelsHash uint64, rcd ResourceCommitData, keysBuf []string) (contentChanged bool, old, cur *VersionedResource, updatedKeysBuf []string) {
	contentHash, keysBuf := hashResourceCommitDataReusable(rcd, keysBuf)
	contentChanged, old, cur = store.InsertVersion(labelsHash, contentHash, rcd.MinTime, rcd.MaxTime, func() *ResourceVersion {
		return buildResourceVersion(rcd)
	})
	return contentChanged, old, cur, keysBuf
}

// CommitResourceToStoreReusableWithRef is like CommitResourceToStoreReusable but
// also sets the series ref on the MemStore entry in the same critical section,
// avoiding a separate SetSeriesRef call (1 fewer lock + map lookup per series).
func CommitResourceToStoreReusableWithRef(store *MemStore[*ResourceVersion], labelsHash uint64, rcd ResourceCommitData, seriesRef uint64, keysBuf []string) (contentChanged bool, old, cur *VersionedResource, updatedKeysBuf []string) {
	contentHash, keysBuf := hashResourceCommitDataReusable(rcd, keysBuf)
	contentChanged, old, cur = store.InsertVersionWithRef(labelsHash, contentHash, rcd.MinTime, rcd.MaxTime, seriesRef, func() *ResourceVersion {
		return buildResourceVersion(rcd)
	})
	return contentChanged, old, cur, keysBuf
}

// buildResourceVersion allocates a ResourceVersion from commit data.
// When rcd.Owned is true, maps are taken directly (zero-copy).
// Otherwise, all maps are deep-copied.
func buildResourceVersion(rcd ResourceCommitData) *ResourceVersion {
	cloneMap := maps.Clone[map[string]string]
	if rcd.Owned {
		cloneMap = func(m map[string]string) map[string]string { return m }
	}

	entities := make([]*Entity, len(rcd.Entities))
	for j, e := range rcd.Entities {
		entityType := e.Type
		if entityType == "" {
			entityType = EntityTypeResource
		}
		entities[j] = &Entity{
			Type:        entityType,
			ID:          cloneMap(e.ID),
			Description: cloneMap(e.Description),
		}
	}
	// Entities are already sorted by hashResourceCommitData.
	return &ResourceVersion{
		Identifying: cloneMap(rcd.Identifying),
		Descriptive: cloneMap(rcd.Descriptive),
		Entities:    entities,
		MinTime:     rcd.MinTime,
		MaxTime:     rcd.MaxTime,
	}
}

// CollectResourceDirect is the hot-path equivalent of CollectFromSeries
// for resources, avoiding interface{} boxing on the return path.
func CollectResourceDirect(accessor kindMetaAccessor) (*VersionedResource, bool) {
	v, ok := accessor.GetKindMeta(KindResource)
	if !ok || v == nil {
		return nil, false
	}
	return v.(*Versioned[*ResourceVersion]), true
}

func (*resourceKindDescriptor) CollectFromSeries(series any) (any, bool) {
	accessor := series.(kindMetaAccessor)
	return accessor.GetKindMeta(KindResource)
}

func (*resourceKindDescriptor) CopyVersioned(v any) any {
	return v.(*Versioned[*ResourceVersion]).Copy(ResourceOps)
}

func (*resourceKindDescriptor) SetOnSeries(series, versioned any) {
	accessor := series.(kindMetaAccessor)
	accessor.SetKindMeta(KindResource, versioned)
}

func (*resourceKindDescriptor) NewStore() any {
	return NewMemStore[*ResourceVersion](ResourceOps)
}

func (*resourceKindDescriptor) SetVersioned(store any, labelsHash uint64, versioned any) {
	store.(*MemStore[*ResourceVersion]).SetVersioned(labelsHash, versioned.(*Versioned[*ResourceVersion]))
}

func (*resourceKindDescriptor) IterVersioned(ctx context.Context, store any, f func(labelsHash uint64, versioned any) error) error {
	return store.(*MemStore[*ResourceVersion]).IterVersionedFlatInline(ctx, func(labelsHash uint64, versions []*ResourceVersion, inlineMinTime, inlineMaxTime int64, isInline bool) error {
		if isInline && len(versions) == 1 {
			thin := resourceOps{}.ThinCopy(versions[0], versions[0])
			thin.MinTime = inlineMinTime
			thin.MaxTime = inlineMaxTime
			return f(labelsHash, &Versioned[*ResourceVersion]{Versions: []*ResourceVersion{thin}})
		}
		return f(labelsHash, &Versioned[*ResourceVersion]{Versions: versions})
	})
}

func (*resourceKindDescriptor) StoreLen(store any) int {
	return store.(*MemStore[*ResourceVersion]).Len()
}

func (*resourceKindDescriptor) DenormalizeIntoStore(store any, labelsHash uint64, versions []VersionWithTime) {
	typed := make([]*ResourceVersion, len(versions))
	for i, vt := range versions {
		cp := copyResourceVersion(vt.Version.(*ResourceVersion))
		cp.MinTime = vt.MinTime
		cp.MaxTime = vt.MaxTime
		typed[i] = cp
	}
	slices.SortFunc(typed, func(a, b *ResourceVersion) int {
		return cmp.Compare(a.MinTime, b.MinTime)
	})
	store.(*MemStore[*ResourceVersion]).SetVersioned(labelsHash, &Versioned[*ResourceVersion]{Versions: typed})
}

func (*resourceKindDescriptor) IterateVersions(versioned any, f func(version any, minTime, maxTime int64)) {
	vr := versioned.(*Versioned[*ResourceVersion])
	for _, rv := range vr.Versions {
		f(rv, rv.MinTime, rv.MaxTime)
	}
}

func (*resourceKindDescriptor) VersionsEqual(a, b any) bool {
	return ResourceVersionsEqual(a.(*ResourceVersion), b.(*ResourceVersion))
}
