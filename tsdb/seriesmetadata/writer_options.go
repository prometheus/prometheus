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

import "github.com/parquet-go/parquet-go"

// BloomFilterFormat controls how bloom filters are written into the Parquet file.
type BloomFilterFormat int

const (
	// BloomFilterNone disables bloom filter generation.
	BloomFilterNone BloomFilterFormat = iota

	// BloomFilterParquetNative embeds split-block bloom filters in the Parquet
	// file footer for series_ref, content_hash, attr_key, and attr_value columns.
	// This is the current behavior when bloom filters are enabled.
	BloomFilterParquetNative

	// BloomFilterSidecar is reserved for future use: bloom filters written to a
	// separate file for independent store-gateway caching. Not yet implemented.
	// BloomFilterSidecar
)

// WriterOptions configures Parquet write behavior for distributed-scale features.
type WriterOptions struct {
	// MaxRowsPerRowGroup limits rows per row group within a namespace.
	// 0 means no limit (one row group per namespace).
	MaxRowsPerRowGroup int

	// BloomFilterFormat controls bloom filter generation. Use BloomFilterParquetNative
	// to embed split-block bloom filters in the Parquet file. Default (BloomFilterNone)
	// disables bloom filters.
	//
	// Note: the read side in this package does not query bloom filters —
	// it loads all matching row groups into memory. Bloom filter querying
	// is expected to happen in the consumer (e.g. Mimir store-gateway)
	// which knows the query-time predicates.
	BloomFilterFormat BloomFilterFormat

	// EnableInvertedIndex writes resource attribute inverted index rows
	// (namespace=resource_attr_index) into the Parquet file. Each row maps
	// a (key, value) attribute pair to a series ref, enabling O(1) reverse
	// lookup without runtime index build.
	EnableInvertedIndex bool

	// IndexedResourceAttrs specifies additional descriptive resource attribute
	// names to include in the inverted index beyond identifying attributes
	// (which are always indexed). nil means index only identifying attributes.
	IndexedResourceAttrs map[string]struct{}

	// RefResolver converts a labelsHash (the in-memory key) to a block-level
	// seriesRef for Parquet mapping rows. If nil, labelsHash is written
	// directly as SeriesRef (backward compat for head/test writes without
	// a block index).
	RefResolver func(labelsHash uint64) (seriesRef uint64, ok bool)

	// WriteStats is populated after a successful write with namespace row
	// counts from the written Parquet file. This allows the caller to
	// capture stats (e.g. for BlockMeta) without parsing the footer.
	WriteStats *WriteStats
}

// WriteStats contains post-write statistics about the Parquet file.
type WriteStats struct {
	// NamespaceRowCounts maps namespace footer keys (e.g. "resource_table_count")
	// to their row counts.
	NamespaceRowCounts map[string]int
}

// writeNamespaceRows writes rows in chunks of maxPerGroup, calling Flush after
// each chunk to create row group boundaries. If maxPerGroup <= 0 all rows are
// written as a single row group.
func writeNamespaceRows(writer *parquet.GenericWriter[metadataRow], rows []metadataRow, maxPerGroup int) error {
	if len(rows) == 0 {
		return nil
	}

	if maxPerGroup <= 0 {
		if _, err := writer.Write(rows); err != nil {
			return err
		}
		return writer.Flush()
	}

	for len(rows) > 0 {
		chunk := rows
		if len(chunk) > maxPerGroup {
			chunk = rows[:maxPerGroup]
		}
		if _, err := writer.Write(chunk); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		rows = rows[len(chunk):]
	}
	return nil
}
