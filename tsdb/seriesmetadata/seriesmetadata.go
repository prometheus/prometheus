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
	"sync"

	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"

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

// Namespace constants for the metadataRow discriminator.
const (
	nsMetadataTable   = "metadata_table"   // content-addressed table: unique (type, unit, help) tuples
	nsMetadataMapping = "metadata_mapping" // maps series→versioned metadata: labelsHash, contentHash, minTime, maxTime
)

// metadataRow is the unified Parquet schema for series metadata.
// The Namespace field discriminates the logical row type.
type metadataRow struct {
	Namespace   string `parquet:"namespace"`
	LabelsHash  uint64 `parquet:"labels_hash"`
	MinTime     int64  `parquet:"mint,optional"`
	MaxTime     int64  `parquet:"maxt,optional"`
	ContentHash uint64 `parquet:"content_hash,optional"`
	MetricName  string `parquet:"metric_name,optional"`
	Type        string `parquet:"type,optional"`
	Unit        string `parquet:"unit,optional"`
	Help        string `parquet:"help,optional"`
}

// Reader provides read access to series metadata.
type Reader interface {
	// Get returns metadata for the series with the given labels hash.
	Get(labelsHash uint64) (metadata.Metadata, bool)

	// GetByMetricName returns all unique metadata entries for the given metric name.
	GetByMetricName(name string) ([]metadata.Metadata, bool)

	// Iter calls the given function for each series metadata entry.
	Iter(f func(labelsHash uint64, meta metadata.Metadata) error) error

	// IterByMetricName calls the given function for each metric name and its metadata entries.
	IterByMetricName(f func(name string, metas []metadata.Metadata) error) error

	// Total returns the total count of unique metric names with metadata.
	Total() uint64

	// Close releases any resources associated with the reader.
	Close() error

	// VersionedMetadataReader methods for time-varying metadata.
	VersionedMetadataReader
}

// metadataEntry stores metadata with both hash and metric name for indexing.
type metadataEntry struct {
	metricName string
	labelsHash uint64
	meta       metadata.Metadata
}

// MemSeriesMetadata is an in-memory implementation of series metadata storage.
type MemSeriesMetadata struct {
	// byHash maps labels hash to metadata entry (latest value).
	byHash map[uint64]*metadataEntry
	// byName maps metric name to a set of unique metadata tuples.
	byName map[string]map[metadata.Metadata]struct{}
	// byHashVersioned maps labels hash to versioned metadata.
	byHashVersioned map[uint64]*versionedEntry
	mtx             sync.RWMutex
}

// versionedEntry pairs a metric name with versioned metadata for indexing.
type versionedEntry struct {
	metricName string
	vm         *VersionedMetadata
}

// NewMemSeriesMetadata creates a new in-memory series metadata store.
func NewMemSeriesMetadata() *MemSeriesMetadata {
	return &MemSeriesMetadata{
		byHash:          make(map[uint64]*metadataEntry),
		byName:          make(map[string]map[metadata.Metadata]struct{}),
		byHashVersioned: make(map[uint64]*versionedEntry),
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

// Set stores metadata for the given metric name and labels hash.
func (m *MemSeriesMetadata) Set(metricName string, labelsHash uint64, meta metadata.Metadata) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if labelsHash != 0 {
		if old, ok := m.byHash[labelsHash]; ok && (old.meta != meta || old.metricName != metricName) {
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
		labelsHash: labelsHash,
		meta:       meta,
	}
	if labelsHash != 0 {
		m.byHash[labelsHash] = entry
	}

	if _, ok := m.byName[metricName]; !ok {
		m.byName[metricName] = make(map[metadata.Metadata]struct{})
	}
	m.byName[metricName][meta] = struct{}{}
}

// SetVersioned stores a versioned metadata entry for the given series.
// It also updates the byHash/byName maps with the latest version's metadata.
func (m *MemSeriesMetadata) SetVersioned(metricName string, labelsHash uint64, version *MetadataVersion) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	entry, ok := m.byHashVersioned[labelsHash]
	if !ok {
		entry = &versionedEntry{
			metricName: metricName,
			vm:         &VersionedMetadata{},
		}
		m.byHashVersioned[labelsHash] = entry
	}
	entry.vm.AddOrExtend(version)

	// Update the non-versioned maps with the latest value.
	cur := entry.vm.CurrentVersion()
	if cur != nil {
		m.setLatestLocked(metricName, labelsHash, cur.Meta)
	}
}

// SetVersionedMetadata bulk-sets a complete VersionedMetadata for a series.
// If metricName is empty, only the versioned store is updated (byHash/byName are not touched).
func (m *MemSeriesMetadata) SetVersionedMetadata(labelsHash uint64, metricName string, vm *VersionedMetadata) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if existing, ok := m.byHashVersioned[labelsHash]; ok && metricName == "" {
		metricName = existing.metricName
	}

	m.byHashVersioned[labelsHash] = &versionedEntry{
		metricName: metricName,
		vm:         vm,
	}

	// Update the non-versioned maps with the latest value, but only if we have a metric name.
	if metricName != "" {
		cur := vm.CurrentVersion()
		if cur != nil {
			m.setLatestLocked(metricName, labelsHash, cur.Meta)
		}
	}
}

// setLatestLocked updates byHash/byName with the latest metadata value.
// Caller must hold m.mtx.
func (m *MemSeriesMetadata) setLatestLocked(metricName string, labelsHash uint64, meta metadata.Metadata) {
	if labelsHash != 0 {
		if old, ok := m.byHash[labelsHash]; ok && (old.meta != meta || old.metricName != metricName) {
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
		labelsHash: labelsHash,
		meta:       meta,
	}
	if labelsHash != 0 {
		m.byHash[labelsHash] = entry
	}

	if _, ok := m.byName[metricName]; !ok {
		m.byName[metricName] = make(map[metadata.Metadata]struct{})
	}
	m.byName[metricName][meta] = struct{}{}
}

// Delete removes metadata for the given labels hash.
func (m *MemSeriesMetadata) Delete(labelsHash uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if entry, ok := m.byHash[labelsHash]; ok {
		if set, exists := m.byName[entry.metricName]; exists {
			delete(set, entry.meta)
			if len(set) == 0 {
				delete(m.byName, entry.metricName)
			}
		}
		delete(m.byHash, labelsHash)
	}
	delete(m.byHashVersioned, labelsHash)
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
func (m *MemSeriesMetadata) GetVersionedMetadata(labelsHash uint64) (*VersionedMetadata, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	entry, ok := m.byHashVersioned[labelsHash]
	if !ok {
		return nil, false
	}
	return entry.vm, true
}

// IterVersionedMetadata calls the given function for each series with versioned metadata.
func (m *MemSeriesMetadata) IterVersionedMetadata(f func(labelsHash uint64, metricName string, vm *VersionedMetadata) error) error {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	for hash, entry := range m.byHashVersioned {
		if err := f(hash, entry.metricName, entry.vm); err != nil {
			return err
		}
	}
	return nil
}

// TotalVersionedMetadata returns the total count of series with versioned metadata.
func (m *MemSeriesMetadata) TotalVersionedMetadata() int {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return len(m.byHashVersioned)
}

// parquetReader implements Reader by reading from a Parquet file.
type parquetReader struct {
	file            *os.File
	byHash          map[uint64]*metadataEntry
	byName          map[string][]metadata.Metadata
	byHashVersioned map[uint64]*versionedEntry

	closeOnce sync.Once
	closeErr  error
}

// newParquetReader creates a reader from an open file.
func newParquetReader(file *os.File) (*parquetReader, error) {
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
		labelsHash  uint64
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
				labelsHash:  row.LabelsHash,
				metricName:  row.MetricName,
				contentHash: row.ContentHash,
				minTime:     row.MinTime,
				maxTime:     row.MaxTime,
			})
		}
	}

	// Phase 3: Resolve mappings → versioned metadata.
	byHashVersioned := make(map[uint64]*versionedEntry)
	// Sort mappings by labelsHash then minTime for deterministic construction.
	sort.Slice(mappings, func(i, j int) bool {
		if mappings[i].labelsHash != mappings[j].labelsHash {
			return mappings[i].labelsHash < mappings[j].labelsHash
		}
		return mappings[i].minTime < mappings[j].minTime
	})
	for _, mp := range mappings {
		meta, ok := contentTable[mp.contentHash]
		if !ok {
			continue // Orphaned mapping row; skip.
		}
		entry, ok := byHashVersioned[mp.labelsHash]
		if !ok {
			entry = &versionedEntry{
				metricName: mp.metricName,
				vm:         &VersionedMetadata{},
			}
			byHashVersioned[mp.labelsHash] = entry
		}
		entry.vm.AddOrExtend(&MetadataVersion{
			Meta:    meta,
			MinTime: mp.minTime,
			MaxTime: mp.maxTime,
		})
	}

	// Phase 4: Derive byHash/byName from versioned data (latest version per series).
	byHash := make(map[uint64]*metadataEntry, len(byHashVersioned))
	byNameSet := make(map[string]map[metadata.Metadata]struct{})
	for hash, ve := range byHashVersioned {
		cur := ve.vm.CurrentVersion()
		if cur == nil {
			continue
		}
		byHash[hash] = &metadataEntry{
			metricName: ve.metricName,
			labelsHash: hash,
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

	return &parquetReader{
		file:            file,
		byHash:          byHash,
		byName:          byName,
		byHashVersioned: byHashVersioned,
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

// GetByMetricName returns all metadata entries for the given metric name.
func (r *parquetReader) GetByMetricName(name string) ([]metadata.Metadata, bool) {
	metas, ok := r.byName[name]
	if !ok || len(metas) == 0 {
		return nil, false
	}
	return metas, true
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
func (r *parquetReader) GetVersionedMetadata(labelsHash uint64) (*VersionedMetadata, bool) {
	entry, ok := r.byHashVersioned[labelsHash]
	if !ok {
		return nil, false
	}
	return entry.vm, true
}

// IterVersionedMetadata calls the given function for each series with versioned metadata.
func (r *parquetReader) IterVersionedMetadata(f func(labelsHash uint64, metricName string, vm *VersionedMetadata) error) error {
	for hash, entry := range r.byHashVersioned {
		if err := f(hash, entry.metricName, entry.vm); err != nil {
			return err
		}
	}
	return nil
}

// TotalVersionedMetadata returns the total count of series with versioned metadata.
func (r *parquetReader) TotalVersionedMetadata() int {
	return len(r.byHashVersioned)
}

// Close releases resources associated with the reader.
func (r *parquetReader) Close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.file.Close()
	})
	return r.closeErr
}

// WriteFile atomically writes series metadata to a Parquet file in the given directory.
// Writes two namespaces: "metadata_table" (content-addressed unique tuples)
// and "metadata_mapping" (per-series versioned mappings).
func WriteFile(logger *slog.Logger, dir string, mr Reader) (int64, error) {
	path := filepath.Join(dir, SeriesMetadataFilename)
	tmp := path + ".tmp"

	var rows []metadataRow

	// 1. Build content-addressed table and mapping rows from versioned metadata.
	contentTable := make(map[uint64]metadata.Metadata) // contentHash → metadata
	err := mr.IterVersionedMetadata(func(labelsHash uint64, metricName string, vm *VersionedMetadata) error {
		for _, v := range vm.Versions {
			ch := HashMetadataContent(v.Meta)
			if _, exists := contentTable[ch]; !exists {
				contentTable[ch] = v.Meta
			}
			rows = append(rows, metadataRow{
				Namespace:   nsMetadataMapping,
				LabelsHash:  labelsHash,
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

	// Sort rows by namespace for compression-friendly grouping.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Namespace < rows[j].Namespace
	})

	if len(rows) == 0 {
		rows = []metadataRow{}
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

	writer := parquet.NewGenericWriter[metadataRow](f)
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

	return size, nil
}

// ReadSeriesMetadata reads series metadata from a Parquet file in the given directory.
// If the file does not exist, it returns an empty reader (graceful degradation).
func ReadSeriesMetadata(dir string) (Reader, int64, error) {
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

	reader, err := newParquetReader(f)
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("create parquet reader: %w", err)
	}

	return reader, stat.Size(), nil
}
