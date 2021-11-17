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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestExample(t *testing.T) {
	// Create a random dir to work in.  Open() doesn't require a pre-existing dir, but
	// we want to make sure not to make a mess where we shouldn't.
	dir := t.TempDir()

	// Open a TSDB for reading and/or writing.
	db, err := Open(dir, nil, nil, DefaultOptions(), nil)
	require.NoError(t, err)

	// Open an appender for writing.
	app := db.Appender(context.Background())

	lbls := labels.FromStrings("foo", "bar")
	var appendedSamples []sample

	// Ref is 0 for the first append since we don't know the reference for the series.
	ts, v := time.Now().Unix(), 123.0
	ref, err := app.Append(0, lbls, ts, v)
	require.NoError(t, err)
	appendedSamples = append(appendedSamples, sample{ts, v})

	// Another append for a second later.
	// Re-using the ref from above since it's the same series, makes append faster.
	time.Sleep(time.Second)
	ts, v = time.Now().Unix(), 124
	_, err = app.Append(ref, lbls, ts, v)
	require.NoError(t, err)
	appendedSamples = append(appendedSamples, sample{ts, v})

	// Commit to storage.
	err = app.Commit()
	require.NoError(t, err)

	// In case you want to do more appends after app.Commit(),
	// you need a new appender.
	// app = db.Appender(context.Background())
	//
	// ... adding more samples.
	//
	// Commit to storage.
	// err = app.Commit()
	// require.NoError(t, err)

	// Open a querier for reading.
	querier, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	var queriedSamples []sample
	for ss.Next() {
		series := ss.At()
		fmt.Println("series:", series.Labels().String())

		it := series.Iterator()
		for it.Next() {
			ts, v := it.At()
			fmt.Println("sample", ts, v)
			queriedSamples = append(queriedSamples, sample{ts, v})
		}

		require.NoError(t, it.Err())
		fmt.Println("it.Err():", it.Err())
	}
	require.NoError(t, ss.Err())
	fmt.Println("ss.Err():", ss.Err())
	ws := ss.Warnings()
	if len(ws) > 0 {
		fmt.Println("warnings:", ws)
	}
	err = querier.Close()
	require.NoError(t, err)

	// Clean up any last resources when done.
	err = db.Close()
	require.NoError(t, err)

	require.Equal(t, appendedSamples, queriedSamples)

	// Output:
	// series: {foo="bar"}
	// sample <ts1> 123
	// sample <ts2> 124
	// it.Err(): <nil>
	// ss.Err(): <nil>
}
