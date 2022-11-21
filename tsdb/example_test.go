// Copyright 2021 The Prometheus Authors
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
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func Example() {
	// Create a random dir to work in.  Open() doesn't require a pre-existing dir, but
	// we want to make sure not to make a mess where we shouldn't.
	dir, err := os.MkdirTemp("", "tsdb-test")
	noErr(err)

	// Open a TSDB for reading and/or writing.
	db, err := Open(dir, nil, nil, DefaultOptions(), nil)
	noErr(err)

	// Open an appender for writing.
	app := db.Appender(context.Background())

	series := labels.FromStrings("foo", "bar")

	// Ref is 0 for the first append since we don't know the reference for the series.
	ref, err := app.Append(0, series, time.Now().Unix(), 123)
	noErr(err)

	// Another append for a second later.
	// Re-using the ref from above since it's the same series, makes append faster.
	time.Sleep(time.Second)
	_, err = app.Append(ref, series, time.Now().Unix(), 124)
	noErr(err)

	// Commit to storage.
	err = app.Commit()
	noErr(err)

	// In case you want to do more appends after app.Commit(),
	// you need a new appender.
	app = db.Appender(context.Background())
	// ... adding more samples.

	// Open a querier for reading.
	querier, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
	noErr(err)
	ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

	for ss.Next() {
		series := ss.At()
		fmt.Println("series:", series.Labels().String())

		it := series.Iterator()
		for it.Next() == chunkenc.ValFloat {
			_, v := it.At() // We ignore the timestamp here, only to have a predictable output we can test against (below)
			fmt.Println("sample", v)
		}

		fmt.Println("it.Err():", it.Err())
	}
	fmt.Println("ss.Err():", ss.Err())
	ws := ss.Warnings()
	if len(ws) > 0 {
		fmt.Println("warnings:", ws)
	}
	err = querier.Close()
	noErr(err)

	// Clean up any last resources when done.
	err = db.Close()
	noErr(err)
	err = os.RemoveAll(dir)
	noErr(err)

	// Output:
	// series: {foo="bar"}
	// sample 123
	// sample 124
	// it.Err(): <nil>
	// ss.Err(): <nil>
}

func noErr(err error) {
	if err != nil {
		panic(err)
	}
}
