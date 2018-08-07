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
	"testing"

	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
)

func TestRecord_EncodeDecode(t *testing.T) {
	var enc RecordEncoder
	var dec RecordDecoder

	series := []RefSeries{
		{
			Ref:    100,
			Labels: labels.FromStrings("abc", "def", "123", "456"),
		}, {
			Ref:    1,
			Labels: labels.FromStrings("abc", "def2", "1234", "4567"),
		}, {
			Ref:    435245,
			Labels: labels.FromStrings("xyz", "def", "foo", "bar"),
		},
	}
	decSeries, err := dec.Series(enc.Series(series, nil), nil)
	testutil.Ok(t, err)
	testutil.Equals(t, series, decSeries)

	samples := []RefSample{
		{Ref: 0, T: 12423423, V: 1.2345},
		{Ref: 123, T: -1231, V: -123},
		{Ref: 2, T: 0, V: 99999},
	}
	decSamples, err := dec.Samples(enc.Samples(samples, nil), nil)
	testutil.Ok(t, err)
	testutil.Equals(t, samples, decSamples)

	// Intervals get split up into single entries. So we don't get back exactly
	// what we put in.
	tstones := []Stone{
		{ref: 123, intervals: Intervals{
			{Mint: -1000, Maxt: 1231231},
			{Mint: 5000, Maxt: 0},
		}},
		{ref: 13, intervals: Intervals{
			{Mint: -1000, Maxt: -11},
			{Mint: 5000, Maxt: 1000},
		}},
	}
	decTstones, err := dec.Tombstones(enc.Tombstones(tstones, nil), nil)
	testutil.Ok(t, err)
	testutil.Equals(t, []Stone{
		{ref: 123, intervals: Intervals{{Mint: -1000, Maxt: 1231231}}},
		{ref: 123, intervals: Intervals{{Mint: 5000, Maxt: 0}}},
		{ref: 13, intervals: Intervals{{Mint: -1000, Maxt: -11}}},
		{ref: 13, intervals: Intervals{{Mint: 5000, Maxt: 1000}}},
	}, decTstones)
}
