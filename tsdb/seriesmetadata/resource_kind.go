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

// ResourceOps is the shared KindOps instance for resources.
var ResourceOps KindOps[*ResourceVersion] = resourceOps{}

// hashResourceContent computes a deterministic xxhash for a ResourceVersion's content.
// The hash covers identifying attrs, descriptive attrs, and all entities.
// It does NOT include MinTime/MaxTime since those are per-mapping, not per-content.
func hashResourceContent(rv *ResourceVersion) uint64 {
	h := xxhash.New()

	hashAttrs(h, rv.Identifying)
	_, _ = h.Write([]byte{1}) // section separator
	hashAttrs(h, rv.Descriptive)
	_, _ = h.Write([]byte{1})

	// Entities must be sorted by Type (enforced by NewResourceVersion and parseResourceContent).
	for _, e := range rv.Entities {
		_, _ = h.WriteString(e.Type)
		_, _ = h.Write([]byte{0})
		hashAttrs(h, e.ID)
		_, _ = h.Write([]byte{1})
		hashAttrs(h, e.Description)
		_, _ = h.Write([]byte{1})
	}

	return h.Sum64()
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
	accessor := series.(kindMetaAccessor)
	// walRecord is a ResourceCommitData (set by the tsdb package).
	rcd := walRecord.(ResourceCommitData)

	metadataEntities := make([]*Entity, len(rcd.Entities))
	for j, e := range rcd.Entities {
		metadataEntities[j] = NewEntity(e.Type, e.ID, e.Description)
	}
	rv := NewResourceVersion(rcd.Identifying, rcd.Descriptive, metadataEntities, rcd.MinTime, rcd.MaxTime)

	existing, _ := accessor.GetKindMeta(KindResource)
	if existing == nil {
		accessor.SetKindMeta(KindResource, &Versioned[*ResourceVersion]{
			Versions: []*ResourceVersion{copyResourceVersion(rv)},
		})
	} else {
		vr := existing.(*Versioned[*ResourceVersion])
		vr.AddOrExtend(ResourceOps, rv)
	}
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
}

// CommitResourceDirect is the hot-path equivalent of CommitToSeries for
// resources, avoiding interface{} boxing of ResourceCommitData.
// Called directly from headAppenderBase.commitResources.
func CommitResourceDirect(accessor kindMetaAccessor, rcd ResourceCommitData) {
	metadataEntities := make([]*Entity, len(rcd.Entities))
	for j, e := range rcd.Entities {
		metadataEntities[j] = NewEntity(e.Type, e.ID, e.Description)
	}
	rv := NewResourceVersion(rcd.Identifying, rcd.Descriptive, metadataEntities, rcd.MinTime, rcd.MaxTime)

	existing, _ := accessor.GetKindMeta(KindResource)
	if existing == nil {
		accessor.SetKindMeta(KindResource, &Versioned[*ResourceVersion]{
			Versions: []*ResourceVersion{copyResourceVersion(rv)},
		})
	} else {
		vr := existing.(*Versioned[*ResourceVersion])
		vr.AddOrExtend(ResourceOps, rv)
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
	return store.(*MemStore[*ResourceVersion]).IterVersioned(ctx, func(labelsHash uint64, v *Versioned[*ResourceVersion]) error {
		return f(labelsHash, v)
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
