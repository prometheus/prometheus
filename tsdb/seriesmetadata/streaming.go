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
	"io"
	"log/slog"
	"math"

	"github.com/parquet-go/parquet-go"
)

// OpenParquetFile opens and validates a series metadata Parquet file,
// returning the *parquet.File for reuse across multiple streaming operations.
// The caller is responsible for managing the lifetime of the underlying
// io.ReaderAt.
//
// Schema version validation matches the batch reader: an unexpected version
// logs a warning but does not fail, preserving forward compatibility with
// files written by newer Prometheus versions.
func OpenParquetFile(logger *slog.Logger, r io.ReaderAt, size int64) (*parquet.File, error) {
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
	return pf, nil
}

// --- StreamVersionedResourcesFromFile ---

// StreamResourceOptions configures streaming resource reads.
type StreamResourceOptions struct {
	// SeriesRefFilter skips mapping rows whose SeriesRef does not pass.
	SeriesRefFilter func(seriesRef uint64) bool

	// RefResolver converts seriesRef → labelsHash. If nil, seriesRef is
	// used as labelsHash directly.
	RefResolver func(seriesRef uint64) (labelsHash uint64, ok bool)

	// MinTimeMs / MaxTimeMs filter versions by time range.
	// Defaults are math.MinInt64 / math.MaxInt64 (no filtering).
	MinTimeMs int64
	MaxTimeMs int64

	// OnVersionedResource is called for each labelsHash with its
	// accumulated versions. Called once per row group per labelsHash.
	OnVersionedResource func(labelsHash uint64, vr *VersionedResource) error
}

func defaultStreamResourceOptions() StreamResourceOptions {
	return StreamResourceOptions{
		MinTimeMs: math.MinInt64,
		MaxTimeMs: math.MaxInt64,
	}
}

// StreamResourceOption is a functional option for StreamVersionedResourcesFromFile.
type StreamResourceOption func(*StreamResourceOptions)

// WithSeriesRefFilter sets a filter on SeriesRef for mapping rows.
func WithSeriesRefFilter(fn func(seriesRef uint64) bool) StreamResourceOption {
	return func(o *StreamResourceOptions) { o.SeriesRefFilter = fn }
}

// WithStreamRefResolver sets the seriesRef→labelsHash resolver.
func WithStreamRefResolver(fn func(seriesRef uint64) (labelsHash uint64, ok bool)) StreamResourceOption {
	return func(o *StreamResourceOptions) { o.RefResolver = fn }
}

// WithTimeRange filters resource versions to those overlapping [minMs, maxMs].
// Both bounds are always applied; defaults (math.MinInt64/math.MaxInt64) include all timestamps.
func WithTimeRange(minMs, maxMs int64) StreamResourceOption {
	return func(o *StreamResourceOptions) {
		o.MinTimeMs = minMs
		o.MaxTimeMs = maxMs
	}
}

// WithOnVersionedResource sets the emit callback.
func WithOnVersionedResource(fn func(labelsHash uint64, vr *VersionedResource) error) StreamResourceOption {
	return func(o *StreamResourceOptions) { o.OnVersionedResource = fn }
}

// StreamVersionedResourcesFromFile streams resource data from a pre-opened
// Parquet file. It scans resource_table row groups to build a content table,
// then scans resource_mapping row groups to emit VersionedResource entries
// via the OnVersionedResource callback.
//
// Versions are accumulated per row group and emitted at the end of each
// mapping row group. The caller handles cross-row-group merging if needed.
func StreamVersionedResourcesFromFile(logger *slog.Logger, file *parquet.File, opts ...StreamResourceOption) error {
	o := defaultStreamResourceOptions()
	for _, opt := range opts {
		opt(&o)
	}
	if o.OnVersionedResource == nil {
		return nil
	}

	nsColIdx := lookupColumnIndex(file.Schema(), "namespace")

	// Phase 1: Build content table from resource_table row groups.
	contentTable := make(map[uint64]*ResourceVersion)
	for _, rg := range file.RowGroups() {
		if nsColIdx >= 0 {
			if ns, ok := rowGroupSingleNamespace(rg, nsColIdx); ok && ns != NamespaceResourceTable {
				continue
			}
		}
		rows, err := readRowGroup[metadataRow](rg)
		if err != nil {
			return fmt.Errorf("read resource_table row group: %w", err)
		}
		for i := range rows {
			row := &rows[i]
			if row.Namespace != NamespaceResourceTable {
				continue
			}
			if _, exists := contentTable[row.ContentHash]; !exists {
				contentTable[row.ContentHash] = parseResourceContent(logger, row)
			}
		}
	}

	// Phase 2: Scan resource_mapping row groups.
	versionsByHash := make(map[uint64][]*ResourceVersion)
	for _, rg := range file.RowGroups() {
		if nsColIdx >= 0 {
			if ns, ok := rowGroupSingleNamespace(rg, nsColIdx); ok && ns != NamespaceResourceMapping {
				continue
			}
		}
		rows, err := readRowGroup[metadataRow](rg)
		if err != nil {
			return fmt.Errorf("read resource_mapping row group: %w", err)
		}

		clear(versionsByHash)

		for i := range rows {
			row := &rows[i]
			if row.Namespace != NamespaceResourceMapping {
				continue
			}

			// Apply SeriesRefFilter.
			if o.SeriesRefFilter != nil && !o.SeriesRefFilter(row.SeriesRef) {
				continue
			}

			// Apply time range filter: skip versions that don't overlap [MinTimeMs, MaxTimeMs].
			if row.MaxTime < o.MinTimeMs || row.MinTime > o.MaxTimeMs {
				continue
			}

			// Resolve content.
			template, ok := contentTable[row.ContentHash]
			if !ok {
				logger.Warn("Mapping references unknown content hash",
					"content_hash", row.ContentHash, "series_ref", row.SeriesRef)
				continue
			}

			// Resolve labelsHash.
			labelsHash := row.SeriesRef
			if o.RefResolver != nil {
				lh, resolved := o.RefResolver(row.SeriesRef)
				if !resolved {
					continue
				}
				labelsHash = lh
			}

			rv := copyResourceVersion(template)
			rv.MinTime = row.MinTime
			rv.MaxTime = row.MaxTime

			versionsByHash[labelsHash] = append(versionsByHash[labelsHash], rv)
		}

		// Emit per row group.
		for labelsHash, versions := range versionsByHash {
			if err := o.OnVersionedResource(labelsHash, &VersionedResource{Versions: versions}); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- StreamResourceAttrIndexFromFile ---

// StreamAttrIndexOptions configures streaming attribute index reads.
type StreamAttrIndexOptions struct {
	// OnEntry is called for each (attrKey, attrValue, seriesRef) tuple.
	OnEntry func(attrKey, attrValue string, seriesRef uint64) error

	// RefResolver optionally converts seriesRef → labelsHash in the callback.
	// If nil, raw seriesRef is passed.
	RefResolver func(seriesRef uint64) (labelsHash uint64, ok bool)
}

// StreamAttrIndexOption is a functional option for StreamResourceAttrIndexFromFile.
type StreamAttrIndexOption func(*StreamAttrIndexOptions)

// WithOnAttrIndexEntry sets the emit callback.
func WithOnAttrIndexEntry(fn func(attrKey, attrValue string, seriesRef uint64) error) StreamAttrIndexOption {
	return func(o *StreamAttrIndexOptions) { o.OnEntry = fn }
}

// WithAttrIndexRefResolver sets the seriesRef→labelsHash resolver.
func WithAttrIndexRefResolver(fn func(seriesRef uint64) (labelsHash uint64, ok bool)) StreamAttrIndexOption {
	return func(o *StreamAttrIndexOptions) { o.RefResolver = fn }
}

// StreamResourceAttrIndexFromFile streams resource attribute inverted index
// entries from a pre-opened Parquet file. Prefers dedicated AttrKey/AttrValue
// columns; falls back to IdentifyingAttrs[0] for backward compatibility.
func StreamResourceAttrIndexFromFile(file *parquet.File, opts ...StreamAttrIndexOption) error {
	var o StreamAttrIndexOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.OnEntry == nil {
		return nil
	}

	nsColIdx := lookupColumnIndex(file.Schema(), "namespace")

	for _, rg := range file.RowGroups() {
		if nsColIdx >= 0 {
			if ns, ok := rowGroupSingleNamespace(rg, nsColIdx); ok && ns != NamespaceResourceAttrIndex {
				continue
			}
		}
		rows, err := readRowGroup[metadataRow](rg)
		if err != nil {
			return fmt.Errorf("read resource_attr_index row group: %w", err)
		}

		for i := range rows {
			row := &rows[i]
			if row.Namespace != NamespaceResourceAttrIndex {
				continue
			}

			var attrKey, attrValue string
			switch {
			case row.AttrKey != "":
				attrKey = row.AttrKey
				attrValue = row.AttrValue
			case len(row.IdentifyingAttrs) > 0:
				attrKey = row.IdentifyingAttrs[0].Key
				attrValue = row.IdentifyingAttrs[0].Value
			default:
				continue
			}

			ref := row.SeriesRef
			if o.RefResolver != nil {
				lh, ok := o.RefResolver(row.SeriesRef)
				if !ok {
					continue
				}
				ref = lh
			}

			if err := o.OnEntry(attrKey, attrValue, ref); err != nil {
				return err
			}
		}
	}

	return nil
}
