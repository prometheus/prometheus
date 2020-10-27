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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
)

func TestRepairBadIndexVersion(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, os.RemoveAll(tmpDir))
	})

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
	tmpDbDir := filepath.Join(tmpDir, "01BZJ9WJQPWHGNC2W4J9TA62KC")

	// Create a copy DB to run test against.
	assert.NoError(t, fileutil.CopyDirs(filepath.Join("testdata", "repair_index_version", "01BZJ9WJQPWHGNC2W4J9TA62KC"), tmpDbDir))

	// Check the current db.
	// In its current state, lookups should fail with the fixed code.
	_, _, err = readMetaFile(tmpDbDir)
	assert.Error(t, err)

	// Touch chunks dir in block to imitate them.
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDbDir, "chunks"), 0777))

	// Read current index to check integrity.
	r, err := index.NewFileReader(filepath.Join(tmpDbDir, indexFilename))
	assert.NoError(t, err)
	p, err := r.Postings("b", "1")
	assert.NoError(t, err)
	for p.Next() {
		t.Logf("next ID %d", p.At())

		var lset labels.Labels
		assert.Error(t, r.Series(p.At(), &lset, nil))
	}
	assert.NoError(t, p.Err())
	assert.NoError(t, r.Close())

	// On DB opening all blocks in the base dir should be repaired.
	db, err := Open(tmpDir, nil, nil, nil)
	assert.NoError(t, err)
	db.Close()

	r, err = index.NewFileReader(filepath.Join(tmpDbDir, indexFilename))
	assert.NoError(t, err)
	defer r.Close()
	p, err = r.Postings("b", "1")
	assert.NoError(t, err)
	res := []labels.Labels{}

	for p.Next() {
		t.Logf("next ID %d", p.At())

		var lset labels.Labels
		var chks []chunks.Meta
		assert.NoError(t, r.Series(p.At(), &lset, &chks))
		res = append(res, lset)
	}

	assert.NoError(t, p.Err())
	assert.Equal(t, []labels.Labels{
		{{Name: "a", Value: "1"}, {Name: "b", Value: "1"}},
		{{Name: "a", Value: "2"}, {Name: "b", Value: "1"}},
	}, res)

	meta, _, err := readMetaFile(tmpDbDir)
	assert.NoError(t, err)
	assert.Equal(t, metaVersion1, meta.Version, "unexpected meta version %d", meta.Version)
}
