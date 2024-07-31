// Copyright 2022 The Prometheus Authors
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
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"testing"

	"github.com/stretchr/testify/require"
)

const testMaxSize int = 32

// Formulas chosen to make testing easy.
// Formulas chosen to make testing easy.
func valEven(pos int) int64 { return int64(pos*2 + 2) } // s[0]=2, s[1]=4, s[2]=6, ..., s[31]=64 - Predictable pre-existing values
func valOdd(pos int) int64  { return int64(pos*2 + 1) } // s[0]=1, s[1]=3, s[2]=5, ..., s[31]=63 - New values will interject at chosen position because they sort before the pre-existing vals.

func makeEvenSampleSlice(n int, sampleFunc func(ts int64) sample) []sample {
	s := make([]sample, n)
	for i := 0; i < n; i++ {
		s[i] = sampleFunc(valEven(i))
	}
	return s
}

func TestOOOInsert(t *testing.T) {
	scenarios := map[string]struct {
		sampleFunc func(ts int64) sample
	}{
		"float": {
			sampleFunc: func(ts int64) sample {
				return sample{t: ts, f: float64(ts)}
			},
		},
		"integer histogram": {
			sampleFunc: func(ts int64) sample {
				return sample{t: ts, h: tsdbutil.GenerateTestHistogram(int(ts))}
			},
		},
		"float histogram": {
			sampleFunc: func(ts int64) sample {
				return sample{t: ts, fh: tsdbutil.GenerateTestFloatHistogram(int(ts))}
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			testOOOInsert(t, scenario.sampleFunc)
		})
	}
}

func testOOOInsert(t *testing.T,
	sampleFunc func(ts int64) sample,
) {
	for numPreExisting := 0; numPreExisting <= testMaxSize; numPreExisting++ {
		// For example, if we have numPreExisting 2, then:
		// chunk.samples indexes filled        0   1
		// chunk.samples with these values     2   4     // valEven
		// we want to test inserting at index  0   1   2 // insertPos=0..numPreExisting
		// we can do this by using values      1,  3   5 // valOdd(insertPos)

		for insertPos := 0; insertPos <= numPreExisting; insertPos++ {
			chunk := NewOOOChunk()
			chunk.samples = make([]sample, numPreExisting)
			chunk.samples = makeEvenSampleSlice(numPreExisting, sampleFunc)
			newSample := sampleFunc(valOdd(insertPos))
			chunk.Insert(newSample.t, newSample.f, newSample.h, newSample.fh)

			var expSamples []sample
			// Our expected new samples slice, will be first the original samples.
			for i := 0; i < insertPos; i++ {
				expSamples = append(expSamples, sampleFunc(valEven(i)))
			}
			// Then the new sample.
			expSamples = append(expSamples, newSample)
			// Followed by any original samples that were pushed back by the new one.
			for i := insertPos; i < numPreExisting; i++ {
				expSamples = append(expSamples, sampleFunc(valEven(i)))
			}

			require.Equal(t, expSamples, chunk.samples, "numPreExisting %d, insertPos %d", numPreExisting, insertPos)
		}
	}
}

// pre-existing samples, with between 1 and testMaxSize pre-existing samples and
// with a chunk initial capacity of testMaxSize/8 and testMaxSize, which lets us test non-full and full chunks, and chunks that need to expand themselves.
func TestOOOInsertDuplicate(t *testing.T) {
	scenarios := map[string]struct {
		sampleFunc func(ts int64) sample
	}{
		"float": {
			sampleFunc: func(ts int64) sample {
				return sample{t: ts, f: float64(ts)}
			},
		},
		"integer histogram": {
			sampleFunc: func(ts int64) sample {
				return sample{t: ts, h: tsdbutil.GenerateTestHistogram(int(ts))}
			},
		},
		"float histogram": {
			sampleFunc: func(ts int64) sample {
				return sample{t: ts, fh: tsdbutil.GenerateTestFloatHistogram(int(ts))}
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			testOOOInsertDuplicate(t, scenario.sampleFunc)
		})
	}
}

func testOOOInsertDuplicate(t *testing.T,
	sampleFunc func(ts int64) sample,
) {
	for num := 1; num <= testMaxSize; num++ {
		for dupPos := 0; dupPos < num; dupPos++ {
			chunk := NewOOOChunk()
			chunk.samples = makeEvenSampleSlice(num, sampleFunc)

			dupSample := chunk.samples[dupPos]
			dupSample.f = 0.123

			ok := chunk.Insert(dupSample.t, dupSample.f, dupSample.h, dupSample.fh)

			expSamples := makeEvenSampleSlice(num, sampleFunc) // We expect no change.
			require.False(t, ok)
			require.Equal(t, expSamples, chunk.samples, "num %d, dupPos %d", num, dupPos)
		}
	}
}
