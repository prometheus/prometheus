// Copyright 2018 The Prometheus Authors
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

package tsdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestRepairBadIndexVersion(t *testing.T) {
	// The broken index used in this test was written by the following script
	// at a broken revision.
	//
	// func main() {
	// 	w, err := index.NewWriter(indexFilename)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	err = w.AddSymbols(map[string]struct{}{
	// 		"a": struct{}{},
	// 		"b": struct{}{},
	// 		"1": struct{}{},
	// 		"2": struct{}{},
	// 	})
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	err = w.AddSeries(1, labels.FromStrings("a", "1", "b", "1"))
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	err = w.AddSeries(2, labels.FromStrings("a", "2", "b", "1"))
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	err = w.WritePostings("b", "1", index.NewListPostings([]uint64{1, 2}))
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	if err := w.Close(); err != nil {
	// 		panic(err)
	// 	}
	// }
	dbDir := filepath.Join("testdata", "repair_index_version", "01BZJ9WJQPWHGNC2W4J9TA62KC")
	tmpDir := filepath.Join("testdata", "repair_index_version", "copy")
	tmpDbDir := filepath.Join(tmpDir, "3MCNSQ8S31EHGJYWK5E1GPJWJZ")

	// Check the current db.
	// In its current state, lookups should fail with the fixed code.
	_, _, err := readMetaFile(dbDir)
	testutil.NotOk(t, err)

	// Touch chunks dir in block.
	testutil.Ok(t, os.MkdirAll(filepath.Join(dbDir, "chunks"), 0777))
	defer func() {
		testutil.Ok(t, os.RemoveAll(filepath.Join(dbDir, "chunks")))
	}()

	r, err := index.NewFileReader(filepath.Join(dbDir, indexFilename))
	testutil.Ok(t, err)
	p, err := r.Postings("b", "1")
	testutil.Ok(t, err)
	for p.Next() {
		t.Logf("next ID %d", p.At())

		var lset labels.Labels
		testutil.NotOk(t, r.Series(p.At(), &lset, nil))
	}
	testutil.Ok(t, p.Err())
	testutil.Ok(t, r.Close())

	// Create a copy DB to run test against.
	if err = fileutil.CopyDirs(dbDir, tmpDbDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		testutil.Ok(t, os.RemoveAll(tmpDir))
	}()
	// On DB opening all blocks in the base dir should be repaired.
	db, err := Open(tmpDir, nil, nil, nil)
	testutil.Ok(t, err)
	db.Close()

	r, err = index.NewFileReader(filepath.Join(tmpDbDir, indexFilename))
	testutil.Ok(t, err)
	defer r.Close()
	p, err = r.Postings("b", "1")
	testutil.Ok(t, err)
	res := []labels.Labels{}

	for p.Next() {
		t.Logf("next ID %d", p.At())

		var lset labels.Labels
		var chks []chunks.Meta
		testutil.Ok(t, r.Series(p.At(), &lset, &chks))
		res = append(res, lset)
	}

	testutil.Ok(t, p.Err())
	testutil.Equals(t, []labels.Labels{
		{{Name: "a", Value: "1"}, {Name: "b", Value: "1"}},
		{{Name: "a", Value: "2"}, {Name: "b", Value: "1"}},
	}, res)

	meta, _, err := readMetaFile(tmpDbDir)
	testutil.Ok(t, err)
	testutil.Assert(t, meta.Version == metaVersion1, "unexpected meta version %d", meta.Version)
}
