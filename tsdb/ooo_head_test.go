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

func makeEvenSampleSlice(n int, sampleFunc func(st, ts int64) sample) []sample {
	s := make([]sample, n)
	for i := range n {
		ts := valEven(i)
		s[i] = sampleFunc(ts, ts) // Use ts as st for consistency
	}
	return s
}

// TestOOOInsert tests the following cases:
// - Number of pre-existing samples anywhere from 0 to testMaxSize-1.
// - Insert new sample before first pre-existing samples, after the last, and anywhere in between.
// - With a chunk initial capacity of testMaxSize/8 and testMaxSize, which lets us test non-full and full chunks, and chunks that need to expand themselves.
// - With st=0 and st!=0 to verify ordering is based on sample.t, not sample.st.
func TestOOOInsert(t *testing.T) {
	scenarios := map[string]struct {
		sampleFunc func(st, ts int64) sample
	}{
		"float st=0": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: 0, t: ts, f: float64(ts)}
			},
		},
		"float st=ts": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: ts, t: ts, f: float64(ts)}
			},
		},
		"float st=ts-100": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: ts - 100, t: ts, f: float64(ts)}
			},
		},
		"float st descending while t ascending": {
			// st values go in opposite direction of t to ensure ordering is by t.
			sampleFunc: func(st, ts int64) sample {
				return sample{st: 1000 - ts, t: ts, f: float64(ts)}
			},
		},
		"integer histogram st=0": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: 0, t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"integer histogram st=ts": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: ts, t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"float histogram st=0": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: 0, t: ts, fh: tsdbutil.GenerateTestFloatHistogram(ts)}
			},
		},
		"float histogram st=ts": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: ts, t: ts, fh: tsdbutil.GenerateTestFloatHistogram(ts)}
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
	sampleFunc func(st, ts int64) sample,
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
			ts := valOdd(insertPos)
			newSample := sampleFunc(ts, ts) // Use ts as st for consistency
			chunk.Insert(newSample.st, newSample.t, newSample.f, newSample.h, newSample.fh)

			var expSamples []sample
			// Our expected new samples slice, will be first the original samples.
			for i := 0; i < insertPos; i++ {
				ts := valEven(i)
				expSamples = append(expSamples, sampleFunc(ts, ts))
			}
			// Then the new sample.
			expSamples = append(expSamples, newSample)
			// Followed by any original samples that were pushed back by the new one.
			for i := insertPos; i < numPreExisting; i++ {
				ts := valEven(i)
				expSamples = append(expSamples, sampleFunc(ts, ts))
			}

			require.Equal(t, expSamples, chunk.samples, "numPreExisting %d, insertPos %d", numPreExisting, insertPos)
		}
	}
}

// TestOOOInsertDuplicate tests the correct behavior when inserting a sample that is a duplicate of any
// pre-existing samples, with between 1 and testMaxSize pre-existing samples and
// with a chunk initial capacity of testMaxSize/8 and testMaxSize, which lets us test non-full and full chunks, and chunks that need to expand themselves.
// With st=0 and st!=0 to verify duplicate detection is based on sample.t, not sample.st.
func TestOOOInsertDuplicate(t *testing.T) {
	scenarios := map[string]struct {
		sampleFunc func(st, ts int64) sample
	}{
		"float st=0": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: 0, t: ts, f: float64(ts)}
			},
		},
		"float st=ts": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: ts, t: ts, f: float64(ts)}
			},
		},
		"float st=ts-100": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: ts - 100, t: ts, f: float64(ts)}
			},
		},
		"float st descending while t ascending": {
			// st values go in opposite direction of t to ensure duplicate detection is by t.
			sampleFunc: func(st, ts int64) sample {
				return sample{st: 1000 - ts, t: ts, f: float64(ts)}
			},
		},
		"integer histogram st=0": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: 0, t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"integer histogram st=ts": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: ts, t: ts, h: tsdbutil.GenerateTestHistogram(ts)}
			},
		},
		"float histogram st=0": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: 0, t: ts, fh: tsdbutil.GenerateTestFloatHistogram(ts)}
			},
		},
		"float histogram st=ts": {
			sampleFunc: func(st, ts int64) sample {
				return sample{st: ts, t: ts, fh: tsdbutil.GenerateTestFloatHistogram(ts)}
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
	sampleFunc func(st, ts int64) sample,
) {
	for num := 1; num <= testMaxSize; num++ {
		for dupPos := 0; dupPos < num; dupPos++ {
			chunk := NewOOOChunk()
			chunk.samples = makeEvenSampleSlice(num, sampleFunc)

			dupSample := chunk.samples[dupPos]
			dupSample.f = 0.123

			ok := chunk.Insert(dupSample.st, dupSample.t, dupSample.f, dupSample.h, dupSample.fh)

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
					oooChunk.Insert(s.st, s.t, s.f, nil, nil)
				case chunkenc.ValHistogram:
					oooChunk.Insert(s.st, s.t, 0, s.h.Copy(), nil)
				case chunkenc.ValFloatHistogram:
					oooChunk.Insert(s.st, s.t, 0, nil, s.fh.Copy())
				default:
					t.Fatalf("unexpected sample type %d", s.Type())
				}
			}

			chunks, err := oooChunk.ToEncodedChunks(math.MinInt64, math.MaxInt64, false)
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

// TestOOOChunks_ToEncodedChunks_WithST tests ToEncodedChunks with storeST=true and storeST=false for float samples.
// When storeST=true, st values are preserved; when storeST=false, AtST() returns 0.
// TODO(@krajorama): Add histogram test cases once ST storage is implemented for histograms.
func TestOOOChunks_ToEncodedChunks_WithST(t *testing.T) {
	testCases := map[string]struct {
		samples []sample
	}{
		"floats with st=0": {
			samples: []sample{
				{st: 0, t: 1000, f: 43.0},
				{st: 0, t: 1100, f: 42.0},
			},
		},
		"floats with st=t": {
			samples: []sample{
				{st: 1000, t: 1000, f: 43.0},
				{st: 1100, t: 1100, f: 42.0},
			},
		},
		"floats with st=t-100": {
			samples: []sample{
				{st: 900, t: 1000, f: 43.0},
				{st: 1000, t: 1100, f: 42.0},
			},
		},
		"floats with varying st": {
			samples: []sample{
				{st: 500, t: 1000, f: 43.0},
				{st: 1100, t: 1100, f: 42.0}, // st == t
				{st: 0, t: 1200, f: 41.0},    // st == 0
			},
		},
	}

	storageScenarios := []struct {
		name             string
		storeST          bool
		expectedEncoding chunkenc.Encoding
	}{
		{"storeST=true", true, chunkenc.EncodingForFloatST},
		{"storeST=false", false, chunkenc.EncXOR},
	}

	for name, tc := range testCases {
		for _, ss := range storageScenarios {
			t.Run(name+"/"+ss.name, func(t *testing.T) {
				oooChunk := OOOChunk{}
				for _, s := range tc.samples {
					oooChunk.Insert(s.st, s.t, s.f, nil, nil)
				}

				chunks, err := oooChunk.ToEncodedChunks(math.MinInt64, math.MaxInt64, ss.storeST)
				require.NoError(t, err)
				require.Len(t, chunks, 1, "number of chunks")

				c := chunks[0]
				require.Equal(t, ss.expectedEncoding, c.chunk.Encoding(), "chunk encoding")
				require.Equal(t, tc.samples[0].t, c.minTime, "chunk minTime")
				require.Equal(t, tc.samples[len(tc.samples)-1].t, c.maxTime, "chunk maxTime")

				// Verify samples can be read back with correct st and t values.
				it := c.chunk.Iterator(nil)
				sampleIndex := 0
				for it.Next() == chunkenc.ValFloat {
					gotT, gotF := it.At()
					gotST := it.AtST()

					if ss.storeST {
						// When storeST=true, st values should be preserved.
						require.Equal(t, tc.samples[sampleIndex].st, gotST, "sample %d st", sampleIndex)
					} else {
						// When storeST=false, AtST() should return 0.
						require.Equal(t, int64(0), gotST, "sample %d st should be 0 when storeST=false", sampleIndex)
					}
					require.Equal(t, tc.samples[sampleIndex].t, gotT, "sample %d t", sampleIndex)
					require.Equal(t, tc.samples[sampleIndex].f, gotF, "sample %d f", sampleIndex)
					sampleIndex++
				}
				require.Equal(t, len(tc.samples), sampleIndex, "number of samples")
			})
		}
	}
}
