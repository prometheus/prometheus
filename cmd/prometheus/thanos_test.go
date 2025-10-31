// Copyright 2025 The Prometheus Authors
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

package main

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb"
)

func TestBlockExcludeFilterThanos(t *testing.T) {
	for _, test := range []struct {
		summary    string         // Description of the test case.
		uploaded   []ulid.ULID    // List of blocks marked as uploaded inside the shipper file.
		setupFn    func(string)   // Optional function to run before the test, takes the path to the shipper file.
		meta       tsdb.BlockMeta // Meta of the block we're checking.
		isExcluded bool           // What do we expect to be returned.
	}{
		{
			summary: "missing file",
			setupFn: func(path string) {
				// Delete shipper file to test error handling.
				os.Remove(path)
			},
			meta:       tsdb.BlockMeta{ULID: ulid.MustNew(1, nil)},
			isExcluded: false,
		},
		{
			summary: "corrupt file",
			setupFn: func(path string) {
				// Overwrite the shipper file content with invalid JSON.
				os.WriteFile(path, []byte("{["), 0o644)
			},
			meta:       tsdb.BlockMeta{ULID: ulid.MustNew(1, nil)},
			isExcluded: false,
		},
		{
			summary:    "empty uploaded list",
			uploaded:   []ulid.ULID{},
			meta:       tsdb.BlockMeta{ULID: ulid.MustNew(1, nil)},
			isExcluded: true,
		},
		{
			summary:  "block meta not present in the uploaded list, level=1",
			uploaded: []ulid.ULID{ulid.MustNew(1, nil), ulid.MustNew(3, nil)},
			meta: tsdb.BlockMeta{
				ULID:       ulid.MustNew(2, nil),
				Compaction: tsdb.BlockMetaCompaction{Level: 1},
			},
			isExcluded: true,
		},
		{
			summary:  "block meta not present in the uploaded list, level=2",
			uploaded: []ulid.ULID{ulid.MustNew(1, nil), ulid.MustNew(3, nil)},
			meta: tsdb.BlockMeta{
				ULID:       ulid.MustNew(2, nil),
				Compaction: tsdb.BlockMetaCompaction{Level: 2},
			},
			isExcluded: false,
		},
		{
			summary:    "block meta present in the uploaded list",
			uploaded:   []ulid.ULID{ulid.MustNew(1, nil), ulid.MustNew(2, nil), ulid.MustNew(3, nil)},
			meta:       tsdb.BlockMeta{ULID: ulid.MustNew(2, nil)},
			isExcluded: false,
		},
	} {
		t.Run(test.summary, func(t *testing.T) {
			dir := t.TempDir()
			shipperPath := path.Join(dir, "shipper.json")

			uploaded := make([]string, 0, len(test.uploaded))
			for _, ul := range test.uploaded {
				uploaded = append(uploaded, ul.String())
			}
			ts := ThanosShipperMetaFile{Uploaded: uploaded}
			data, err := json.Marshal(ts)
			require.NoError(t, err, "failed to marshall ThanosShipperMetaFile file")
			require.NoError(t, os.WriteFile(shipperPath, data, 0o644), "failed to write ThanosShipperMetaFile file")

			if test.setupFn != nil {
				test.setupFn(shipperPath)
			}

			fn := exludeBlocksPendingThanosUpload(promslog.NewNopLogger(), shipperPath)
			isExcluded := fn(&test.meta)
			require.Equal(t, test.isExcluded, isExcluded)
		})
	}
}
