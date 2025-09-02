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
func (hsi *HistogramStatsIterator) Reset(it chunkenc.Iterator) {
	hsi.Iterator = it
	hsi.lastFH = nil
}

// Next mostly relays to the underlying iterator, but changes a ValHistogram
// return into a ValFloatHistogram return.
func (hsi *HistogramStatsIterator) Next() chunkenc.ValueType {
	vt := hsi.Iterator.Next()
	if vt == chunkenc.ValHistogram {
		return chunkenc.ValFloatHistogram
	}
	return vt
}

// Seek mostly relays to the underlying iterator, but changes a ValHistogram
// return into a ValFloatHistogram return.
func (hsi *HistogramStatsIterator) Seek(t int64) chunkenc.ValueType {
	vt := hsi.Iterator.Seek(t)
	if vt == chunkenc.ValHistogram {
		return chunkenc.ValFloatHistogram
	}
	return vt
}

// AtHistogram must never be called.
func (*HistogramStatsIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("HistogramStatsIterator.AtHistogram must never be called")
}

// AtFloatHistogram returns the next timestamp/float histogram pair. The method
// performs a counter reset detection on the fly. It will return an explicit
// hint (not UnknownCounterReset) if the previous sample has been accessed with
// the same iterator.
func (hsi *HistogramStatsIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	var t int64
	t, hsi.currentFH = hsi.Iterator.AtFloatHistogram(hsi.currentFH)
	if value.IsStaleNaN(hsi.currentFH.Sum) {
		return t, &histogram.FloatHistogram{Sum: hsi.currentFH.Sum}
	}

	if fh == nil {
		fh = &histogram.FloatHistogram{
			CounterResetHint: hsi.getFloatResetHint(hsi.currentFH.CounterResetHint),
			Count:            hsi.currentFH.Count,
			Sum:              hsi.currentFH.Sum,
		}
		hsi.setLastFH(hsi.currentFH)
		return t, fh
	}

	returnValue := histogram.FloatHistogram{
		CounterResetHint: hsi.getFloatResetHint(hsi.currentFH.CounterResetHint),
		Count:            hsi.currentFH.Count,
		Sum:              hsi.currentFH.Sum,
	}
	returnValue.CopyTo(fh)

	hsi.setLastFH(hsi.currentFH)
	return t, fh
}

func (hsi *HistogramStatsIterator) setLastFH(fh *histogram.FloatHistogram) {
	if hsi.lastFH == nil {
		hsi.lastFH = fh.Copy()
	} else {
		fh.CopyTo(hsi.lastFH)
	}
}

func (hsi *HistogramStatsIterator) getFloatResetHint(hint histogram.CounterResetHint) histogram.CounterResetHint {
	if hint != histogram.UnknownCounterReset {
		return hint
	}
	prevFH := hsi.lastFH
	if prevFH == nil {
		// We don't know if there's a counter reset. Note that this
		// generally will trigger an explicit counter reset detection by
		// the PromQL engine, which in turn isn't as reliable in this
		// case because the PromQL engine will not see the buckets.
		// However, we can assume that in cases where the counter reset
		// detection is relevant, an iteration through the series has
		// happened, and therefore we do not end up here in the first
		// place.
		return histogram.UnknownCounterReset
	}
	if hsi.currentFH.DetectReset(prevFH) {
		return histogram.CounterReset
	}
	return histogram.NotCounterReset
}
