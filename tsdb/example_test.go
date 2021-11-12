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
	"context"
	"fmt"
	"math"

	"github.com/prometheus/prometheus/model/labels"
)

func noErr(err error) {
	if err != nil {
		panic(err)
	}
}

func Example() {
	// Open a TSDB for reading and/or writing.
	db, err := Open("tsdb-test", nil, nil, DefaultOptions(), nil)
	noErr(err)

	// Open an appender for writing.
	app := db.Appender(context.Background())

	series := labels.FromStrings("foo", "bar")

	// Ref is 0 for the first append since we don't know the reference for the series.
	// To make this reproducable we use a fixed timestamp, but you will probably
	// use time.Now().Unix() or similar
	// Fun fact: this beautiful timestamp was Valentine's day in Europe.
	ref, err := app.Append(0, series, 1234567890, 123)
	noErr(err)

	// Another append for a second later.
	// Re-using the ref from above since it's the same series, makes append faster.
	_, err = app.Append(ref, series, 1234567891, 124)
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
		for it.Next() {
			t, v := it.At()
			fmt.Println("sample", t, v)
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

	// Output:
	// series: {foo="bar"}
	// sample 1234567890 123
	// sample 1234567891 124
	// it.Err(): <nil>
	// ss.Err(): <nil>
}
