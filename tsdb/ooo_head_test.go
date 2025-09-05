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
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

const testMaxSize int = 32

// Formulas chosen to make testing easy.
func valEven(pos int) int64 { return int64(pos*2 + 2) } // s[0]=2, s[1]=4, s[2]=6, ..., s[31]=64 - Predictable pre-existing values
func valOdd(pos int) int64  { return int64(pos*2 + 1) } // s[0]=1, s[1]=3, s[2]=5, ..., s[31]=63 - New values will interject at chosen position because they sort before the pre-existing vals.

func makeEvenSampleSlice(n int, sampleFunc func(ts int64) sample) []sample {
	s := make([]sample, n)
	for i := range n {
		s[i] = sampleFunc(valEven(i))
	}
	return s
}

// TestOOOInsert tests the following cases:
// - Number of pre-existing samples anywhere from 0 to testMaxSize-1.
// - Insert new sample before first pre-existing samples, after the last, and anywhere in between.
// - With a chunk initial capacity of testMaxSize/8 and testMaxSize, which lets us test non-full and full chunks, and chunks that need to expand themselves.
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
				return sample{t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"float histogram": {
			sampleFunc: func(ts int64) sample {
				return sample{t: ts, fh: tsdbutil.GenerateTestFloatHistogram(ts)}
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

// TestOOOInsertDuplicate tests the correct behavior when inserting a sample that is a duplicate of any
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
				return sample{t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"float histogram": {
			sampleFunc: func(ts int64) sample {
				return sample{t: ts, fh: tsdbutil.GenerateTestFloatHistogram(ts)}
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

type chunkVerify struct {
	encoding chunkenc.Encoding
	minTime  int64
	maxTime  int64
}

func TestOOOChunks_ToEncodedChunks(t *testing.T) {
	h1 := tsdbutil.GenerateTestHistogram(1)
	// Make h2 appendable but with more buckets, to trigger recoding.
	h2 := h1.Copy()
	h2.PositiveSpans = append(h2.PositiveSpans, histogram.Span{Offset: 1, Length: 1})
	h2.PositiveBuckets = append(h2.PositiveBuckets, 12)
	h2explicit := h2.Copy()
	h2explicit.CounterResetHint = histogram.CounterReset

	testCases := map[string]struct {
		samples               []sample
		expectedCounterResets []histogram.CounterResetHint
		expectedChunks        []chunkVerify
	}{
		"empty": {
			samples: []sample{},
		},
		"has floats": {
			samples: []sample{
				{t: 1000, f: 43.0},
				{t: 1100, f: 42.0},
			},
			expectedCounterResets: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.UnknownCounterReset},
			expectedChunks: []chunkVerify{
				{encoding: chunkenc.EncXOR, minTime: 1000, maxTime: 1100},
			},
		},
		"mix of floats and histograms": {
			samples: []sample{
				{t: 1000, f: 43.0},
				{t: 1100, h: h1},
				{t: 1200, f: 42.0},
			},
			expectedCounterResets: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.UnknownCounterReset, histogram.UnknownCounterReset},
			expectedChunks: []chunkVerify{
				{encoding: chunkenc.EncXOR, minTime: 1000, maxTime: 1000},
				{encoding: chunkenc.EncHistogram, minTime: 1100, maxTime: 1100},
				{encoding: chunkenc.EncXOR, minTime: 1200, maxTime: 1200},
			},
		},
		"has an implicit counter reset": {
			samples: []sample{
				{t: 1000, h: h2},
				{t: 1100, h: h1},
			},
			expectedCounterResets: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.UnknownCounterReset},
			expectedChunks: []chunkVerify{
				{encoding: chunkenc.EncHistogram, minTime: 1000, maxTime: 1000},
				{encoding: chunkenc.EncHistogram, minTime: 1100, maxTime: 1100},
			},
		},
		"has an explicit counter reset": {
			samples: []sample{
				{t: 1100, h: h2explicit},
			},
			expectedCounterResets: []histogram.CounterResetHint{histogram.UnknownCounterReset},
			expectedChunks: []chunkVerify{
				{encoding: chunkenc.EncHistogram, minTime: 1100, maxTime: 1100},
			},
		},
		"has an explicit counter reset inside": {
			samples: []sample{
				{t: 1000, h: h1},
				{t: 1100, h: h2explicit},
			},
			expectedCounterResets: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.UnknownCounterReset},
			expectedChunks: []chunkVerify{
				{encoding: chunkenc.EncHistogram, minTime: 1000, maxTime: 1000},
				{encoding: chunkenc.EncHistogram, minTime: 1100, maxTime: 1100},
			},
		},
		"has a recoded histogram": { // Regression test for wrong minT, maxT in histogram recoding.
			samples: []sample{
				{t: 0, h: h1},
				{t: 1, h: h2},
			},
			expectedCounterResets: []histogram.CounterResetHint{histogram.UnknownCounterReset, histogram.NotCounterReset},
			expectedChunks: []chunkVerify{
				{encoding: chunkenc.EncHistogram, minTime: 0, maxTime: 1},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Sanity check.
			require.Len(t, tc.expectedCounterResets, len(tc.samples), "number of samples and counter resets")

			oooChunk := OOOChunk{}
			for _, s := range tc.samples {
				switch s.Type() {
				case chunkenc.ValFloat:
					oooChunk.Insert(s.t, s.f, nil, nil)
				case chunkenc.ValHistogram:
					oooChunk.Insert(s.t, 0, s.h.Copy(), nil)
				case chunkenc.ValFloatHistogram:
					oooChunk.Insert(s.t, 0, nil, s.fh.Copy())
				default:
					t.Fatalf("unexpected sample type %d", s.Type())
				}
			}

			chunks, err := oooChunk.ToEncodedChunks(math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			require.Len(t, chunks, len(tc.expectedChunks), "number of chunks")
			sampleIndex := 0
			for i, c := range chunks {
				require.Equal(t, tc.expectedChunks[i].encoding, c.chunk.Encoding(), "chunk %d encoding", i)
				require.Equal(t, tc.expectedChunks[i].minTime, c.minTime, "chunk %d minTime", i)
				require.Equal(t, tc.expectedChunks[i].maxTime, c.maxTime, "chunk %d maxTime", i)
				samples, err := storage.ExpandSamples(c.chunk.Iterator(nil), newSample)
				require.GreaterOrEqual(t, len(tc.samples)-sampleIndex, len(samples), "too many samples in chunk %d expected less than %d", i, len(tc.samples)-sampleIndex)
				require.NoError(t, err)
				if len(samples) == 0 {
					// Ignore empty chunks.
					continue
				}
				switch c.chunk.Encoding() {
				case chunkenc.EncXOR:
					for j, s := range samples {
						require.Equal(t, chunkenc.ValFloat, s.Type())
						// XOR chunks don't have counter reset hints, so we shouldn't expect anything else than UnknownCounterReset.
						require.Equal(t, histogram.UnknownCounterReset, tc.expectedCounterResets[sampleIndex+j], "sample reset hint %d", sampleIndex+j)
						require.Equal(t, tc.samples[sampleIndex+j].f, s.F(), "sample %d", sampleIndex+j)
					}
				case chunkenc.EncHistogram:
					for j, s := range samples {
						require.Equal(t, chunkenc.ValHistogram, s.Type())
						require.Equal(t, tc.expectedCounterResets[sampleIndex+j], s.H().CounterResetHint, "sample reset hint %d", sampleIndex+j)
						compareTo := tc.samples[sampleIndex+j].h.Copy()
						compareTo.CounterResetHint = tc.expectedCounterResets[sampleIndex+j]
						require.Equal(t, compareTo, s.H().Compact(0), "sample %d", sampleIndex+j)
					}
				case chunkenc.EncFloatHistogram:
					for j, s := range samples {
						require.Equal(t, chunkenc.ValFloatHistogram, s.Type())
						require.Equal(t, tc.expectedCounterResets[sampleIndex+j], s.FH().CounterResetHint, "sample reset hint %d", sampleIndex+j)
						compareTo := tc.samples[sampleIndex+j].fh.Copy()
						compareTo.CounterResetHint = tc.expectedCounterResets[sampleIndex+j]
						require.Equal(t, compareTo, s.FH().Compact(0), "sample %d", sampleIndex+j)
					}
				}
				sampleIndex += len(samples)
			}
			require.Equal(t, len(tc.samples), sampleIndex, "number of samples")
		})
	}
}
