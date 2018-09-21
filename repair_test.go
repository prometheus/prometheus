package tsdb

import (
	"os"
	"testing"

	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/fileutil"
	"github.com/prometheus/tsdb/index"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
)

func TestRepairBadIndexVersion(t *testing.T) {
	// The broken index used in this test was written by the following script
	// at a broken revision.
	//
	// func main() {
	// 	w, err := index.NewWriter("index")
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
	const dbDir = "testdata/repair_index_version/01BZJ9WJQPWHGNC2W4J9TA62KC"
	tmpDir := "testdata/repair_index_version/copy"
	tmpDbDir := tmpDir + "/3MCNSQ8S31EHGJYWK5E1GPJWJZ"

	// Check the current db.
	// In its current state, lookups should fail with the fixed code.
	meta, err := readMetaFile(dbDir)
	testutil.NotOk(t, err)

	// Touch chunks dir in block.
	os.MkdirAll(dbDir+"/chunks", 0777)
	defer os.RemoveAll(dbDir + "/chunks")

	r, err := index.NewFileReader(dbDir + "/index")
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
	defer os.RemoveAll(tmpDir)
	// On DB opening all blocks in the base dir should be repaired.
	db, err := Open(tmpDir, nil, nil, nil)
	testutil.Ok(t, err)
	db.Close()

	r, err = index.NewFileReader(tmpDbDir + "/index")
	testutil.Ok(t, err)
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
		{{"a", "1"}, {"b", "1"}},
		{{"a", "2"}, {"b", "1"}},
	}, res)

	meta, err = readMetaFile(tmpDbDir)
	testutil.Ok(t, err)
	testutil.Assert(t, meta.Version == 1, "unexpected meta version %d", meta.Version)
}
