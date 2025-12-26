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

	// VersionedResourceReader provides access to versioned OTel resources.
	// Resources include both identifying/descriptive attributes and typed entities.
	VersionedResourceReader
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

	// resourceStore stores OTel resources (attributes + entities) per series
	resourceStore *MemResourceStore
}

// NewMemSeriesMetadata creates a new in-memory series metadata store.
func NewMemSeriesMetadata() *MemSeriesMetadata {
	return &MemSeriesMetadata{
		byHash:        make(map[uint64]*metadataEntry),
		byName:        make(map[string]*metadataEntry),
		resourceStore: NewMemResourceStore(),
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

// parquetReader implements Reader by reading from a Parquet file.
type parquetReader struct {
	file   *os.File
	byHash map[uint64]*metadataEntry
	byName map[string]*metadataEntry

	// resourceStore stores OTel resources (attributes + entities) loaded from the file
	resourceStore *MemResourceStore

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
	resourceStore := NewMemResourceStore()

	// Temporary map to collect resource versions by labelsHash
	resourceVersionsByHash := make(map[uint64][]*ResourceVersion)

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
				entities = append(entities, &Entity{
					Type:        entityType,
					ID:          entityID,
					Description: entityDesc,
				})
			}

			rv := &ResourceVersion{
				Identifying: identifying,
				Descriptive: descriptive,
				Entities:    entities,
				MinTime:     row.MinTime,
				MaxTime:     row.MaxTime,
			}
			resourceVersionsByHash[row.LabelsHash] = append(resourceVersionsByHash[row.LabelsHash], rv)
		}
	}

	// Set versioned resources (already sorted by MinTime from WriteFile order)
	for labelsHash, versions := range resourceVersionsByHash {
		resourceStore.SetVersionedResource(labelsHash, &VersionedResource{
			Versions: versions,
		})
	}

	return &parquetReader{
		file:          file,
		byHash:        byHash,
		byName:        byName,
		resourceStore: resourceStore,
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
// Writes both metric metadata and unified resources (attributes + entities) using the schema.
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

			// Convert resource-level descriptive attributes to list
			descAttrs := make([]EntityAttributeEntry, 0, len(rv.Descriptive))
			for k, v := range rv.Descriptive {
				descAttrs = append(descAttrs, EntityAttributeEntry{
					Key:   k,
					Value: v,
				})
			}

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

				entityDescAttrs := make([]EntityAttributeEntry, 0, len(entity.Description))
				for k, v := range entity.Description {
					entityDescAttrs = append(entityDescAttrs, EntityAttributeEntry{
						Key:   k,
						Value: v,
					})
				}

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
