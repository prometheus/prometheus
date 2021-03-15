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

package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoizedSeriesIterator(t *testing.T) {
	var it *MemoizedSeriesIterator

	sampleEq := func(ets int64, ev float64) {
		ts, v := it.Values()
		require.Equal(t, ets, ts, "timestamp mismatch")
		require.Equal(t, ev, v, "value mismatch")
	}
	prevSampleEq := func(ets int64, ev float64, eok bool) {
		ts, v, ok := it.PeekPrev()
		require.Equal(t, eok, ok, "exist mismatch")
		require.Equal(t, ets, ts, "timestamp mismatch")
		require.Equal(t, ev, v, "value mismatch")
	}

	it = NewMemoizedIterator(NewListSeriesIterator(samples{
		sample{t: 1, v: 2},
		sample{t: 2, v: 3},
		sample{t: 3, v: 4},
		sample{t: 4, v: 5},
		sample{t: 5, v: 6},
		sample{t: 99, v: 8},
		sample{t: 100, v: 9},
		sample{t: 101, v: 10},
	}), 2)

	require.True(t, it.Seek(-123), "seek failed")
	sampleEq(1, 2)
	prevSampleEq(0, 0, false)

	require.True(t, it.Next(), "next failed")
	sampleEq(2, 3)
	prevSampleEq(1, 2, true)

	require.True(t, it.Next(), "next failed")
	require.True(t, it.Next(), "next failed")
	require.True(t, it.Next(), "next failed")
	sampleEq(5, 6)
	prevSampleEq(4, 5, true)

	require.True(t, it.Seek(5), "seek failed")
	sampleEq(5, 6)
	prevSampleEq(4, 5, true)

	require.True(t, it.Seek(101), "seek failed")
	sampleEq(101, 10)
	prevSampleEq(100, 9, true)

	require.False(t, it.Next(), "next succeeded unexpectedly")
}

func BenchmarkMemoizedSeriesIterator(b *testing.B) {
	// Simulate a 5 minute rate.
	it := NewMemoizedIterator(newFakeSeriesIterator(int64(b.N), 30), 5*60)

	b.SetBytes(int64(b.N * 16))
	b.ReportAllocs()
	b.ResetTimer()

	for it.Next() {
		// scan everything
	}
	require.NoError(b, it.Err())
}
