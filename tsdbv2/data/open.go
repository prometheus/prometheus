// Copyright 2024 The Prometheus Authors

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

package data

import (
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"

	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
)

// Open opens a pebble database. [bitmaps.Merge] is set as merge operator.
func Open(databasePath string, opts *pebble.Options) (db *pebble.DB, _ error) {
	if opts == nil {
		opts = &pebble.Options{FS: vfs.Default}
	}
	opts.Merger = bitmaps.Merge
	return pebble.Open(databasePath, opts)
}

// Test creates a test pebble database.
func Test(t testing.TB) *pebble.DB {
	t.Helper()
	db, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	return db
}
