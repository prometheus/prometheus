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
}

// NewMemSeriesMetadata creates a new in-memory series metadata store.
func NewMemSeriesMetadata() *MemSeriesMetadata {
	return &MemSeriesMetadata{
		byHash: make(map[uint64]*metadataEntry),
		byName: make(map[string]*metadataEntry),
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
		if existing.labelsHash != labelsHash && existing.labelsHash != 0 {
			delete(m.byHash, existing.labelsHash)
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

// parquetReader implements Reader by reading from a Parquet file.
type parquetReader struct {
	file   *os.File
	byHash map[uint64]*metadataEntry
	byName map[string]*metadataEntry

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

	// Build lookup maps from rows
	byHash := make(map[uint64]*metadataEntry, len(rows))
	byName := make(map[string]*metadataEntry, len(rows))
	for i := range rows {
		row := &rows[i]
		entry := &metadataEntry{
			metricName: row.MetricName,
			labelsHash: row.LabelsHash,
			meta: metadata.Metadata{
				Type: model.MetricType(row.Type),
				Unit: row.Unit,
				Help: row.Help,
			},
		}
		if row.LabelsHash != 0 {
			byHash[row.LabelsHash] = entry
		}
		byName[row.MetricName] = entry
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

	// Collect all metadata into rows, deduplicated by metric name
	seen := make(map[string]struct{})
	var rows []seriesMetadataRow
	err := mr.IterByMetricName(func(name string, meta metadata.Metadata) error {
		if _, ok := seen[name]; ok {
			return nil // Skip duplicates
		}
		seen[name] = struct{}{}
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
