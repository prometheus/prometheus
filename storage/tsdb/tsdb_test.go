// Copyright 2017 The Prometheus Authors
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
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/tsdb"
	"github.com/prometheus/tsdb/labels"
)

// h.minTime in different scenarios:
// no blocks no WAL: set to the time of the first  appended sample
// no blocks with WAL: set to the smallest sample from the WAL
// with blocks no WAL: set to the last block maxT
// with blocks with WAL: same as above

// TestBlockRanges checks the following use cases:
//  - No samples can be added with timestamps lower than the last block maxt.
//  - The compactor doesn't create overlaping blocks
// even when the last blocks is not within the default boundaries.
// This can happen when a third party tool creates a block from the existing wal.
//  - Lower bondary is based on the smallest sample in the head and
// upper boundary is rounded to the configured block range.
func TestBlockRanges(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_storage")
	if err != nil {
		t.Fatalf("Opening test dir failed: %s", err)
	}

	// Test that the compactor doesn't create overlapping blocks when a non standard block already exists.
	// We create the block manualy so that we don't have a wall and run a separate test for wal after.
	rangeToTriggercompaction := tsdb.DefaultOptions.BlockRanges[0]/2*3 + 1
	createBlock(t, dir, 0, 3)

	lbl := labels.Labels{{"a", "b"}}

	db, err := tsdb.Open(dir, nil, nil, tsdb.DefaultOptions)
	if err != nil {
		t.Fatalf("Opening test storage failed: %s", err)
	}
	defer func() {
		os.RemoveAll(dir)
	}()

	lastBlockMaxT := db.Blocks()[len(db.Blocks())-1].Meta().MaxTime

	app := db.Appender()
	_, err = app.Add(lbl, lastBlockMaxT-1, rand.Float64())
	if err == nil {
		t.Fatalf("appending a sample with a timestamp covered by a previous block shouldn't be possible")
	}

	// Add samples to trigger a new compaction and check that the resulting block doesn't overlap with the last block.
	_, err = app.Add(lbl, lastBlockMaxT+1, rand.Float64())
	Ok(t, err)
	_, err = app.Add(lbl, lastBlockMaxT+2, rand.Float64())
	Ok(t, err)
	_, err = app.Add(lbl, lastBlockMaxT+3+rangeToTriggercompaction, rand.Float64())
	Ok(t, err)

	lastAppendedSample := lastBlockMaxT + 3 + rangeToTriggercompaction

	Ok(t, app.Commit())

	for x := 1; x < 5; x++ {
		if len(db.Blocks()) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(db.Blocks()) != 2 {
		t.Fatal("no new block created after the set timeout")
	}

	if db.Blocks()[0].Meta().MaxTime > db.Blocks()[1].Meta().MinTime {
		t.Fatalf("new block overlaps  old:%v,new:%v", db.Blocks()[0].Meta(), db.Blocks()[1].Meta())
	}

	// Create some blocks offline and ensure that loaded wal doesn't load samples
	// with timestamp lower than the maxt of the latest block.

	// Add more sample that will end up in the wal.

	_, err = app.Add(lbl, lastAppendedSample+1, rand.Float64())
	Ok(t, err)
	_, err = app.Add(lbl, lastAppendedSample+2, rand.Float64())
	Ok(t, err)
	_, err = app.Add(lbl, lastAppendedSample+3, rand.Float64())
	Ok(t, err)
	Ok(t, app.Commit())
	Ok(t, db.Close())

	// Create a block offline that covers some of the existing wal samples timestamp.
	createBlock(t, dir, lastAppendedSample+1, lastAppendedSample+2)

	db, err = tsdb.Open(dir, nil, nil, tsdb.DefaultOptions)
	if err != nil {
		t.Fatalf("Opening test storage failed: %s", err)
	}
	defer db.Close()

	if len(db.Blocks()) != 3 {
		t.Fatal("db doesn't include expected number of blocks")
	}
	if db.Blocks()[2].Meta().MaxTime != lastAppendedSample+2 {
		t.Fatalf("unexpected maxt ofthe last block :%v", db.Blocks()[2].Meta().MaxTime)
	}

	app = db.Appender()

	_, err = app.Add(lbl, lastAppendedSample+4, rand.Float64())
	Ok(t, err)
	_, err = app.Add(lbl, lastAppendedSample+5, rand.Float64())
	Ok(t, err)
	_, err = app.Add(lbl, lastAppendedSample+3+rangeToTriggercompaction, rand.Float64())
	Ok(t, err)

	Ok(t, app.Commit())

	for x := 1; x < 5; x++ {
		if len(db.Blocks()) == 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(db.Blocks()) != 4 {
		t.Fatal("no new block created after the set timeout")
	}

	if db.Blocks()[2].Meta().MaxTime > db.Blocks()[3].Meta().MinTime {
		t.Fatalf("new block overlaps  old:%v,new:%v", db.Blocks()[2].Meta(), db.Blocks()[3].Meta())
	}
}

// createBlock creates a block with nSeries series, and nSamples samples.
func createBlock(tb *testing.T, dir string, mint, maxt int64) {
	head, err := tsdb.NewHead(nil, nil, nil, 2*60*60*1000)
	Ok(tb, err)
	defer head.Close()

	app := head.Appender()

	_, err = app.Add(labels.Labels{{"a", "b"}}, int64(mint), rand.Float64())
	Ok(tb, err)
	_, err = app.Add(labels.Labels{{"a", "b"}}, int64(maxt), rand.Float64())
	Ok(tb, err)

	Ok(tb, app.Commit())

	compactor, err := tsdb.NewLeveledCompactor(nil, log.NewNopLogger(), []int64{1000000}, nil)
	Ok(tb, err)

	Ok(tb, os.MkdirAll(dir, 0777))

	_, err = compactor.Write(dir, head, head.MinTime(), head.MaxTime(), nil)
	Ok(tb, err)

}

// Ok fails the test if an err is not nil.
func Ok(tb *testing.T, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatalf("\033[31munexpected error: %v\033[39m\n", err)
	}
}
