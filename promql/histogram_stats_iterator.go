// Copyright 2024 The Prometheus Authors
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

package promql

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// HistogramStatsIterator is an iterator that returns histogram objects that
// have only their sum and count values populated. The iterator handles counter
// reset detection internally and sets the counter reset hint accordingly in
// each returned histogram object. The Next and Seek methods of the iterator
// will never return ValHistogram, but ValFloatHistogram instead. Effectively,
// the iterator enforces conversion of (integer) Histogram to FloatHistogram.
// The AtHistogram method must not be called (and will panic).
type HistogramStatsIterator struct {
	chunkenc.Iterator

	currentFH *histogram.FloatHistogram
	lastFH    *histogram.FloatHistogram
}

// NewHistogramStatsIterator creates a new HistogramStatsIterator.
func NewHistogramStatsIterator(it chunkenc.Iterator) *HistogramStatsIterator {
	return &HistogramStatsIterator{
		Iterator:  it,
		currentFH: &histogram.FloatHistogram{},
	}
}

// Reset resets this iterator for use with a new underlying iterator, reusing
// objects already allocated where possible.
func (f *HistogramStatsIterator) Reset(it chunkenc.Iterator) {
	f.Iterator = it
	f.lastFH = nil
}

// Next mostly relays to the underlying iterator, but changes a ValHistogram
// return into a ValFloatHistogram return.
func (f *HistogramStatsIterator) Next() chunkenc.ValueType {
	vt := f.Iterator.Next()
	if vt == chunkenc.ValHistogram {
		return chunkenc.ValFloatHistogram
	}
	return vt
}

// Seek mostly relays to the underlying iterator, but changes a ValHistogram
// return into a ValFloatHistogram return.
func (f *HistogramStatsIterator) Seek(t int64) chunkenc.ValueType {
	// If the Seek is going to move the iterator, we have to forget the
	// lastFH.
	if t > f.Iterator.AtT() {
		f.lastFH = nil
	}
	vt := f.Iterator.Seek(t)
	if vt == chunkenc.ValHistogram {
		return chunkenc.ValFloatHistogram
	}
	return vt
}

func (f *HistogramStatsIterator) At() (int64, float64) {
	return f.Iterator.At()
}

// AtHistogram must never be called.
func (*HistogramStatsIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("HistogramStatsIterator.AtHistogram must never be called")
}

// AtFloatHistogram returns the next timestamp/float histogram pair. The method
// performs a counter reset detection on the fly. It will return an explicit
// hint (not UnknownCounterReset) if the previous sample has been accessed with
// the same iterator.
func (f *HistogramStatsIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	var t int64
	t, f.currentFH = f.Iterator.AtFloatHistogram(f.currentFH)
	if value.IsStaleNaN(f.currentFH.Sum) {
		return t, &histogram.FloatHistogram{Sum: f.currentFH.Sum}
	}

	if fh == nil {
		fh = &histogram.FloatHistogram{
			CounterResetHint: f.getFloatResetHint(f.currentFH.CounterResetHint),
			Count:            f.currentFH.Count,
			Sum:              f.currentFH.Sum,
		}
		f.setLastFH(f.currentFH)
		return t, fh
	}

	returnValue := histogram.FloatHistogram{
		CounterResetHint: f.getFloatResetHint(f.currentFH.CounterResetHint),
		Count:            f.currentFH.Count,
		Sum:              f.currentFH.Sum,
	}
	returnValue.CopyTo(fh)

	f.setLastFH(f.currentFH)
	return t, fh
}

func (f *HistogramStatsIterator) setLastFH(fh *histogram.FloatHistogram) {
	if f.lastFH == nil {
		f.lastFH = fh.Copy()
	} else {
		fh.CopyTo(f.lastFH)
	}
}

func (f *HistogramStatsIterator) getFloatResetHint(hint histogram.CounterResetHint) histogram.CounterResetHint {
	if hint != histogram.UnknownCounterReset {
		return hint
	}
	prevFH := f.lastFH
	if prevFH == nil {
		// We don't know if there's a counter reset.
		return histogram.UnknownCounterReset
	}
	if f.currentFH.DetectReset(prevFH) {
		return histogram.CounterReset
	}
	return histogram.NotCounterReset
}
