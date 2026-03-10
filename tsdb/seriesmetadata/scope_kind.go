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

func (scopeOps) ContentHash(v *ScopeVersion) uint64 { return hashScopeContent(v) }
func (scopeOps) ThinCopy(canonical, v *ScopeVersion) *ScopeVersion {
	return &ScopeVersion{
		Name:      canonical.Name,
		Version:   canonical.Version,
		SchemaURL: canonical.SchemaURL,
		Attrs:     canonical.Attrs,
		MinTime:   v.MinTime,
		MaxTime:   v.MaxTime,
	}
}

func (scopeOps) IsInterned(canonical, v *ScopeVersion) bool {
	return mapSameUnderlying(canonical.Attrs, v.Attrs)
}

// ScopeOps is the shared KindOps instance for scopes.
var ScopeOps KindOps[*ScopeVersion] = scopeOps{}

// hashScopeContent computes a deterministic xxhash for a ScopeVersion's content.
// The hash covers name, version, schema URL, and attributes.
// It does NOT include MinTime/MaxTime.
func hashScopeContent(sv *ScopeVersion) uint64 {
	hash, _ := hashScopeContentReusable(sv, nil)
	return hash
}

// hashScopeContentReusable is like hashScopeContent but accepts and returns
// a reusable keys buffer to avoid per-call []string allocations on the write path.
func hashScopeContentReusable(sv *ScopeVersion, keysBuf []string) (uint64, []string) {
	var h xxhash.Digest

	_, _ = h.WriteString(sv.Name)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(sv.Version)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(sv.SchemaURL)
	_, _ = h.Write([]byte{0})
	keysBuf = hashAttrs(&h, sv.Attrs, keysBuf)

	return h.Sum64(), keysBuf
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
	return buildScopeTableRow(contentHash, version.(*ScopeVersion))
}

// buildScopeTableRow converts a ScopeVersion into a content-addressed table row.
func buildScopeTableRow(contentHash uint64, sv *ScopeVersion) metadataRow {
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
	// Owned indicates the caller guarantees exclusive ownership of the Attrs map.
	// When true, buildScopeVersion takes the map directly instead of cloning.
	Owned bool
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

// hashScopeCommitData computes the content hash from raw ScopeCommitData
// without cloning any maps. Identical to hashScopeContent for equivalent data.
func hashScopeCommitData(scd ScopeCommitData) uint64 {
	hash, _ := hashScopeCommitDataReusable(scd, nil)
	return hash
}

// hashScopeCommitDataReusable is like hashScopeCommitData but accepts and
// returns a reusable keys buffer to avoid per-call []string allocations.
func hashScopeCommitDataReusable(scd ScopeCommitData, keysBuf []string) (uint64, []string) {
	var h xxhash.Digest

	_, _ = h.WriteString(scd.Name)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(scd.Version)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(scd.SchemaURL)
	_, _ = h.Write([]byte{0})
	keysBuf = hashAttrs(&h, scd.Attrs, keysBuf)

	return h.Sum64(), keysBuf
}

// CommitScopeToStore builds a ScopeVersion from ScopeCommitData and
// commits it directly to the MemStore, bypassing per-series storage entirely.
// Returns contentChanged=false when only the time range was extended (no WAL
// write needed — the >99% hot path).
// Returns contentChanged=true with old/cur materialized when content changed.
//
// Uses InsertVersion to avoid deep-copying attrs map when a canonical already
// exists in the content dedup table.
func CommitScopeToStore(store *MemStore[*ScopeVersion], labelsHash uint64, scd ScopeCommitData) (contentChanged bool, old, cur *VersionedScope) {
	contentHash := hashScopeCommitData(scd)

	return store.InsertVersion(labelsHash, contentHash, scd.MinTime, scd.MaxTime, func() *ScopeVersion {
		return buildScopeVersion(scd)
	})
}

// CommitScopeToStoreReusable is like CommitScopeToStore but accepts and
// returns a reusable keys buffer for hash computation, avoiding per-call
// []string allocations on the ingestion hot path.
func CommitScopeToStoreReusable(store *MemStore[*ScopeVersion], labelsHash uint64, scd ScopeCommitData, keysBuf []string) (contentChanged bool, old, cur *VersionedScope, updatedKeysBuf []string) {
	contentHash, keysBuf := hashScopeCommitDataReusable(scd, keysBuf)
	contentChanged, old, cur = store.InsertVersion(labelsHash, contentHash, scd.MinTime, scd.MaxTime, func() *ScopeVersion {
		return buildScopeVersion(scd)
	})
	return contentChanged, old, cur, keysBuf
}

// CommitScopeToStoreReusableWithRef is like CommitScopeToStoreReusable but
// also sets the series ref on the MemStore entry in the same critical section,
// avoiding a separate SetSeriesRef call (1 fewer lock + map lookup per series).
func CommitScopeToStoreReusableWithRef(store *MemStore[*ScopeVersion], labelsHash uint64, scd ScopeCommitData, seriesRef uint64, keysBuf []string) (contentChanged bool, old, cur *VersionedScope, updatedKeysBuf []string) {
	contentHash, keysBuf := hashScopeCommitDataReusable(scd, keysBuf)
	contentChanged, old, cur = store.InsertVersionWithRef(labelsHash, contentHash, scd.MinTime, scd.MaxTime, seriesRef, func() *ScopeVersion {
		return buildScopeVersion(scd)
	})
	return contentChanged, old, cur, keysBuf
}

// buildScopeVersion allocates a ScopeVersion from commit data.
// When scd.Owned is true, the attrs map is taken directly (zero-copy).
// Otherwise, the attrs map is deep-copied.
func buildScopeVersion(scd ScopeCommitData) *ScopeVersion {
	attrs := scd.Attrs
	if !scd.Owned {
		attrs = maps.Clone(scd.Attrs)
	}
	return &ScopeVersion{
		Name:      scd.Name,
		Version:   scd.Version,
		SchemaURL: scd.SchemaURL,
		Attrs:     attrs,
		MinTime:   scd.MinTime,
		MaxTime:   scd.MaxTime,
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
