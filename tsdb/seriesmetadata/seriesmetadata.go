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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// sortMetadata sorts a metadata slice deterministically by Type, Help, Unit.
func sortMetadata(metas []metadata.Metadata) {
	slices.SortFunc(metas, func(a, b metadata.Metadata) int {
		if c := cmp.Compare(string(a.Type), string(b.Type)); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Help, b.Help); c != 0 {
			return c
		}
		return cmp.Compare(a.Unit, b.Unit)
	})
}

// SeriesMetadataFilename is the name of the series metadata file in a block directory.
const SeriesMetadataFilename = "series_metadata.parquet"

// Namespace constants for the content-addressed metadataRow discriminator.
const (
	nsMetadataTable   = "metadata_table"   // content-addressed table: unique (type, unit, help) tuples
	nsMetadataMapping = "metadata_mapping" // maps series→versioned metadata: seriesRef, contentHash, minTime, maxTime
)

// schemaVersion is stored in the Parquet footer for future schema evolution.
const schemaVersion = "1"

// Reader provides read access to series metadata.
type Reader interface {
	// Get returns metadata for the series with the given ref.
	Get(ref uint64) (metadata.Metadata, bool)

	// GetByMetricName returns all unique metadata entries for the given metric name.
	GetByMetricName(name string) ([]metadata.Metadata, bool)

	// Iter calls the given function for each series metadata entry.
	Iter(f func(ref uint64, meta metadata.Metadata) error) error

	// IterByMetricName calls the given function for each metric name and its metadata entries.
	IterByMetricName(f func(name string, metas []metadata.Metadata) error) error

	// Total returns the total count of unique metric names with metadata.
	Total() uint64

	// Close releases any resources associated with the reader.
	Close() error

	// VersionedMetadataReader methods for time-varying metadata.
	VersionedMetadataReader

	// VersionedResourceReader provides access to versioned OTel resources.
	// Resources include both identifying/descriptive attributes and typed entities.
	VersionedResourceReader

	// VersionedScopeReader provides access to versioned OTel InstrumentationScope data.
	VersionedScopeReader
}

// metadataEntry stores metadata with both ref and metric name for indexing.
type metadataEntry struct {
	metricName string
	ref        uint64
	meta       metadata.Metadata
}

// MemSeriesMetadata is an in-memory implementation of series metadata storage.
type MemSeriesMetadata struct {
	// byRef maps series ref to metadata entry (latest value).
	byRef map[uint64]*metadataEntry
	// byName maps metric name to a set of unique metadata tuples.
	byName map[string]map[metadata.Metadata]struct{}
	// byRefVersioned maps series ref to versioned metadata.
	byRefVersioned map[uint64]*versionedEntry
	mtx            sync.RWMutex

	// resourceStore stores OTel resources (attributes + entities) per series
	resourceStore *MemResourceStore

	// scopeStore stores OTel InstrumentationScope data per series
	scopeStore *MemScopeStore
}

// versionedEntry pairs a metric name with versioned metadata for indexing.
type versionedEntry struct {
	metricName string
	labels     labels.Labels
	vm         *VersionedMetadata
}

// NewMemSeriesMetadata creates a new in-memory series metadata store.
func NewMemSeriesMetadata() *MemSeriesMetadata {
	return &MemSeriesMetadata{
		byRef:          make(map[uint64]*metadataEntry),
		byName:         make(map[string]map[metadata.Metadata]struct{}),
		byRefVersioned: make(map[uint64]*versionedEntry),
		resourceStore:  NewMemResourceStore(),
		scopeStore:     NewMemScopeStore(),
	}
}

// MetricCount returns the number of metric entries.
func (m *MemSeriesMetadata) MetricCount() int {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return len(m.byName)
}

// ResourceCount returns the number of unique series with resource data.
func (m *MemSeriesMetadata) ResourceCount() int { return m.resourceStore.Len() }

// ScopeCount returns the number of unique series with scope data.
func (m *MemSeriesMetadata) ScopeCount() int { return m.scopeStore.Len() }

// Get returns metadata for the series with the given ref.
func (m *MemSeriesMetadata) Get(ref uint64) (metadata.Metadata, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byRef[ref]
	if !ok {
		return metadata.Metadata{}, false
	}
	return entry.meta, true
}

// GetByMetricName returns all unique metadata entries for the given metric name.
func (m *MemSeriesMetadata) GetByMetricName(name string) ([]metadata.Metadata, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	set, ok := m.byName[name]
	if !ok || len(set) == 0 {
		return nil, false
	}
	result := make([]metadata.Metadata, 0, len(set))
	for meta := range set {
		result = append(result, meta)
	}
	sortMetadata(result)
	return result, true
}

// Set stores metadata for the given metric name and series ref.
func (m *MemSeriesMetadata) Set(metricName string, ref uint64, meta metadata.Metadata) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if ref != 0 {
		if old, ok := m.byRef[ref]; ok && (old.meta != meta || old.metricName != metricName) {
			if set, exists := m.byName[old.metricName]; exists {
				delete(set, old.meta)
				if len(set) == 0 {
					delete(m.byName, old.metricName)
				}
			}
		}
	}

	entry := &metadataEntry{
		metricName: metricName,
		ref:        ref,
		meta:       meta,
	}
	if ref != 0 {
		m.byRef[ref] = entry
	}

	if _, ok := m.byName[metricName]; !ok {
		m.byName[metricName] = make(map[metadata.Metadata]struct{})
	}
	m.byName[metricName][meta] = struct{}{}
}

// SetVersioned stores a versioned metadata entry for the given series.
// It also updates the byRef/byName maps with the latest version's metadata.
func (m *MemSeriesMetadata) SetVersioned(metricName string, ref uint64, version *MetadataVersion) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	entry, ok := m.byRefVersioned[ref]
	if !ok {
		entry = &versionedEntry{
			metricName: metricName,
			vm:         &VersionedMetadata{},
		}
		m.byRefVersioned[ref] = entry
	}
	entry.vm.AddOrExtend(version)

	// Update the non-versioned maps with the latest value.
	cur := entry.vm.CurrentVersion()
	if cur != nil {
		m.setLatestLocked(metricName, ref, cur.Meta)
	}
}

// SetVersionedMetadata bulk-sets a complete VersionedMetadata for a series.
// If metricName is empty, only the versioned store is updated (byRef/byName are not touched).
func (m *MemSeriesMetadata) SetVersionedMetadata(ref uint64, metricName string, vm *VersionedMetadata) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byRefVersioned[ref]; ok && metricName == "" {
		metricName = existing.metricName
	}

	m.byRefVersioned[ref] = &versionedEntry{
		metricName: metricName,
		vm:         vm,
	}

	// Update the non-versioned maps with the latest value, but only if we have a metric name.
	if metricName != "" {
		cur := vm.CurrentVersion()
		if cur != nil {
			m.setLatestLocked(metricName, ref, cur.Meta)
		}
	}
}

// SetVersionedMetadataWithLabels bulk-sets a complete VersionedMetadata for a series,
// storing the full label set alongside the metric name.
func (m *MemSeriesMetadata) SetVersionedMetadataWithLabels(ref uint64, lset labels.Labels, vm *VersionedMetadata) {
	metricName := lset.Get(labels.MetricName)

	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byRefVersioned[ref]; ok && metricName == "" {
		metricName = existing.metricName
	}

	m.byRefVersioned[ref] = &versionedEntry{
		metricName: metricName,
		labels:     lset,
		vm:         vm,
	}

	if metricName != "" {
		cur := vm.CurrentVersion()
		if cur != nil {
			m.setLatestLocked(metricName, ref, cur.Meta)
		}
	}
}

// setLatestLocked updates byRef/byName with the latest metadata value.
// Caller must hold m.mtx.
func (m *MemSeriesMetadata) setLatestLocked(metricName string, ref uint64, meta metadata.Metadata) {
	if ref != 0 {
		if old, ok := m.byRef[ref]; ok && (old.meta != meta || old.metricName != metricName) {
			if set, exists := m.byName[old.metricName]; exists {
				delete(set, old.meta)
				if len(set) == 0 {
					delete(m.byName, old.metricName)
				}
			}
		}
	}

	entry := &metadataEntry{
		metricName: metricName,
		ref:        ref,
		meta:       meta,
	}
	if ref != 0 {
		m.byRef[ref] = entry
	}

	if _, ok := m.byName[metricName]; !ok {
		m.byName[metricName] = make(map[metadata.Metadata]struct{})
	}
	m.byName[metricName][meta] = struct{}{}
}

// Delete removes metadata for the given series ref.
func (m *MemSeriesMetadata) Delete(ref uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if entry, ok := m.byRef[ref]; ok {
		if set, exists := m.byName[entry.metricName]; exists {
			delete(set, entry.meta)
			if len(set) == 0 {
				delete(m.byName, entry.metricName)
			}
		}
		delete(m.byRef, ref)
	}
	delete(m.byRefVersioned, ref)
}

// Iter calls the given function for each metadata entry by series ref.
func (m *MemSeriesMetadata) Iter(f func(ref uint64, meta metadata.Metadata) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for r, entry := range m.byRef {
		if err := f(r, entry.meta); err != nil {
			return err
		}
	}
	return nil
}

// IterByMetricName calls the given function for each metric name and its metadata entries.
func (m *MemSeriesMetadata) IterByMetricName(f func(name string, metas []metadata.Metadata) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for name, set := range m.byName {
		metas := make([]metadata.Metadata, 0, len(set))
		for meta := range set {
			metas = append(metas, meta)
		}
		sortMetadata(metas)
		if err := f(name, metas); err != nil {
			return err
		}
	}
	return nil
}

// Total returns the total count of unique metric names with metadata.
func (m *MemSeriesMetadata) Total() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return uint64(len(m.byName))
}

// Close is a no-op for in-memory storage.
func (*MemSeriesMetadata) Close() error {
	return nil
}

// GetVersionedMetadata returns the versioned metadata for a series.
func (m *MemSeriesMetadata) GetVersionedMetadata(ref uint64) (*VersionedMetadata, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byRefVersioned[ref]
	if !ok {
		return nil, false
	}
	return entry.vm, true
}

// IterVersionedMetadata calls the given function for each series with versioned metadata.
func (m *MemSeriesMetadata) IterVersionedMetadata(f func(ref uint64, metricName string, lset labels.Labels, vm *VersionedMetadata) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for r, entry := range m.byRefVersioned {
		if err := f(r, entry.metricName, entry.labels, entry.vm); err != nil {
			return err
		}
	}
	return nil
}

// TotalVersionedMetadata returns the total count of series with versioned metadata.
func (m *MemSeriesMetadata) TotalVersionedMetadata() int {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return len(m.byRefVersioned)
}

// GetResource returns the current (latest) resource for the series.
func (m *MemSeriesMetadata) GetResource(labelsHash uint64) (*ResourceVersion, bool) {
	return m.resourceStore.GetResource(labelsHash)
}

// GetVersionedResource returns all versions of the resource for the series.
func (m *MemSeriesMetadata) GetVersionedResource(labelsHash uint64) (*VersionedResource, bool) {
	return m.resourceStore.GetVersionedResource(labelsHash)
}

// GetResourceAt returns the resource version active at the given timestamp.
func (m *MemSeriesMetadata) GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool) {
	return m.resourceStore.GetResourceAt(labelsHash, timestamp)
}

// SetResource stores a resource for the series.
func (m *MemSeriesMetadata) SetResource(labelsHash uint64, resource *ResourceVersion) {
	m.resourceStore.SetResource(labelsHash, resource)
}

// SetVersionedResource stores versioned resources for the series.
func (m *MemSeriesMetadata) SetVersionedResource(labelsHash uint64, resources *VersionedResource) {
	m.resourceStore.SetVersionedResource(labelsHash, resources)
}

// DeleteResource removes all resource data for the series.
func (m *MemSeriesMetadata) DeleteResource(labelsHash uint64) {
	m.resourceStore.DeleteResource(labelsHash)
}

// IterResources calls the function for each series' current resource.
func (m *MemSeriesMetadata) IterResources(f func(labelsHash uint64, resource *ResourceVersion) error) error {
	return m.resourceStore.IterResources(f)
}

// IterVersionedResources calls the function for each series' versioned resources.
func (m *MemSeriesMetadata) IterVersionedResources(f func(labelsHash uint64, resources *VersionedResource) error) error {
	return m.resourceStore.IterVersionedResources(f)
}

// TotalResources returns the count of series with resources.
func (m *MemSeriesMetadata) TotalResources() uint64 {
	return m.resourceStore.TotalResources()
}

// TotalResourceVersions returns the total count of all resource versions.
func (m *MemSeriesMetadata) TotalResourceVersions() uint64 {
	return m.resourceStore.TotalResourceVersions()
}

// GetVersionedScope returns all versions of the scope for the series.
func (m *MemSeriesMetadata) GetVersionedScope(labelsHash uint64) (*VersionedScope, bool) {
	return m.scopeStore.GetVersionedScope(labelsHash)
}

// SetVersionedScope stores versioned scopes for the series.
func (m *MemSeriesMetadata) SetVersionedScope(labelsHash uint64, scopes *VersionedScope) {
	m.scopeStore.SetVersionedScope(labelsHash, scopes)
}

// IterVersionedScopes calls the function for each series' versioned scopes.
func (m *MemSeriesMetadata) IterVersionedScopes(f func(labelsHash uint64, scopes *VersionedScope) error) error {
	return m.scopeStore.IterVersionedScopes(f)
}

// TotalScopes returns the count of series with scopes.
func (m *MemSeriesMetadata) TotalScopes() uint64 {
	return m.scopeStore.TotalScopes()
}

// TotalScopeVersions returns the total count of all scope versions.
func (m *MemSeriesMetadata) TotalScopeVersions() uint64 {
	return m.scopeStore.TotalScopeVersions()
}

// parquetReader implements Reader by reading from a Parquet file.
type parquetReader struct {
	file           *os.File
	byRef          map[uint64]*metadataEntry
	byName         map[string][]metadata.Metadata
	byRefVersioned map[uint64]*versionedEntry

	// resourceStore stores OTel resources (attributes + entities) loaded from the file
	resourceStore *MemResourceStore

	// scopeStore stores OTel InstrumentationScope data loaded from the file
	scopeStore *MemScopeStore

	closeOnce sync.Once
	closeErr  error
}

// newParquetReader creates a reader from an open file.
func newParquetReader(logger *slog.Logger, file *os.File) (*parquetReader, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	rows, err := parquet.Read[metadataRow](file, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("read parquet rows: %w", err)
	}

	contentTable := make(map[uint64]metadata.Metadata)

	// Phase 1: Build content table from metadata_table rows.
	for i := range rows {
		row := &rows[i]
		if row.Namespace == nsMetadataTable {
			contentTable[row.ContentHash] = metadata.Metadata{
				Type: model.MetricType(row.Type),
				Unit: row.Unit,
				Help: row.Help,
			}
		}
	}

	// Phase 2: Collect mapping entries.
	type mappingEntry struct {
		seriesRef   uint64
		metricName  string
		contentHash uint64
		minTime     int64
		maxTime     int64
	}
	var mappings []mappingEntry
	for i := range rows {
		row := &rows[i]
		if row.Namespace == nsMetadataMapping {
			mappings = append(mappings, mappingEntry{
				seriesRef:   row.SeriesRef,
				metricName:  row.MetricName,
				contentHash: row.ContentHash,
				minTime:     row.MinTime,
				maxTime:     row.MaxTime,
			})
		}
	}

	// Phase 3: Resolve mappings → versioned metadata.
	byRefVersioned := make(map[uint64]*versionedEntry)
	// Sort mappings by seriesRef then minTime for deterministic construction.
	sort.Slice(mappings, func(i, j int) bool {
		if mappings[i].seriesRef != mappings[j].seriesRef {
			return mappings[i].seriesRef < mappings[j].seriesRef
		}
		return mappings[i].minTime < mappings[j].minTime
	})
	for _, mp := range mappings {
		meta, ok := contentTable[mp.contentHash]
		if !ok {
			continue // Orphaned mapping row; skip.
		}
		entry, ok := byRefVersioned[mp.seriesRef]
		if !ok {
			// labels is intentionally left empty: the Parquet format stores
			// series refs, not label sets. Callers that need labels must
			// resolve them via the block's index reader.
			entry = &versionedEntry{
				metricName: mp.metricName,
				vm:         &VersionedMetadata{},
			}
			byRefVersioned[mp.seriesRef] = entry
		}
		entry.vm.AddOrExtend(&MetadataVersion{
			Meta:    meta,
			MinTime: mp.minTime,
			MaxTime: mp.maxTime,
		})
	}

	// Phase 4: Derive byRef/byName from versioned data (latest version per series).
	byRef := make(map[uint64]*metadataEntry, len(byRefVersioned))
	byNameSet := make(map[string]map[metadata.Metadata]struct{})
	for r, ve := range byRefVersioned {
		cur := ve.vm.CurrentVersion()
		if cur == nil {
			continue
		}
		byRef[r] = &metadataEntry{
			metricName: ve.metricName,
			ref:        r,
			meta:       cur.Meta,
		}
		if _, ok := byNameSet[ve.metricName]; !ok {
			byNameSet[ve.metricName] = make(map[metadata.Metadata]struct{})
		}
		byNameSet[ve.metricName][cur.Meta] = struct{}{}
	}
	byName := make(map[string][]metadata.Metadata, len(byNameSet))
	for name, set := range byNameSet {
		metas := make([]metadata.Metadata, 0, len(set))
		for meta := range set {
			metas = append(metas, meta)
		}
		sortMetadata(metas)
		byName[name] = metas
	}

	// Also read resource and scope rows.
	resourceStore := NewMemResourceStore()
	scopeStore := NewMemScopeStore()
	resourceVersionsByHash := make(map[uint64][]*ResourceVersion)
	scopeVersionsByHash := make(map[uint64][]*ScopeVersion)

	for i := range rows {
		row := &rows[i]

		switch row.Namespace {
		case NamespaceResource:
			// Convert Parquet row to unified ResourceVersion
			identifying := make(map[string]string, len(row.IdentifyingAttrs))
			for _, attr := range row.IdentifyingAttrs {
				identifying[attr.Key] = attr.Value
			}
			descriptive := make(map[string]string, len(row.DescriptiveAttrs))
			for _, attr := range row.DescriptiveAttrs {
				descriptive[attr.Key] = attr.Value
			}

			// Parse entities from row
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

			rv := &ResourceVersion{
				Identifying: identifying,
				Descriptive: descriptive,
				Entities:    entities,
				MinTime:     row.MinTime,
				MaxTime:     row.MaxTime,
			}
			resourceVersionsByHash[row.LabelsHash] = append(resourceVersionsByHash[row.LabelsHash], rv)

		case NamespaceScope:
			attrs := make(map[string]string, len(row.ScopeAttrs))
			for _, attr := range row.ScopeAttrs {
				attrs[attr.Key] = attr.Value
			}
			sv := &ScopeVersion{
				Name:      row.ScopeName,
				Version:   row.ScopeVersionStr,
				SchemaURL: row.SchemaURL,
				Attrs:     attrs,
				MinTime:   row.MinTime,
				MaxTime:   row.MaxTime,
			}
			scopeVersionsByHash[row.LabelsHash] = append(scopeVersionsByHash[row.LabelsHash], sv)
		}
	}

	// Set versioned resources (already sorted by MinTime from WriteFile order)
	for labelsHash, versions := range resourceVersionsByHash {
		resourceStore.SetVersionedResource(labelsHash, &VersionedResource{
			Versions: versions,
		})
	}

	// Set versioned scopes (already sorted by MinTime from WriteFile order)
	for labelsHash, versions := range scopeVersionsByHash {
		scopeStore.SetVersionedScope(labelsHash, &VersionedScope{
			Versions: versions,
		})
	}

	return &parquetReader{
		file:           file,
		byRef:          byRef,
		byName:         byName,
		byRefVersioned: byRefVersioned,
		resourceStore:  resourceStore,
		scopeStore:     scopeStore,
	}, nil
}

// Get returns metadata for the series with the given ref.
func (r *parquetReader) Get(ref uint64) (metadata.Metadata, bool) {
	entry, ok := r.byRef[ref]
	if !ok {
		return metadata.Metadata{}, false
	}
	return entry.meta, true
}

// GetByMetricName returns all metadata entries for the given metric name.
func (r *parquetReader) GetByMetricName(name string) ([]metadata.Metadata, bool) {
	metas, ok := r.byName[name]
	if !ok || len(metas) == 0 {
		return nil, false
	}
	return metas, true
}

// Iter calls the given function for each metadata entry by series ref.
func (r *parquetReader) Iter(f func(ref uint64, meta metadata.Metadata) error) error {
	for sr, entry := range r.byRef {
		if err := f(sr, entry.meta); err != nil {
			return err
		}
	}
	return nil
}

// IterByMetricName calls the given function for each metric name and its metadata entries.
func (r *parquetReader) IterByMetricName(f func(name string, metas []metadata.Metadata) error) error {
	for name, metas := range r.byName {
		if err := f(name, metas); err != nil {
			return err
		}
	}
	return nil
}

// Total returns the total count of metadata entries.
func (r *parquetReader) Total() uint64 {
	return uint64(len(r.byName))
}

// GetVersionedMetadata returns the versioned metadata for a series.
func (r *parquetReader) GetVersionedMetadata(ref uint64) (*VersionedMetadata, bool) {
	entry, ok := r.byRefVersioned[ref]
	if !ok {
		return nil, false
	}
	return entry.vm, true
}

// IterVersionedMetadata calls the given function for each series with versioned metadata.
func (r *parquetReader) IterVersionedMetadata(f func(ref uint64, metricName string, lset labels.Labels, vm *VersionedMetadata) error) error {
	for sr, entry := range r.byRefVersioned {
		if err := f(sr, entry.metricName, entry.labels, entry.vm); err != nil {
			return err
		}
	}
	return nil
}

// TotalVersionedMetadata returns the total count of series with versioned metadata.
func (r *parquetReader) TotalVersionedMetadata() int {
	return len(r.byRefVersioned)
}

// GetResource returns the current (latest) resource for the series.
func (r *parquetReader) GetResource(labelsHash uint64) (*ResourceVersion, bool) {
	return r.resourceStore.GetResource(labelsHash)
}

// GetVersionedResource returns all versions of the resource for the series.
func (r *parquetReader) GetVersionedResource(labelsHash uint64) (*VersionedResource, bool) {
	return r.resourceStore.GetVersionedResource(labelsHash)
}

// GetResourceAt returns the resource version active at the given timestamp.
func (r *parquetReader) GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool) {
	return r.resourceStore.GetResourceAt(labelsHash, timestamp)
}

// IterResources calls the function for each series' current resource.
func (r *parquetReader) IterResources(f func(labelsHash uint64, resource *ResourceVersion) error) error {
	return r.resourceStore.IterResources(f)
}

// IterVersionedResources calls the function for each series' versioned resources.
func (r *parquetReader) IterVersionedResources(f func(labelsHash uint64, resources *VersionedResource) error) error {
	return r.resourceStore.IterVersionedResources(f)
}

// TotalResources returns the count of series with resources.
func (r *parquetReader) TotalResources() uint64 {
	return r.resourceStore.TotalResources()
}

// TotalResourceVersions returns the total count of all resource versions.
func (r *parquetReader) TotalResourceVersions() uint64 {
	return r.resourceStore.TotalResourceVersions()
}

// GetVersionedScope returns all versions of the scope for the series.
func (r *parquetReader) GetVersionedScope(labelsHash uint64) (*VersionedScope, bool) {
	return r.scopeStore.GetVersionedScope(labelsHash)
}

// IterVersionedScopes calls the function for each series' versioned scopes.
func (r *parquetReader) IterVersionedScopes(f func(labelsHash uint64, scopes *VersionedScope) error) error {
	return r.scopeStore.IterVersionedScopes(f)
}

// TotalScopes returns the count of series with scopes.
func (r *parquetReader) TotalScopes() uint64 {
	return r.scopeStore.TotalScopes()
}

// TotalScopeVersions returns the total count of all scope versions.
func (r *parquetReader) TotalScopeVersions() uint64 {
	return r.scopeStore.TotalScopeVersions()
}

// Close releases resources associated with the reader.
func (r *parquetReader) Close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.file.Close()
	})
	return r.closeErr
}

// sortAttrEntries sorts attribute entries by key for deterministic Parquet output.
func sortAttrEntries(entries []EntityAttributeEntry) {
	slices.SortFunc(entries, func(a, b EntityAttributeEntry) int {
		return cmp.Compare(a.Key, b.Key)
	})
}

// WriteFile atomically writes series metadata to a Parquet file in the given directory.
// Writes content-addressed metadata namespaces plus resource and scope rows.
func WriteFile(logger *slog.Logger, dir string, mr Reader) (int64, error) {
	path := filepath.Join(dir, SeriesMetadataFilename)
	tmp := path + ".tmp"

	var rows []metadataRow

	// 1. Build content-addressed table and mapping rows from versioned metadata.
	contentTable := make(map[uint64]metadata.Metadata) // contentHash → metadata
	err := mr.IterVersionedMetadata(func(ref uint64, metricName string, _ labels.Labels, vm *VersionedMetadata) error {
		for _, v := range vm.Versions {
			ch := HashMetadataContent(v.Meta)
			if _, exists := contentTable[ch]; !exists {
				contentTable[ch] = v.Meta
			}
			rows = append(rows, metadataRow{
				Namespace:   nsMetadataMapping,
				SeriesRef:   ref,
				ContentHash: ch,
				MinTime:     v.MinTime,
				MaxTime:     v.MaxTime,
				MetricName:  metricName,
			})
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate versioned metadata: %w", err)
	}

	// 3. Write content table rows.
	for ch, meta := range contentTable {
		rows = append(rows, metadataRow{
			Namespace:   nsMetadataTable,
			ContentHash: ch,
			Type:        string(meta.Type),
			Unit:        meta.Unit,
			Help:        meta.Help,
		})
	}

	// Collect all resources into rows (one row per version, including attributes and entities)
	err = mr.IterVersionedResources(func(labelsHash uint64, vresource *VersionedResource) error {
		for _, rv := range vresource.Versions {
			// Convert resource-level identifying attributes to list
			idAttrs := make([]EntityAttributeEntry, 0, len(rv.Identifying))
			for k, v := range rv.Identifying {
				idAttrs = append(idAttrs, EntityAttributeEntry{
					Key:   k,
					Value: v,
				})
			}
			sortAttrEntries(idAttrs)

			// Convert resource-level descriptive attributes to list
			descAttrs := make([]EntityAttributeEntry, 0, len(rv.Descriptive))
			for k, v := range rv.Descriptive {
				descAttrs = append(descAttrs, EntityAttributeEntry{
					Key:   k,
					Value: v,
				})
			}
			sortAttrEntries(descAttrs)

			// Convert entities to list
			entityRows := make([]EntityRow, 0, len(rv.Entities))
			for _, entity := range rv.Entities {
				entityIDAttrs := make([]EntityAttributeEntry, 0, len(entity.ID))
				for k, v := range entity.ID {
					entityIDAttrs = append(entityIDAttrs, EntityAttributeEntry{
						Key:   k,
						Value: v,
					})
				}
				sortAttrEntries(entityIDAttrs)

				entityDescAttrs := make([]EntityAttributeEntry, 0, len(entity.Description))
				for k, v := range entity.Description {
					entityDescAttrs = append(entityDescAttrs, EntityAttributeEntry{
						Key:   k,
						Value: v,
					})
				}
				sortAttrEntries(entityDescAttrs)

				entityRows = append(entityRows, EntityRow{
					Type:        entity.Type,
					ID:          entityIDAttrs,
					Description: entityDescAttrs,
				})
			}

			rows = append(rows, metadataRow{
				Namespace:        NamespaceResource,
				LabelsHash:       labelsHash,
				MinTime:          rv.MinTime,
				MaxTime:          rv.MaxTime,
				IdentifyingAttrs: idAttrs,
				DescriptiveAttrs: descAttrs,
				Entities:         entityRows,
			})
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate resources: %w", err)
	}

	// Collect all scopes into rows (one row per version)
	err = mr.IterVersionedScopes(func(labelsHash uint64, vscope *VersionedScope) error {
		for _, sv := range vscope.Versions {
			scopeAttrs := make([]EntityAttributeEntry, 0, len(sv.Attrs))
			for k, v := range sv.Attrs {
				scopeAttrs = append(scopeAttrs, EntityAttributeEntry{Key: k, Value: v})
			}
			sortAttrEntries(scopeAttrs)
			rows = append(rows, metadataRow{
				Namespace:       NamespaceScope,
				LabelsHash:      labelsHash,
				MinTime:         sv.MinTime,
				MaxTime:         sv.MaxTime,
				ScopeName:       sv.Name,
				ScopeVersionStr: sv.Version,
				SchemaURL:       sv.SchemaURL,
				ScopeAttrs:      scopeAttrs,
			})
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate scopes: %w", err)
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

	// Sort rows for better compression: group by namespace, then by labels_hash,
	// then by MinTime. This ensures metadata rows are together, resource rows for the
	// same series are adjacent, and versions are in time order.
	slices.SortFunc(rows, func(a, b metadataRow) int {
		if c := strings.Compare(a.Namespace, b.Namespace); c != 0 {
			return c
		}
		if c := cmp.Compare(a.LabelsHash, b.LabelsHash); c != 0 {
			return c
		}
		return cmp.Compare(a.MinTime, b.MinTime)
	})

	// Count rows by type for footer metadata.
	var metricCount, resourceCount, scopeCount int
	for i := range rows {
		switch rows[i].Namespace {
		case nsMetadataTable, nsMetadataMapping:
			metricCount++
		case NamespaceResource:
			resourceCount++
		case NamespaceScope:
			scopeCount++
		}
	}

	// Write parquet data with zstd compression and footer metadata.
	writer := parquet.NewGenericWriter[metadataRow](f,
		parquet.Compression(&zstd.Codec{Level: zstd.SpeedBetterCompression}),
		parquet.KeyValueMetadata("schema_version", schemaVersion),
		parquet.KeyValueMetadata("metric_count", strconv.Itoa(metricCount)),
		parquet.KeyValueMetadata("resource_count", strconv.Itoa(resourceCount)),
		parquet.KeyValueMetadata("scope_count", strconv.Itoa(scopeCount)),
	)
	if _, err := writer.Write(rows); err != nil {
		return 0, fmt.Errorf("write parquet rows: %w", err)
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
		"metrics", metricCount, "resources", resourceCount, "scopes", scopeCount, "size", size)

	return size, nil
}

// ReadSeriesMetadata reads series metadata from a Parquet file in the given directory.
// If the file does not exist, it returns an empty reader (graceful degradation).
func ReadSeriesMetadata(logger *slog.Logger, dir string) (Reader, int64, error) {
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

	reader, err := newParquetReader(logger, f)
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("create parquet reader: %w", err)
	}

	return reader, stat.Size(), nil
}
