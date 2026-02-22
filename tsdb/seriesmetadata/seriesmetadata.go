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

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"

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
	IterKind(id KindID, f func(labelsHash uint64, versioned any) error) error

	// KindLen returns the number of entries for a kind.
	KindLen(id KindID) int
}

// MemSeriesMetadata is an in-memory implementation of series metadata storage.
// It wraps per-kind stores accessible both generically (via IterKind/KindLen)
// and type-safely (via ResourceStore/ScopeStore).
type MemSeriesMetadata struct {
	stores map[KindID]any // each value is *MemStore[V] for the appropriate V
}

// NewMemSeriesMetadata creates a new in-memory series metadata store.
func NewMemSeriesMetadata() *MemSeriesMetadata {
	m := &MemSeriesMetadata{
		stores: make(map[KindID]any, len(allKindsRegistered)),
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

// ResourceCount returns the number of unique series with resource data.
func (m *MemSeriesMetadata) ResourceCount() int { return m.ResourceStore().Len() }

// ScopeCount returns the number of unique series with scope data.
func (m *MemSeriesMetadata) ScopeCount() int { return m.ScopeStore().Len() }

// Close is a no-op for in-memory storage.
func (*MemSeriesMetadata) Close() error { return nil }

// IterKind iterates all entries for a kind.
func (m *MemSeriesMetadata) IterKind(id KindID, f func(labelsHash uint64, versioned any) error) error {
	kind, ok := KindByID(id)
	if !ok {
		return nil
	}
	store, ok := m.stores[id]
	if !ok {
		return nil
	}
	return kind.IterVersioned(store, f)
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

func (m *MemSeriesMetadata) IterResources(f func(labelsHash uint64, resource *ResourceVersion) error) error {
	return m.ResourceStore().Iter(f)
}

func (m *MemSeriesMetadata) IterVersionedResources(f func(labelsHash uint64, resources *VersionedResource) error) error {
	return m.ResourceStore().IterVersioned(f)
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

func (m *MemSeriesMetadata) IterVersionedScopes(f func(labelsHash uint64, scopes *VersionedScope) error) error {
	return m.ScopeStore().IterVersioned(f)
}

func (m *MemSeriesMetadata) TotalScopes() uint64 {
	return m.ScopeStore().TotalEntries()
}

func (m *MemSeriesMetadata) TotalScopeVersions() uint64 {
	return m.ScopeStore().TotalVersions()
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

func (r *parquetReader) IterResources(f func(labelsHash uint64, resource *ResourceVersion) error) error {
	return r.mem.IterResources(f)
}

func (r *parquetReader) IterVersionedResources(f func(labelsHash uint64, resources *VersionedResource) error) error {
	return r.mem.IterVersionedResources(f)
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

func (r *parquetReader) IterVersionedScopes(f func(labelsHash uint64, scopes *VersionedScope) error) error {
	return r.mem.IterVersionedScopes(f)
}

func (r *parquetReader) TotalScopes() uint64 {
	return r.mem.TotalScopes()
}

func (r *parquetReader) TotalScopeVersions() uint64 {
	return r.mem.TotalScopeVersions()
}

func (r *parquetReader) IterKind(id KindID, f func(labelsHash uint64, versioned any) error) error {
	return r.mem.IterKind(id, f)
}

func (r *parquetReader) KindLen(id KindID) int {
	return r.mem.KindLen(id)
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

	// Per-kind: content table (dedup) and mapping rows.
	type kindWriteState struct {
		kind         KindDescriptor
		contentTable map[uint64]metadataRow // contentHash → table row
		mappingRows  []metadataRow
	}

	kindStates := make(map[KindID]*kindWriteState)
	for _, kind := range AllKinds() {
		kindStates[kind.ID()] = &kindWriteState{
			kind:         kind,
			contentTable: make(map[uint64]metadataRow),
		}
	}

	// Iterate all kinds and build rows.
	for _, kind := range AllKinds() {
		state := kindStates[kind.ID()]
		err := mr.IterKind(kind.ID(), func(labelsHash uint64, versioned any) error {
			kind.IterateVersions(versioned, func(version any, minTime, maxTime int64) {
				contentHash := kind.ContentHash(version)
				if _, exists := state.contentTable[contentHash]; !exists {
					state.contentTable[contentHash] = kind.BuildTableRow(contentHash, version)
				} else {
					existing := state.contentTable[contentHash]
					existingVersion := kind.ParseTableRow(logger, &existing)
					if !kind.VersionsEqual(existingVersion, version) {
						logger.Warn("Hash collision detected in content-addressed table",
							"kind", string(kind.ID()), "content_hash", contentHash, "labels_hash", labelsHash)
					}
				}
				seriesRef := labelsHash
				if opts.RefResolver != nil {
					ref, ok := opts.RefResolver(labelsHash)
					if !ok {
						logger.Warn("Skipping unresolvable labels hash in write",
							"kind", string(kind.ID()), "labels_hash", labelsHash)
						return
					}
					seriesRef = ref
				}
				state.mappingRows = append(state.mappingRows, metadataRow{
					Namespace:   kind.MappingNamespace(),
					SeriesRef:   seriesRef,
					ContentHash: contentHash,
					MinTime:     minTime,
					MaxTime:     maxTime,
				})
			})
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("iterate %s: %w", kind.ID(), err)
		}
	}

	// Build per-namespace row slices.
	var allNamespaceRows [][]metadataRow
	totalRows := 0
	metadataCounts := make(map[string]int) // for footer metadata

	for _, kind := range AllKinds() {
		state := kindStates[kind.ID()]

		tableRows := make([]metadataRow, 0, len(state.contentTable))
		for _, row := range state.contentTable {
			tableRows = append(tableRows, row)
		}
		sortMetadataRows(tableRows)
		sortMetadataRows(state.mappingRows)

		metadataCounts[string(kind.ID())+"_table_count"] = len(tableRows)
		metadataCounts[string(kind.ID())+"_mapping_count"] = len(state.mappingRows)
		totalRows += len(tableRows) + len(state.mappingRows)

		allNamespaceRows = append(allNamespaceRows, tableRows, state.mappingRows)
	}

	if totalRows == 0 {
		return 0, nil
	}

	// Create temp file.
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

	// Build writer options.
	writerOpts := []parquet.WriterOption{
		parquet.Compression(&zstd.Codec{Level: zstd.SpeedBetterCompression}),
		parquet.KeyValueMetadata("schema_version", schemaVersion),
		parquet.KeyValueMetadata("row_group_layout", "namespace_partitioned"),
	}
	for k, v := range metadataCounts {
		writerOpts = append(writerOpts, parquet.KeyValueMetadata(k, strconv.Itoa(v)))
	}
	if opts.EnableBloomFilters {
		writerOpts = append(writerOpts,
			parquet.BloomFilters(
				parquet.SplitBlockFilter(10, "series_ref"),
				parquet.SplitBlockFilter(10, "content_hash"),
			),
		)
	}

	writer := parquet.NewGenericWriter[metadataRow](f, writerOpts...)

	for _, nsRows := range allNamespaceRows {
		if err := writeNamespaceRows(writer, nsRows, opts.MaxRowsPerRowGroup); err != nil {
			return 0, fmt.Errorf("write parquet rows: %w", err)
		}
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

	logger.Info("Series metadata written",
		"resource_table", metadataCounts["resource_table_count"],
		"resource_mappings", metadataCounts["resource_mapping_count"],
		"scope_table", metadataCounts["scope_table_count"],
		"scope_mappings", metadataCounts["scope_mapping_count"],
		"size", size)

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
