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

// seriesMetadataRow is the Parquet schema for series metadata.
// Each row represents metadata for a metric, deduplicated by metric name.
type seriesMetadataRow struct {
	// MetricName is the __name__ label value for the metric.
	MetricName string `parquet:"metric_name"`
	// LabelsHash is the stable hash of the series labels (using labels.StableHash).
	// Kept for deduplication during compaction.
	LabelsHash uint64 `parquet:"labels_hash"`
	// Type is the metric type as a string (counter, gauge, histogram, etc.)
	Type string `parquet:"type"`
	// Unit is the metric unit (e.g., "bytes", "seconds")
	Unit string `parquet:"unit"`
	// Help is the metric help text
	Help string `parquet:"help"`
}

// Reader provides read access to series metadata.
type Reader interface {
	// Get returns metadata for the series with the given labels hash.
	// Returns empty metadata and false if not found.
	Get(labelsHash uint64) (metadata.Metadata, bool)

	// GetByMetricName returns all unique metadata entries for the given metric name.
	// Different series with the same metric name may report different metadata
	// (e.g., from different scrape targets). Returns nil and false if not found.
	GetByMetricName(name string) ([]metadata.Metadata, bool)

	// Iter calls the given function for each series metadata entry.
	// Iteration stops early if f returns an error.
	Iter(f func(labelsHash uint64, meta metadata.Metadata) error) error

	// IterByMetricName calls the given function for each metric name and its metadata entries.
	// A metric name may have multiple unique metadata entries from different series.
	// Iteration stops early if f returns an error.
	IterByMetricName(f func(name string, metas []metadata.Metadata) error) error

	// Total returns the total count of unique metric names with metadata.
	Total() uint64

	// Close releases any resources associated with the reader.
	Close() error
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
	// byName maps metric name to a set of unique metadata tuples.
	// Different series with the same metric name may report different metadata.
	byName map[string]map[metadata.Metadata]struct{}
	mtx    sync.RWMutex
}

// NewMemSeriesMetadata creates a new in-memory series metadata store.
func NewMemSeriesMetadata() *MemSeriesMetadata {
	return &MemSeriesMetadata{
		byHash: make(map[uint64]*metadataEntry),
		byName: make(map[string]map[metadata.Metadata]struct{}),
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
// Results are sorted deterministically by Type, Help, Unit.
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
// The metadata is added to the set of unique entries for the metric name.
// If an entry already exists for the same hash with different metadata,
// the old metadata is removed from the byName set before adding the new one.
func (m *MemSeriesMetadata) Set(metricName string, labelsHash uint64, meta metadata.Metadata) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// If this hash already has an entry with different metadata or a different
	// metric name, remove the old metadata from the byName set to avoid orphaned entries.
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
// Entries per metric name are sorted deterministically by Type, Help, Unit.
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

// parquetReader implements Reader by reading from a Parquet file.
type parquetReader struct {
	file   *os.File
	byHash map[uint64]*metadataEntry
	byName map[string][]metadata.Metadata

	closeOnce sync.Once
	closeErr  error
}

// newParquetReader creates a reader from an open file.
func newParquetReader(file *os.File) (*parquetReader, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	// Use the generic Read function which reads all rows at once
	rows, err := parquet.Read[seriesMetadataRow](file, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("read parquet rows: %w", err)
	}

	// Build lookup maps from rows.
	// Parquet stores one row per metric name, so byName has single-element slices.
	byHash := make(map[uint64]*metadataEntry, len(rows))
	byName := make(map[string][]metadata.Metadata, len(rows))
	for i := range rows {
		row := &rows[i]
		meta := metadata.Metadata{
			Type: model.MetricType(row.Type),
			Unit: row.Unit,
			Help: row.Help,
		}
		entry := &metadataEntry{
			metricName: row.MetricName,
			labelsHash: row.LabelsHash,
			meta:       meta,
		}
		if row.LabelsHash != 0 {
			byHash[row.LabelsHash] = entry
		}
		byName[row.MetricName] = []metadata.Metadata{meta}
	}

	return &parquetReader{
		file:   file,
		byHash: byHash,
		byName: byName,
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
func WriteFile(logger *slog.Logger, dir string, mr Reader) (int64, error) {
	path := filepath.Join(dir, SeriesMetadataFilename)
	tmp := path + ".tmp"

	// Collect all metadata into rows, one row per metric name.
	// Parquet stores one metadata entry per metric name; when multiple entries
	// exist (e.g., from different targets), the first is used.
	var rows []seriesMetadataRow
	err := mr.IterByMetricName(func(name string, metas []metadata.Metadata) error {
		if len(metas) == 0 {
			return nil
		}
		meta := metas[0]
		rows = append(rows, seriesMetadataRow{
			MetricName: name,
			LabelsHash: 0, // Hash is not needed in output, but we keep the schema field
			Type:       string(meta.Type),
			Unit:       meta.Unit,
			Help:       meta.Help,
		})
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate metadata by name: %w", err)
	}

	// If no metadata, write an empty file
	if len(rows) == 0 {
		rows = []seriesMetadataRow{}
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
		if tmp != "" {
			if err := os.RemoveAll(tmp); err != nil {
				logger.Error("remove temp file", "err", err.Error())
			}
		}
	}()

	// Write parquet data
	writer := parquet.NewGenericWriter[seriesMetadataRow](f)
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
	tmp = "" // Prevent defer from removing the renamed file

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
