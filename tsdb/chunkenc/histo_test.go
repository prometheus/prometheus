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

package chunkenc

import (
	"testing"

	"github.com/prometheus/prometheus/pkg/histogram"
	"github.com/stretchr/testify/require"
)

func TestHistoChunkSameBuckets(t *testing.T) {

	c := NewHistoChunk()

	type res struct {
		t int64
		h histogram.SparseHistogram
	}

	// create fresh appender and add the first histogram

	app, err := c.Appender()
	require.NoError(t, err)
	require.Equal(t, 0, c.NumSamples())

	ts := int64(1234567890)

	h := histogram.SparseHistogram{
		Count:     5,
		ZeroCount: 2,
		Sum:       18.4,
		//ZeroThreshold: 1, TODO
		Schema: 1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 1, -1, 0}, // counts: 1, 2, 1, 1 (total 5)
		NegativeSpans:   nil,
		NegativeBuckets: []int64{},
	}

	app.AppendHistogram(ts, h)
	require.Equal(t, 1, c.NumSamples())

	exp := []res{
		{t: ts, h: h},
	}

	// add an updated histogram

	ts += 16
	h.Count += 9
	h.ZeroCount++
	h.Sum = 24.4
	h.PositiveBuckets = []int64{5, -2, 1, -2} // counts: 5, 3, 4, 2 (total 14)

	app.AppendHistogram(ts, h)
	exp = append(exp, res{t: ts, h: h})

	require.Equal(t, 2, c.NumSamples())

	// add update with new appender

	app, err = c.Appender()
	require.NoError(t, err)
	require.Equal(t, 2, c.NumSamples())

	ts += 14
	h.Count += 13
	h.ZeroCount += 2
	h.Sum = 24.4
	h.PositiveBuckets = []int64{6, 1, -3, 6} // counts: 6, 7, 4, 10 (total 27)

	app.AppendHistogram(ts, h)
	exp = append(exp, res{t: ts, h: h})

	require.Equal(t, 3, c.NumSamples())

	// 1. Expand iterator in simple case.
	it1 := c.iterator(nil)
	require.NoError(t, it1.Err())
	var res1 []res
	for it1.Next() {
		ts, h := it1.AtHistogram()
		res1 = append(res1, res{t: ts, h: h.Copy()})
	}
	require.NoError(t, it1.Err())
	require.Equal(t, exp, res1)

	// 2. Expand second iterator while reusing first one.
	//it2 := c.Iterator(it1)
	//var res2 []pair
	//for it2.Next() {
	//	ts, v := it2.At()
	//	res2 = append(res2, pair{t: ts, v: v})
	//	}
	//	require.NoError(t, it2.Err())
	//	require.Equal(t, exp, res2)

	// 3. Test iterator Seek.
	//	mid := len(exp) / 2

	//	it3 := c.Iterator(nil)
	//	var res3 []pair
	//	require.Equal(t, true, it3.Seek(exp[mid].t))
	// Below ones should not matter.
	//	require.Equal(t, true, it3.Seek(exp[mid].t))
	//	require.Equal(t, true, it3.Seek(exp[mid].t))
	//	ts, v = it3.At()
	//	res3 = append(res3, pair{t: ts, v: v})

	//	for it3.Next() {
	//		ts, v := it3.At()
	//		res3 = append(res3, pair{t: ts, v: v})
	//	}
	//	require.NoError(t, it3.Err())
	//	require.Equal(t, exp[mid:], res3)
	//	require.Equal(t, false, it3.Seek(exp[len(exp)-1].t+1))
}
