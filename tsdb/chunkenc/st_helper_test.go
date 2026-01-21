// Copyright The Prometheus Authors
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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

// testChunkSTHandling tests handling of start times in chunks.
// It uses 0-4 samples with timestamp 1000,2000,3000,4000 and monotonically
// increasing start times that are chosen from 0-(ts-500) for each sample.
// All combinations of start times are tested for each number of samples.
func testChunkSTHandling(t *testing.T, vt ValueType, chunkFactory func() Chunk) {
	sampleAppend := func(app Appender, vt ValueType, st, ts int64, v float64) {
		switch vt {
		case ValFloat:
			app.Append(st, ts, v)
		case ValHistogram:
			_, recoded, _, err := app.AppendHistogram(nil, st, ts, &histogram.Histogram{Sum: v, Count: uint64(v * 10)}, false)
			require.NoError(t, err)
			require.False(t, recoded)
		case ValFloatHistogram:
			_, recoded, _, err := app.AppendFloatHistogram(nil, st, ts, &histogram.FloatHistogram{Sum: v, Count: v * 10}, false)
			require.NoError(t, err)
			require.False(t, recoded)
		default:
			t.Fatalf("unsupported value type %v", vt)
		}
	}

	get := func(it Iterator, vt ValueType) (int64, int64, float64) {
		switch vt {
		case ValFloat:
			ts, v := it.At()
			return it.AtST(), ts, v
		case ValHistogram:
			ts, h := it.AtHistogram(nil)
			return it.AtST(), ts, float64(h.Sum)
		case ValFloatHistogram:
			ts, fh := it.AtFloatHistogram(nil)
			return it.AtST(), ts, fh.Sum
		default:
			t.Fatalf("unsupported value type %v", vt)
			return 0, 0, 0
		}
	}

	runCase := func(t *testing.T, samples []triple) {
		chunk := chunkFactory()
		app, err := chunk.Appender()
		require.NoError(t, err)
		for _, s := range samples {
			sampleAppend(app, vt, s.st, s.t, s.v)
		}
		it := chunk.Iterator(nil)
		for i, s := range samples {
			require.Equal(t, vt, it.Next())
			st, ts, f := get(it, vt)
			require.Equal(t, s.t, ts, "%d: timestamp mismatch", i)
			require.Equal(t, s.st, st, "%d: start time mismatch", i)
			require.InDelta(t, s.v, f, 1e-9, "%d: value mismatch", i)
		}
		require.Equal(t, ValNone, it.Next())
		require.NoError(t, it.Err())
	}

	t.Run("manual for debugging", func(t *testing.T) {
		samples := []triple{
			{st: 0, t: 1000, v: 1.5},
			{st: 0, t: 2000, v: 2.5},
			{st: 0, t: 3000, v: 3.5},
			{st: 0, t: 4000, v: 4.5},
		}
		runCase(t, samples)
	})

	stTimes := []int64{0, 500, 1000, 1500, 2000, 2500, 3000, 3500, 4000}
	for numberOfSamples := range 5 {
		samples := make([]triple, numberOfSamples)
		sampleSTidx := make([]int, numberOfSamples)
		for {
			for j := range numberOfSamples {
				samples[j] = triple{
					st: stTimes[sampleSTidx[j]],
					t:  int64(1000 * (j + 1)),
					v:  float64(j) + 0.5,
				}
			}

			t.Run(fmt.Sprintf("%v", samples), func(t *testing.T) {
				runCase(t, samples)
			})

			exhausted := true
			for j := numberOfSamples - 1; j >= 0; j-- {
				if sampleSTidx[j] < j+2 {
					sampleSTidx[j]++
					exhausted = false
					break
				}
				sampleSTidx[j] = 0
			}
			if exhausted {
				break
			}
		}
	}
}
