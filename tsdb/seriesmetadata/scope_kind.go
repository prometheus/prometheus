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
	RegisterKind(&scopeKindDescriptor{})
}

// scopeOps implements KindOps for *ScopeVersion.
type scopeOps struct{}

func (scopeOps) Equal(a, b *ScopeVersion) bool      { return ScopeVersionsEqual(a, b) }
func (scopeOps) Copy(v *ScopeVersion) *ScopeVersion { return CopyScopeVersion(v) }

// ScopeOps is the shared KindOps instance for scopes.
var ScopeOps KindOps[*ScopeVersion] = scopeOps{}

// hashScopeContent computes a deterministic xxhash for a ScopeVersion's content.
// The hash covers name, version, schema URL, and attributes.
// It does NOT include MinTime/MaxTime.
func hashScopeContent(sv *ScopeVersion) uint64 {
	h := xxhash.New()

	_, _ = h.WriteString(sv.Name)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(sv.Version)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(sv.SchemaURL)
	_, _ = h.Write([]byte{0})
	hashAttrs(h, sv.Attrs)

	return h.Sum64()
}

// scopeKindDescriptor implements KindDescriptor for OTel scopes.
type scopeKindDescriptor struct{}

func (*scopeKindDescriptor) ID() KindID                   { return KindScope }
func (*scopeKindDescriptor) WALRecordType() WALRecordType { return WALScopeUpdate }
func (*scopeKindDescriptor) TableNamespace() string       { return NamespaceScopeTable }
func (*scopeKindDescriptor) MappingNamespace() string     { return NamespaceScopeMapping }

// DecodeWAL and EncodeWAL delegate to pluggable functions to avoid import cycle.
func (*scopeKindDescriptor) DecodeWAL(rec []byte, into any) (any, error) {
	return ScopeDecodeWAL(rec, into)
}

func (*scopeKindDescriptor) EncodeWAL(records any, buf []byte) []byte {
	return ScopeEncodeWAL(records, buf)
}

// ScopeDecodeWAL is set by the tsdb package to break the import cycle.
var ScopeDecodeWAL func(rec []byte, into any) (any, error)

// ScopeEncodeWAL is set by the tsdb package to break the import cycle.
var ScopeEncodeWAL func(records any, buf []byte) []byte

func (*scopeKindDescriptor) ParseTableRow(_ *slog.Logger, row *metadataRow) any {
	return parseScopeContent(row)
}

func (*scopeKindDescriptor) BuildTableRow(contentHash uint64, version any) metadataRow {
	sv := version.(*ScopeVersion)
	scopeAttrs := make([]EntityAttributeEntry, 0, len(sv.Attrs))
	for k, v := range sv.Attrs {
		scopeAttrs = append(scopeAttrs, EntityAttributeEntry{Key: k, Value: v})
	}
	sortAttrEntries(scopeAttrs)
	return metadataRow{
		Namespace:       NamespaceScopeTable,
		ContentHash:     contentHash,
		ScopeName:       sv.Name,
		ScopeVersionStr: sv.Version,
		SchemaURL:       sv.SchemaURL,
		ScopeAttrs:      scopeAttrs,
	}
}

func (*scopeKindDescriptor) ContentHash(version any) uint64 {
	return hashScopeContent(version.(*ScopeVersion))
}

// ScopeCommitData carries scope WAL record data without importing tsdb/record.
type ScopeCommitData struct {
	Name      string
	Version   string
	SchemaURL string
	Attrs     map[string]string
	MinTime   int64
	MaxTime   int64
}

func (*scopeKindDescriptor) CommitToSeries(series, walRecord any) {
	CommitScopeDirect(series.(kindMetaAccessor), walRecord.(ScopeCommitData))
}

// CommitScopeDirect is the hot-path commit for scopes.
// It constructs the ScopeVersion with exactly one deep copy of the attrs map
// and takes ownership — no further copies via AddOrExtend or CopyScopeVersion.
// Called directly from headAppenderBase.commitScopes and from
// CommitToSeries (cold path, WAL replay).
func CommitScopeDirect(accessor kindMetaAccessor, scd ScopeCommitData) {
	sv := &ScopeVersion{
		Name:      scd.Name,
		Version:   scd.Version,
		SchemaURL: scd.SchemaURL,
		Attrs:     maps.Clone(scd.Attrs),
		MinTime:   scd.MinTime,
		MaxTime:   scd.MaxTime,
	}

	existing, _ := accessor.GetKindMeta(KindScope)
	if existing == nil {
		accessor.SetKindMeta(KindScope, &Versioned[*ScopeVersion]{
			Versions: []*ScopeVersion{sv},
		})
	} else {
		vs := existing.(*Versioned[*ScopeVersion])
		if len(vs.Versions) > 0 && ScopeOps.Equal(vs.Versions[len(vs.Versions)-1], sv) {
			vs.Versions[len(vs.Versions)-1].UpdateTimeRange(sv.MinTime, sv.MaxTime)
		} else {
			vs.Versions = append(vs.Versions, sv)
		}
	}
}

// CollectScopeDirect is the hot-path equivalent of CollectFromSeries
// for scopes, avoiding interface{} boxing on the return path.
func CollectScopeDirect(accessor kindMetaAccessor) (*VersionedScope, bool) {
	v, ok := accessor.GetKindMeta(KindScope)
	if !ok || v == nil {
		return nil, false
	}
	return v.(*Versioned[*ScopeVersion]), true
}

func (*scopeKindDescriptor) CollectFromSeries(series any) (any, bool) {
	accessor := series.(kindMetaAccessor)
	return accessor.GetKindMeta(KindScope)
}

func (*scopeKindDescriptor) CopyVersioned(v any) any {
	return v.(*Versioned[*ScopeVersion]).Copy(ScopeOps)
}

func (*scopeKindDescriptor) SetOnSeries(series, versioned any) {
	accessor := series.(kindMetaAccessor)
	accessor.SetKindMeta(KindScope, versioned)
}

func (*scopeKindDescriptor) NewStore() any {
	return NewMemStore[*ScopeVersion](ScopeOps)
}

func (*scopeKindDescriptor) SetVersioned(store any, labelsHash uint64, versioned any) {
	store.(*MemStore[*ScopeVersion]).SetVersioned(labelsHash, versioned.(*Versioned[*ScopeVersion]))
}

func (*scopeKindDescriptor) IterVersioned(ctx context.Context, store any, f func(labelsHash uint64, versioned any) error) error {
	return store.(*MemStore[*ScopeVersion]).IterVersioned(ctx, func(labelsHash uint64, v *Versioned[*ScopeVersion]) error {
		return f(labelsHash, v)
	})
}

func (*scopeKindDescriptor) StoreLen(store any) int {
	return store.(*MemStore[*ScopeVersion]).Len()
}

func (*scopeKindDescriptor) DenormalizeIntoStore(store any, labelsHash uint64, versions []VersionWithTime) {
	typed := make([]*ScopeVersion, len(versions))
	for i, vt := range versions {
		cp := CopyScopeVersion(vt.Version.(*ScopeVersion))
		cp.MinTime = vt.MinTime
		cp.MaxTime = vt.MaxTime
		typed[i] = cp
	}
	slices.SortFunc(typed, func(a, b *ScopeVersion) int {
		return cmp.Compare(a.MinTime, b.MinTime)
	})
	store.(*MemStore[*ScopeVersion]).SetVersioned(labelsHash, &Versioned[*ScopeVersion]{Versions: typed})
}

func (*scopeKindDescriptor) IterateVersions(versioned any, f func(version any, minTime, maxTime int64)) {
	vs := versioned.(*Versioned[*ScopeVersion])
	for _, sv := range vs.Versions {
		f(sv, sv.MinTime, sv.MaxTime)
	}
}

func (*scopeKindDescriptor) VersionsEqual(a, b any) bool {
	return ScopeVersionsEqual(a.(*ScopeVersion), b.(*ScopeVersion))
}
