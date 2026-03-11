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

package scrape

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

func TestSynthesizeNumber_ValidCounter(t *testing.T) {
	st := &startTimeSynthesis{}

	// Scrape loop anchors first sample
	n := st.ensureNumberSynthesis()
	n.prevValue = 10.0
	n.refValue = 10.0
	n.startTime = 1000

	// Second scrape, no reset
	v, ct, skip := st.synthesizeNumber(15.0, 2000)
	require.Equal(t, 5.0, v)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)

	// Third scrape, no reset
	v, ct, skip = st.synthesizeNumber(20.0, 3000)
	require.Equal(t, 10.0, v)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)
}

func TestSynthesizeNumber_CounterReset(t *testing.T) {
	st := &startTimeSynthesis{}

	// Scrape loop anchors first sample
	n := st.ensureNumberSynthesis()
	n.prevValue = 100.0
	n.refValue = 100.0
	n.startTime = 1000

	// First reset (value goes down)
	v, ct, skip := st.synthesizeNumber(5.0, 2000)
	require.Equal(t, 5.0, v)
	require.Equal(t, int64(1999), ct)
	require.False(t, skip)
	require.Equal(t, 5.0, st.number.prevValue)
	require.Equal(t, 0.0, st.number.refValue)

	// Increment
	v, ct, skip = st.synthesizeNumber(15.0, 3000)
	require.Equal(t, 15.0, v)
	require.Equal(t, int64(1999), ct)
	require.False(t, skip)
}

func TestSynthesizeFloatHistogram_ValidAndReset(t *testing.T) {
	st := &startTimeSynthesis{}

	// Scrape loop anchors first sample
	fh := st.ensureFloatHistogramSynthesis()
	fh.prevSum = 50.5
	fh.prevCount = 10
	fh.prevZeroCount = 2
	fh.refSum = 50.5
	fh.refCount = 10
	fh.refZeroCount = 2
	fh.startTime = 1000

	// Next scrape
	fh2 := &histogram.FloatHistogram{
		Count:     25,
		Sum:       120.0,
		ZeroCount: 5,
	}
	v, ct, skip := st.synthesizeFloatHistogram(fh2, 2000)
	require.Equal(t, 15.0, v.Count)
	require.Equal(t, 69.5, v.Sum)
	require.Equal(t, 3.0, v.ZeroCount)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)

	// Reset scrape (values go down)
	fh3 := &histogram.FloatHistogram{
		Count:     5,
		Sum:       12.0,
		ZeroCount: 1,
	}
	v, ct, skip = st.synthesizeFloatHistogram(fh3, 3000)
	require.Equal(t, 5.0, v.Count)
	require.Equal(t, 12.0, v.Sum)
	require.Equal(t, 1.0, v.ZeroCount)
	require.Equal(t, int64(2999), ct)
	require.False(t, skip)
	require.Equal(t, uint64(0), st.floatHistogram.refCount)
	require.Equal(t, 0.0, st.floatHistogram.refSum)
}

func TestSynthesizeHistogram_ValidAndReset(t *testing.T) {
	st := &startTimeSynthesis{}

	// Scrape loop anchors first sample
	h := st.ensureHistogramSynthesis()
	h.prevSum = 50.5
	h.prevCount = 10
	h.prevZeroCount = 2
	h.refSum = 50.5
	h.refCount = 10
	h.refZeroCount = 2
	h.startTime = 1000

	// Next scrape
	h2 := &histogram.Histogram{
		Count:     25,
		Sum:       120.0,
		ZeroCount: 5,
	}
	v, ct, skip := st.synthesizeHistogram(h2, 2000)
	require.Equal(t, uint64(15), v.Count)
	require.Equal(t, 69.5, v.Sum)
	require.Equal(t, uint64(3), v.ZeroCount)
	require.Equal(t, int64(1000), ct)
	require.False(t, skip)

	// Reset scrape (values go down)
	h3 := &histogram.Histogram{
		Count:     5,
		Sum:       12.0,
		ZeroCount: 1,
	}
	v, ct, skip = st.synthesizeHistogram(h3, 3000)
	require.Equal(t, uint64(5), v.Count)
	require.Equal(t, 12.0, v.Sum)
	require.Equal(t, uint64(1), v.ZeroCount)
	require.Equal(t, int64(2999), ct)
	require.False(t, skip)
	require.Equal(t, uint64(0), st.histogram.refCount)
	require.Equal(t, 0.0, st.histogram.refSum)
}
