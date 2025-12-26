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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// SeriesMetadataFilename is the name of the series metadata file in a block directory.
const SeriesMetadataFilename = "series_metadata.parquet"

// Reader provides read access to series metadata.
type Reader interface {
	// Get returns metadata for the series with the given labels hash.
	// Returns empty metadata and false if not found.
	Get(labelsHash uint64) (metadata.Metadata, bool)

	// GetByMetricName returns metadata for the given metric name.
	// Returns empty metadata and false if not found.
	GetByMetricName(name string) (metadata.Metadata, bool)

	// Iter calls the given function for each series metadata entry.
	// Iteration stops early if f returns an error.
	Iter(f func(labelsHash uint64, meta metadata.Metadata) error) error

	// IterByMetricName calls the given function for each metric name and its metadata.
	// Iteration stops early if f returns an error.
	IterByMetricName(f func(name string, meta metadata.Metadata) error) error

	// Total returns the total count of metadata entries.
	Total() uint64

	// Close releases any resources associated with the reader.
	Close() error

	// VersionedResourceAttributesReader provides access to versioned resource attributes.
	// Deprecated: Use VersionedEntityReader instead.
	VersionedResourceAttributesReader

	// VersionedEntityReader provides access to versioned OTel entities.
	VersionedEntityReader
}

// metadataEntry stores metadata with both hash and metric name for indexing.
type metadataEntry struct {
	metricName string
	labelsHash uint64
	meta       metadata.Metadata
}

// MemSeriesMetadata is an in-memory implementation of series metadata storage.
// It is used both as a write buffer and as the return value when no metadata file exists.
type MemSeriesMetadata struct {
	// byHash maps labels hash to metadata entry
	byHash map[uint64]*metadataEntry
	// byName maps metric name to metadata entry (for deduplication)
	byName map[string]*metadataEntry
	mtx    sync.RWMutex

	// resourceAttrs stores resource attributes per series
	// Deprecated: Use entityStore instead.
	resourceAttrs *MemResourceAttributes

	// entityStore stores OTel entities per series
	entityStore *MemEntityStore
}

// NewMemSeriesMetadata creates a new in-memory series metadata store.
func NewMemSeriesMetadata() *MemSeriesMetadata {
	return &MemSeriesMetadata{
		byHash:        make(map[uint64]*metadataEntry),
		byName:        make(map[string]*metadataEntry),
		resourceAttrs: NewMemResourceAttributes(),
		entityStore:   NewMemEntityStore(),
	}
}

// Get returns metadata for the series with the given labels hash.
func (m *MemSeriesMetadata) Get(labelsHash uint64) (metadata.Metadata, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHash[labelsHash]
	if !ok {
		return metadata.Metadata{}, false
	}
	return entry.meta, true
}

// GetByMetricName returns metadata for the given metric name.
func (m *MemSeriesMetadata) GetByMetricName(name string) (metadata.Metadata, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byName[name]
	if !ok {
		return metadata.Metadata{}, false
	}
	return entry.meta, true
}

// Set stores metadata for the given metric name and labels hash.
// If metadata already exists for the metric name, it is overwritten.
func (m *MemSeriesMetadata) Set(metricName string, labelsHash uint64, meta metadata.Metadata) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// Check if we already have an entry for this metric name
	if existing, ok := m.byName[metricName]; ok {
		// Remove old hash mapping if hash changed
		if existing.labelsHash != labelsHash {
			delete(m.byHash, existing.labelsHash)
		}
	}

	entry := &metadataEntry{
		metricName: metricName,
		labelsHash: labelsHash,
		meta:       meta,
	}
	m.byHash[labelsHash] = entry
	m.byName[metricName] = entry
}

// Delete removes metadata for the given labels hash.
func (m *MemSeriesMetadata) Delete(labelsHash uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if entry, ok := m.byHash[labelsHash]; ok {
		delete(m.byName, entry.metricName)
		delete(m.byHash, labelsHash)
	}
}

// Iter calls the given function for each metadata entry by labels hash.
func (m *MemSeriesMetadata) Iter(f func(labelsHash uint64, meta metadata.Metadata) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHash {
		if err := f(hash, entry.meta); err != nil {
			return err
		}
	}
	return nil
}

// IterByMetricName calls the given function for each metric name and its metadata.
func (m *MemSeriesMetadata) IterByMetricName(f func(name string, meta metadata.Metadata) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for name, entry := range m.byName {
		if err := f(name, entry.meta); err != nil {
			return err
		}
	}
	return nil
}

// Total returns the total count of metadata entries.
func (m *MemSeriesMetadata) Total() uint64 {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return uint64(len(m.byName))
}

// Close is a no-op for in-memory storage.
func (*MemSeriesMetadata) Close() error {
	return nil
}

// GetResourceAttributes returns resource attributes for the series with the given labels hash.
func (m *MemSeriesMetadata) GetResourceAttributes(labelsHash uint64) (*ResourceAttributes, bool) {
	return m.resourceAttrs.GetResourceAttributes(labelsHash)
}

// SetResourceAttributes stores resource attributes for the given labels hash.
func (m *MemSeriesMetadata) SetResourceAttributes(labelsHash uint64, attrs *ResourceAttributes) {
	m.resourceAttrs.SetResourceAttributes(labelsHash, attrs)
}

// DeleteResourceAttributes removes resource attributes for the given labels hash.
func (m *MemSeriesMetadata) DeleteResourceAttributes(labelsHash uint64) {
	m.resourceAttrs.DeleteResourceAttributes(labelsHash)
}

// IterResourceAttributes calls the given function for each series' resource attributes.
func (m *MemSeriesMetadata) IterResourceAttributes(f func(labelsHash uint64, attrs *ResourceAttributes) error) error {
	return m.resourceAttrs.IterResourceAttributes(f)
}

// TotalResourceAttributes returns the total count of series with resource attributes.
func (m *MemSeriesMetadata) TotalResourceAttributes() uint64 {
	return m.resourceAttrs.TotalResourceAttributes()
}

// GetVersionedResourceAttributes returns all versions of resource attributes for the series.
func (m *MemSeriesMetadata) GetVersionedResourceAttributes(labelsHash uint64) (*VersionedResourceAttributes, bool) {
	return m.resourceAttrs.GetVersionedResourceAttributes(labelsHash)
}

// GetResourceAttributesAt returns the resource attributes active at the given timestamp.
func (m *MemSeriesMetadata) GetResourceAttributesAt(labelsHash uint64, timestamp int64) (*ResourceAttributes, bool) {
	return m.resourceAttrs.GetResourceAttributesAt(labelsHash, timestamp)
}

// IterVersionedResourceAttributes calls the given function for each series' versioned attributes.
func (m *MemSeriesMetadata) IterVersionedResourceAttributes(f func(labelsHash uint64, attrs *VersionedResourceAttributes) error) error {
	return m.resourceAttrs.IterVersionedResourceAttributes(f)
}

// TotalResourceAttributeVersions returns the total count of all versions across all series.
func (m *MemSeriesMetadata) TotalResourceAttributeVersions() uint64 {
	return m.resourceAttrs.TotalResourceAttributeVersions()
}

// SetVersionedResourceAttributes stores versioned resource attributes for the given labels hash.
func (m *MemSeriesMetadata) SetVersionedResourceAttributes(labelsHash uint64, attrs *VersionedResourceAttributes) {
	m.resourceAttrs.SetVersionedResourceAttributes(labelsHash, attrs)
}

// GetEntity returns the current (latest) entity for the series.
func (m *MemSeriesMetadata) GetEntity(labelsHash uint64) (*Entity, bool) {
	return m.entityStore.GetEntity(labelsHash)
}

// GetVersionedEntity returns all versions of the entity for the series.
func (m *MemSeriesMetadata) GetVersionedEntity(labelsHash uint64) (*VersionedEntity, bool) {
	return m.entityStore.GetVersionedEntity(labelsHash)
}

// GetEntityAt returns the entity version active at the given timestamp.
func (m *MemSeriesMetadata) GetEntityAt(labelsHash uint64, timestamp int64) (*Entity, bool) {
	return m.entityStore.GetEntityAt(labelsHash, timestamp)
}

// SetEntity stores an entity for the series.
func (m *MemSeriesMetadata) SetEntity(labelsHash uint64, entity *Entity) {
	m.entityStore.SetEntity(labelsHash, entity)
}

// SetVersionedEntity stores versioned entities for the series.
func (m *MemSeriesMetadata) SetVersionedEntity(labelsHash uint64, entities *VersionedEntity) {
	m.entityStore.SetVersionedEntity(labelsHash, entities)
}

// DeleteEntity removes all entity data for the series.
func (m *MemSeriesMetadata) DeleteEntity(labelsHash uint64) {
	m.entityStore.DeleteEntity(labelsHash)
}

// IterEntities calls the function for each series' current entity.
func (m *MemSeriesMetadata) IterEntities(f func(labelsHash uint64, entity *Entity) error) error {
	return m.entityStore.IterEntities(f)
}

// IterVersionedEntities calls the function for each series' versioned entities.
func (m *MemSeriesMetadata) IterVersionedEntities(f func(labelsHash uint64, entities *VersionedEntity) error) error {
	return m.entityStore.IterVersionedEntities(f)
}

// TotalEntities returns the count of series with entities.
func (m *MemSeriesMetadata) TotalEntities() uint64 {
	return m.entityStore.TotalEntities()
}

// TotalEntityVersions returns the total count of all entity versions.
func (m *MemSeriesMetadata) TotalEntityVersions() uint64 {
	return m.entityStore.TotalEntityVersions()
}

// parquetReader implements Reader by reading from a Parquet file.
type parquetReader struct {
	file   *os.File
	byHash map[uint64]*metadataEntry
	byName map[string]*metadataEntry

	// resourceAttrs stores resource attributes loaded from the file
	// Deprecated: Use entityStore instead.
	resourceAttrs *MemResourceAttributes

	// entityStore stores OTel entities loaded from the file
	entityStore *MemEntityStore

	closeOnce sync.Once
	closeErr  error
}

// newParquetReader creates a reader from an open file.
func newParquetReader(file *os.File) (*parquetReader, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	// Read all rows using the unified schema
	rows, err := parquet.Read[metadataRow](file, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("read parquet rows: %w", err)
	}

	// Build lookup maps from rows
	byHash := make(map[uint64]*metadataEntry)
	byName := make(map[string]*metadataEntry)
	resourceAttrs := NewMemResourceAttributes()
	entityStore := NewMemEntityStore()

	// Temporary maps to collect versions by labelsHash
	resourceVersionsByHash := make(map[uint64][]*ResourceAttributes)
	entityVersionsByHash := make(map[uint64][]*Entity)

	for i := range rows {
		row := &rows[i]

		switch row.Namespace {
		case NamespaceMetric, "": // Empty namespace for backward compatibility
			if row.MetricName == "" {
				continue // Skip invalid metric rows
			}
			entry := &metadataEntry{
				metricName: row.MetricName,
				labelsHash: row.LabelsHash,
				meta: metadata.Metadata{
					Type: model.MetricType(row.Type),
					Unit: row.Unit,
					Help: row.Help,
				},
			}
			byHash[row.LabelsHash] = entry
			byName[row.MetricName] = entry

		case NamespaceResourceAttrs:
			// Convert Parquet row to ResourceAttributes (deprecated)
			attrs := make(map[string]string, len(row.Attributes))
			for _, attr := range row.Attributes {
				attrs[attr.Key] = attr.Value
			}
			ra := &ResourceAttributes{
				ServiceName:       row.ServiceName,
				ServiceNamespace:  row.ServiceNamespace,
				ServiceInstanceID: row.ServiceInstanceID,
				Attributes:        attrs,
				MinTime:           row.MinTime,
				MaxTime:           row.MaxTime,
			}
			// Collect versions - will be sorted and set below
			resourceVersionsByHash[row.LabelsHash] = append(resourceVersionsByHash[row.LabelsHash], ra)

		case NamespaceEntity:
			// Convert Parquet row to Entity
			id := make(map[string]string, len(row.IdentifyingAttrs))
			for _, attr := range row.IdentifyingAttrs {
				id[attr.Key] = attr.Value
			}
			description := make(map[string]string, len(row.DescriptiveAttrs))
			for _, attr := range row.DescriptiveAttrs {
				description[attr.Key] = attr.Value
			}
			entityType := row.EntityType
			if entityType == "" {
				entityType = EntityTypeResource
			}
			entity := &Entity{
				Type:        entityType,
				Id:          id,
				Description: description,
				MinTime:     row.MinTime,
				MaxTime:     row.MaxTime,
			}
			// Collect versions - will be sorted and set below
			entityVersionsByHash[row.LabelsHash] = append(entityVersionsByHash[row.LabelsHash], entity)
		}
	}

	// Set versioned resource attributes (already sorted by MinTime from WriteFile order)
	for labelsHash, versions := range resourceVersionsByHash {
		resourceAttrs.SetVersionedResourceAttributes(labelsHash, &VersionedResourceAttributes{
			Versions: versions,
		})
	}

	// Set versioned entities (already sorted by MinTime from WriteFile order)
	for labelsHash, versions := range entityVersionsByHash {
		entityStore.SetVersionedEntity(labelsHash, &VersionedEntity{
			Versions: versions,
		})
	}

	return &parquetReader{
		file:          file,
		byHash:        byHash,
		byName:        byName,
		resourceAttrs: resourceAttrs,
		entityStore:   entityStore,
	}, nil
}

// Get returns metadata for the series with the given labels hash.
func (r *parquetReader) Get(labelsHash uint64) (metadata.Metadata, bool) {
	entry, ok := r.byHash[labelsHash]
	if !ok {
		return metadata.Metadata{}, false
	}
	return entry.meta, true
}

// GetByMetricName returns metadata for the given metric name.
func (r *parquetReader) GetByMetricName(name string) (metadata.Metadata, bool) {
	entry, ok := r.byName[name]
	if !ok {
		return metadata.Metadata{}, false
	}
	return entry.meta, true
}

// Iter calls the given function for each metadata entry by labels hash.
func (r *parquetReader) Iter(f func(labelsHash uint64, meta metadata.Metadata) error) error {
	for hash, entry := range r.byHash {
		if err := f(hash, entry.meta); err != nil {
			return err
		}
	}
	return nil
}

// IterByMetricName calls the given function for each metric name and its metadata.
func (r *parquetReader) IterByMetricName(f func(name string, meta metadata.Metadata) error) error {
	for name, entry := range r.byName {
		if err := f(name, entry.meta); err != nil {
			return err
		}
	}
	return nil
}

// Total returns the total count of metadata entries.
func (r *parquetReader) Total() uint64 {
	return uint64(len(r.byName))
}

// GetResourceAttributes returns resource attributes for the series with the given labels hash.
func (r *parquetReader) GetResourceAttributes(labelsHash uint64) (*ResourceAttributes, bool) {
	return r.resourceAttrs.GetResourceAttributes(labelsHash)
}

// IterResourceAttributes calls the given function for each series' resource attributes.
func (r *parquetReader) IterResourceAttributes(f func(labelsHash uint64, attrs *ResourceAttributes) error) error {
	return r.resourceAttrs.IterResourceAttributes(f)
}

// TotalResourceAttributes returns the total count of series with resource attributes.
func (r *parquetReader) TotalResourceAttributes() uint64 {
	return r.resourceAttrs.TotalResourceAttributes()
}

// GetVersionedResourceAttributes returns all versions of resource attributes for the series.
func (r *parquetReader) GetVersionedResourceAttributes(labelsHash uint64) (*VersionedResourceAttributes, bool) {
	return r.resourceAttrs.GetVersionedResourceAttributes(labelsHash)
}

// GetResourceAttributesAt returns the resource attributes active at the given timestamp.
func (r *parquetReader) GetResourceAttributesAt(labelsHash uint64, timestamp int64) (*ResourceAttributes, bool) {
	return r.resourceAttrs.GetResourceAttributesAt(labelsHash, timestamp)
}

// IterVersionedResourceAttributes calls the given function for each series' versioned attributes.
func (r *parquetReader) IterVersionedResourceAttributes(f func(labelsHash uint64, attrs *VersionedResourceAttributes) error) error {
	return r.resourceAttrs.IterVersionedResourceAttributes(f)
}

// TotalResourceAttributeVersions returns the total count of all versions across all series.
func (r *parquetReader) TotalResourceAttributeVersions() uint64 {
	return r.resourceAttrs.TotalResourceAttributeVersions()
}

// GetEntity returns the current (latest) entity for the series.
func (r *parquetReader) GetEntity(labelsHash uint64) (*Entity, bool) {
	return r.entityStore.GetEntity(labelsHash)
}

// GetVersionedEntity returns all versions of the entity for the series.
func (r *parquetReader) GetVersionedEntity(labelsHash uint64) (*VersionedEntity, bool) {
	return r.entityStore.GetVersionedEntity(labelsHash)
}

// GetEntityAt returns the entity version active at the given timestamp.
func (r *parquetReader) GetEntityAt(labelsHash uint64, timestamp int64) (*Entity, bool) {
	return r.entityStore.GetEntityAt(labelsHash, timestamp)
}

// IterEntities calls the function for each series' current entity.
func (r *parquetReader) IterEntities(f func(labelsHash uint64, entity *Entity) error) error {
	return r.entityStore.IterEntities(f)
}

// IterVersionedEntities calls the function for each series' versioned entities.
func (r *parquetReader) IterVersionedEntities(f func(labelsHash uint64, entities *VersionedEntity) error) error {
	return r.entityStore.IterVersionedEntities(f)
}

// TotalEntities returns the count of series with entities.
func (r *parquetReader) TotalEntities() uint64 {
	return r.entityStore.TotalEntities()
}

// TotalEntityVersions returns the total count of all entity versions.
func (r *parquetReader) TotalEntityVersions() uint64 {
	return r.entityStore.TotalEntityVersions()
}

// Close releases resources associated with the reader.
// Safe to call multiple times; only the first call closes the file.
func (r *parquetReader) Close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.file.Close()
	})
	return r.closeErr
}

// WriteFile atomically writes series metadata to a Parquet file in the given directory.
// It follows the same atomic write pattern as tombstones: write to .tmp, then rename.
// Writes both metric metadata and resource attributes using the unified schema.
func WriteFile(logger *slog.Logger, dir string, mr Reader) (int64, error) {
	path := filepath.Join(dir, SeriesMetadataFilename)
	tmp := path + ".tmp"

	var rows []metadataRow

	// Collect all metric metadata into rows, deduplicated by metric name
	seen := make(map[string]struct{})
	err := mr.IterByMetricName(func(name string, meta metadata.Metadata) error {
		if _, ok := seen[name]; ok {
			return nil // Skip duplicates
		}
		seen[name] = struct{}{}
		rows = append(rows, metadataRow{
			Namespace:  NamespaceMetric,
			MetricName: name,
			LabelsHash: 0, // Hash is not strictly needed for metric metadata
			Type:       string(meta.Type),
			Unit:       meta.Unit,
			Help:       meta.Help,
		})
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate metadata by name: %w", err)
	}

	// Collect all resource attributes into rows (one row per version)
	// Deprecated: Use entities instead.
	err = mr.IterVersionedResourceAttributes(func(labelsHash uint64, vattrs *VersionedResourceAttributes) error {
		for _, attrs := range vattrs.Versions {
			// Convert attributes map to list
			attrList := make([]AttributeEntry, 0, len(attrs.Attributes))
			for k, v := range attrs.Attributes {
				attrList = append(attrList, AttributeEntry{
					Key:           k,
					Value:         v,
					IsIdentifying: IsIdentifyingAttribute(k),
				})
			}

			rows = append(rows, metadataRow{
				Namespace:         NamespaceResourceAttrs,
				LabelsHash:        labelsHash,
				MinTime:           attrs.MinTime,
				MaxTime:           attrs.MaxTime,
				ServiceName:       attrs.ServiceName,
				ServiceNamespace:  attrs.ServiceNamespace,
				ServiceInstanceID: attrs.ServiceInstanceID,
				Attributes:        attrList,
			})
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate resource attributes: %w", err)
	}

	// Collect all entities into rows (one row per version)
	err = mr.IterVersionedEntities(func(labelsHash uint64, ventities *VersionedEntity) error {
		for _, entity := range ventities.Versions {
			// Convert identifying attributes to list
			idAttrs := make([]EntityAttributeEntry, 0, len(entity.Id))
			for k, v := range entity.Id {
				idAttrs = append(idAttrs, EntityAttributeEntry{
					Key:   k,
					Value: v,
				})
			}

			// Convert descriptive attributes to list
			descAttrs := make([]EntityAttributeEntry, 0, len(entity.Description))
			for k, v := range entity.Description {
				descAttrs = append(descAttrs, EntityAttributeEntry{
					Key:   k,
					Value: v,
				})
			}

			rows = append(rows, metadataRow{
				Namespace:        NamespaceEntity,
				LabelsHash:       labelsHash,
				MinTime:          entity.MinTime,
				MaxTime:          entity.MaxTime,
				EntityType:       entity.Type,
				IdentifyingAttrs: idAttrs,
				DescriptiveAttrs: descAttrs,
			})
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate entities: %w", err)
	}

	// Create temp file
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
		if err := os.RemoveAll(tmp); err != nil {
			logger.Error("remove temp file", "err", err.Error())
		}
	}()

	// Write parquet data
	writer := parquet.NewGenericWriter[metadataRow](f)
	if _, err := writer.Write(rows); err != nil {
		return 0, fmt.Errorf("write parquet rows: %w", err)
	}
	if err := writer.Close(); err != nil {
		return 0, fmt.Errorf("close parquet writer: %w", err)
	}

	// Sync to disk
	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("sync file: %w", err)
	}

	// Get file size before closing
	stat, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat file: %w", err)
	}
	size := stat.Size()

	// Close the file before rename
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("close file: %w", err)
	}
	f = nil // Prevent double close in defer

	// Atomic rename
	if err := fileutil.Replace(tmp, path); err != nil {
		return 0, fmt.Errorf("rename temp file: %w", err)
	}

	return size, nil
}

// ReadSeriesMetadata reads series metadata from a Parquet file in the given directory.
// If the file does not exist, it returns an empty reader (graceful degradation).
func ReadSeriesMetadata(dir string) (Reader, int64, error) {
	path := filepath.Join(dir, SeriesMetadataFilename)

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		// No metadata file - return empty reader (backward compatibility)
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

	reader, err := newParquetReader(f)
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("create parquet reader: %w", err)
	}

	return reader, stat.Size(), nil
}
