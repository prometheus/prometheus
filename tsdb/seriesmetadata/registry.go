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
	"log/slog"
)

// KindID uniquely identifies a metadata kind.
type KindID string

const (
	KindResource KindID = "resource"
	KindScope    KindID = "scope"
)

// WALRecordType mirrors record.Type (uint8) to avoid an import cycle
// between seriesmetadata and tsdb/record.
type WALRecordType = uint8

// WAL record type constants matching record.ResourceUpdate and record.ScopeUpdate.
const (
	WALResourceUpdate WALRecordType = 11
	WALScopeUpdate    WALRecordType = 12
)

// KindDescriptor provides runtime dispatch for a metadata kind at
// serialization boundaries (WAL, Parquet, head commit/replay).
// Methods use `any` for type erasure since the registry is not generic.
type KindDescriptor interface {
	// ID returns the unique identifier for this kind.
	ID() KindID

	// WALRecordType returns the WAL record type constant for this kind.
	WALRecordType() WALRecordType

	// TableNamespace returns the Parquet namespace for content-addressed table rows.
	TableNamespace() string

	// MappingNamespace returns the Parquet namespace for series mapping rows.
	MappingNamespace() string

	// --- WAL encode/decode (type-erased at boundary) ---

	// DecodeWAL decodes a WAL record. `into` is a pooled slice to reuse.
	// Returns the decoded slice (type-erased).
	DecodeWAL(rec []byte, into any) (any, error)

	// EncodeWAL encodes records into a WAL record. Returns the encoded bytes.
	EncodeWAL(records any, buf []byte) []byte

	// --- Parquet conversion ---

	// ParseTableRow converts a Parquet table row into a version value (type-erased).
	ParseTableRow(logger *slog.Logger, row *metadataRow) any

	// BuildTableRow builds a Parquet table row from a content hash and version.
	BuildTableRow(contentHash uint64, version any) metadataRow

	// ContentHash computes the content hash for a version value.
	ContentHash(version any) uint64

	// --- Head integration ---

	// CommitToSeries applies a single WAL record entry to a memSeries.
	// `series` is a kindMetaAccessor, `walRecord` is a single WAL record entry.
	CommitToSeries(series, walRecord any)

	// CollectFromSeries extracts the *Versioned[V] from a memSeries (type-erased).
	// Returns nil, false if the series has no data for this kind.
	CollectFromSeries(series any) (any, bool)

	// CopyVersioned deep-copies a *Versioned[V] (type-erased).
	CopyVersioned(v any) any

	// SetOnSeries sets the *Versioned[V] on a memSeries (type-erased).
	SetOnSeries(series, versioned any)

	// --- Store operations (type-erased wrappers around MemStore) ---

	// NewStore creates a new *MemStore[V] (type-erased).
	NewStore() any

	// SetVersioned merges versioned data into the store.
	SetVersioned(store any, labelsHash uint64, versioned any)

	// IterVersioned iterates all entries in the store.
	IterVersioned(ctx context.Context, store any, f func(labelsHash uint64, versioned any) error) error

	// StoreLen returns the number of entries in the store.
	StoreLen(store any) int

	// --- Parquet denormalization ---

	// DenormalizeIntoStore copies versions from mapping rows, sets their time ranges,
	// sorts by MinTime, and sets the resulting *Versioned[V] on the store.
	DenormalizeIntoStore(store any, labelsHash uint64, versions []VersionWithTime)

	// IterateVersions iterates each version in a *Versioned[V] (type-erased),
	// calling f with the version and its MinTime/MaxTime.
	IterateVersions(versioned any, f func(version any, minTime, maxTime int64))

	// VersionsEqual compares two version values for content equality.
	VersionsEqual(a, b any) bool
}

// kindMetaAccessor is implemented by memSeries to provide kind-generic access
// to per-series metadata. Defined here (in seriesmetadata) so that KindDescriptor
// implementations can use it without importing tsdb.
type kindMetaAccessor interface {
	GetKindMeta(id KindID) (any, bool)
	SetKindMeta(id KindID, v any)
}

// Global kind registry.
var (
	kindsByID          = make(map[KindID]KindDescriptor)
	kindsByWALType     = make(map[WALRecordType]KindDescriptor)
	kindsByTableNS     = make(map[string]KindDescriptor)
	kindsByMappingNS   = make(map[string]KindDescriptor)
	allKindsRegistered []KindDescriptor
)

// RegisterKind registers a KindDescriptor in the global registry.
// Must be called from init() functions.
func RegisterKind(desc KindDescriptor) {
	id := desc.ID()
	if _, exists := kindsByID[id]; exists {
		panic("duplicate kind registration: " + string(id))
	}
	kindsByID[id] = desc
	kindsByWALType[desc.WALRecordType()] = desc
	kindsByTableNS[desc.TableNamespace()] = desc
	kindsByMappingNS[desc.MappingNamespace()] = desc
	allKindsRegistered = append(allKindsRegistered, desc)
}

// KindByID looks up a kind by its unique ID.
func KindByID(id KindID) (KindDescriptor, bool) {
	d, ok := kindsByID[id]
	return d, ok
}

// KindByWALType looks up a kind by its WAL record type.
func KindByWALType(t WALRecordType) (KindDescriptor, bool) {
	d, ok := kindsByWALType[t]
	return d, ok
}

// KindByTableNS looks up a kind by its Parquet table namespace.
func KindByTableNS(ns string) (KindDescriptor, bool) {
	d, ok := kindsByTableNS[ns]
	return d, ok
}

// KindByMappingNS looks up a kind by its Parquet mapping namespace.
func KindByMappingNS(ns string) (KindDescriptor, bool) {
	d, ok := kindsByMappingNS[ns]
	return d, ok
}

// AllKinds returns all registered kinds in registration order.
func AllKinds() []KindDescriptor {
	return allKindsRegistered
}
