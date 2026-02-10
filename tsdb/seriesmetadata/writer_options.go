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

// WriterOptions configures Parquet write behavior for distributed-scale features.
type WriterOptions struct {
	// MaxRowsPerRowGroup limits rows per row group within a namespace.
	// 0 means no limit (one row group per namespace).
	MaxRowsPerRowGroup int

	// EnableBloomFilters adds split-block bloom filters to labels_hash and
	// content_hash columns. Increases file size slightly but enables
	// store-gateway to skip row groups without deserialization.
	//
	// Note: the read side in this package does not query bloom filters —
	// it loads all matching row groups into memory. Bloom filter querying
	// is expected to happen in the consumer (e.g. Mimir store-gateway)
	// which knows the query-time predicates.
	EnableBloomFilters bool
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
