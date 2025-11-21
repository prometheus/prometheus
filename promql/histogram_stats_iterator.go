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

	current       *histogram.FloatHistogram
	last          *histogram.FloatHistogram
	lastIsCurrent bool
}

// NewHistogramStatsIterator creates a new HistogramStatsIterator.
func NewHistogramStatsIterator(it chunkenc.Iterator) *HistogramStatsIterator {
	return &HistogramStatsIterator{
		Iterator: it,
		current:  &histogram.FloatHistogram{},
	}
}

// Reset resets this iterator for use with a new underlying iterator, reusing
// objects already allocated where possible.
func (hsi *HistogramStatsIterator) Reset(it chunkenc.Iterator) {
	hsi.Iterator = it
	hsi.last = nil
	hsi.lastIsCurrent = false
}

// Next mostly relays to the underlying iterator, but changes a ValHistogram
// return into a ValFloatHistogram return.
func (hsi *HistogramStatsIterator) Next() chunkenc.ValueType {
	hsi.lastIsCurrent = false
	vt := hsi.Iterator.Next()
	if vt == chunkenc.ValHistogram {
		return chunkenc.ValFloatHistogram
	}
	return vt
}

// Seek mostly relays to the underlying iterator, but changes a ValHistogram
// return into a ValFloatHistogram return.
func (hsi *HistogramStatsIterator) Seek(t int64) chunkenc.ValueType {
	// If the Seek is going to move the iterator, we have to forget the
	// lastFH and mark the currentFH as not current anymore.
	if t > hsi.AtT() {
		hsi.last = nil
		hsi.lastIsCurrent = false
	}
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
	populateFH := func(src *histogram.FloatHistogram, detectReset bool) {
		h := histogram.FloatHistogram{
			CounterResetHint: src.CounterResetHint,
			Count:            src.Count,
			Sum:              src.Sum,
		}
		if detectReset {
			h.CounterResetHint = hsi.getResetHint(src.CounterResetHint)
		}
		if fh == nil {
			// Note that we cannot simply write `fh = &h` here
			// because that would let h escape to the heap.
			fh = &histogram.FloatHistogram{}
			*fh = h
		} else {
			h.CopyTo(fh)
		}
	}

	if hsi.lastIsCurrent {
		// Nothing changed since last AtFloatHistogram call. Return a
		// copy of the stored last histogram rather than doing counter
		// reset detection again (which would yield a potentially wrong
		// result of "no counter reset").
		populateFH(hsi.last, false)
		return hsi.AtT(), fh
	}

	var t int64
	t, hsi.current = hsi.Iterator.AtFloatHistogram(hsi.current)
	if value.IsStaleNaN(hsi.current.Sum) {
		populateFH(hsi.current, false)
		return t, fh
	}
	populateFH(hsi.current, true)
	hsi.setLastFromCurrent(fh.CounterResetHint)
	return t, fh
}

// setLastFromCurrent stores a copy of hsi.current as hsi.last. The
// CounterResetHint of hsi.last is set to the provided value, though. This is
// meant to store the value we have calculated on the fly so that we can return
// the same without re-calculation in case AtFloatHistogram is called multiple
// times.
func (hsi *HistogramStatsIterator) setLastFromCurrent(hint histogram.CounterResetHint) {
	if hsi.last == nil {
		hsi.last = hsi.current.Copy()
	} else {
		hsi.current.CopyTo(hsi.last)
	}
	hsi.last.CounterResetHint = hint
	hsi.lastIsCurrent = true
}

func (hsi *HistogramStatsIterator) getResetHint(hint histogram.CounterResetHint) histogram.CounterResetHint {
	if hint != histogram.UnknownCounterReset {
		return hint
	}
	if hsi.last == nil {
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
	if hsi.current.DetectReset(hsi.last) {
		return histogram.CounterReset
	}
	return histogram.NotCounterReset
}
