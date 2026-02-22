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
	"io"
	"log/slog"
)

// ReaderOption configures Parquet read behavior.
type ReaderOption func(*readerOptions)

type readerOptions struct {
	namespaceFilter map[string]struct{}
	refResolver     func(seriesRef uint64) (labelsHash uint64, ok bool)
}

// WithNamespaceFilter restricts loading to rows matching the given namespaces.
// When set, row groups for non-matching namespaces are skipped entirely (for
// files written with namespace-partitioned row groups). For single-row-group
// files, all rows are read but only matching namespaces are processed.
//
// Note: resource_mapping rows require resource_table rows to resolve;
// include both when filtering for resources.
func WithNamespaceFilter(namespaces ...string) ReaderOption {
	return func(o *readerOptions) {
		o.namespaceFilter = make(map[string]struct{}, len(namespaces))
		for _, ns := range namespaces {
			o.namespaceFilter[ns] = struct{}{}
		}
	}
}

// WithRefResolver provides a function that converts a block-level seriesRef
// (stored in Parquet mapping rows) back to a labelsHash for in-memory lookups.
// If not set, SeriesRef values are used as-is (handles head-written files
// where seriesRef == labelsHash).
func WithRefResolver(fn func(seriesRef uint64) (labelsHash uint64, ok bool)) ReaderOption {
	return func(o *readerOptions) {
		o.refResolver = fn
	}
}

// WithResourceAttrIndexOnly loads only the resource attribute inverted index.
// Useful for Mimir store-gateway reverse-lookup queries that only need the index.
func WithResourceAttrIndexOnly() ReaderOption {
	return WithNamespaceFilter(NamespaceResourceAttrIndex)
}

// WithResourceData loads resource tables and mappings (no inverted index).
func WithResourceData() ReaderOption {
	return WithNamespaceFilter(NamespaceResourceTable, NamespaceResourceMapping)
}

// WithScopeData loads scope tables and mappings.
func WithScopeData() ReaderOption {
	return WithNamespaceFilter(NamespaceScopeTable, NamespaceScopeMapping)
}

// WithFullResourceData loads resource tables, mappings, and inverted index.
func WithFullResourceData() ReaderOption {
	return WithNamespaceFilter(NamespaceResourceTable, NamespaceResourceMapping, NamespaceResourceAttrIndex)
}

// ReadSeriesMetadataFromReaderAt reads series metadata from an io.ReaderAt.
// This is the API for distributed systems like Mimir that provide
// objstore.Bucket-backed readers. The caller is responsible for closing
// the underlying reader.
func ReadSeriesMetadataFromReaderAt(logger *slog.Logger, r io.ReaderAt, size int64, opts ...ReaderOption) (Reader, error) {
	return newParquetReaderFromReaderAt(logger, r, size, opts...)
}
