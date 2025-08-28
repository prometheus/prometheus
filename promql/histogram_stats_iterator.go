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

// HistogramStatsIterator is an iterator that returns histogram objects
// which have only their sum and count values populated. The iterator handles
// counter reset detection internally and sets the counter reset hint accordingly
// in each returned histogram object.
type HistogramStatsIterator struct {
	chunkenc.Iterator

	currentH *histogram.Histogram
	lastH    *histogram.Histogram

	currentFH *histogram.FloatHistogram
	lastFH    *histogram.FloatHistogram

	currentSeriesRead bool
}

// NewHistogramStatsIterator creates a new HistogramStatsIterator.
func NewHistogramStatsIterator(it chunkenc.Iterator) *HistogramStatsIterator {
	return &HistogramStatsIterator{
		Iterator:  it,
		currentH:  &histogram.Histogram{},
		currentFH: &histogram.FloatHistogram{},
	}
}

// Reset resets this iterator for use with a new underlying iterator, reusing
// objects already allocated where possible.
func (hsi *HistogramStatsIterator) Reset(it chunkenc.Iterator) {
	hsi.Iterator = it
	hsi.currentSeriesRead = false
}

// AtHistogram returns the next timestamp/histogram pair. The counter reset
// detection is guaranteed to be correct only when the caller does not switch
// between AtHistogram and AtFloatHistogram calls.
func (hsi *HistogramStatsIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	var t int64
	t, hsi.currentH = hsi.Iterator.AtHistogram(hsi.currentH)
	if value.IsStaleNaN(hsi.currentH.Sum) {
		h = &histogram.Histogram{Sum: hsi.currentH.Sum}
		return t, h
	}

	if h == nil {
		h = &histogram.Histogram{
			CounterResetHint: hsi.getResetHint(hsi.currentH),
			Count:            hsi.currentH.Count,
			Sum:              hsi.currentH.Sum,
		}
		hsi.setLastH(hsi.currentH)
		return t, h
	}

	returnValue := histogram.Histogram{
		CounterResetHint: hsi.getResetHint(hsi.currentH),
		Count:            hsi.currentH.Count,
		Sum:              hsi.currentH.Sum,
	}
	returnValue.CopyTo(h)

	hsi.setLastH(hsi.currentH)
	return t, h
}

// AtFloatHistogram returns the next timestamp/float histogram pair. The counter
// reset detection is guaranteed to be correct only when the caller does not
// switch between AtHistogram and AtFloatHistogram calls.
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

func (hsi *HistogramStatsIterator) setLastH(h *histogram.Histogram) {
	hsi.lastFH = nil
	if hsi.lastH == nil {
		hsi.lastH = h.Copy()
	} else {
		h.CopyTo(hsi.lastH)
	}

	hsi.currentSeriesRead = true
}

func (hsi *HistogramStatsIterator) setLastFH(fh *histogram.FloatHistogram) {
	hsi.lastH = nil
	if hsi.lastFH == nil {
		hsi.lastFH = fh.Copy()
	} else {
		fh.CopyTo(hsi.lastFH)
	}

	hsi.currentSeriesRead = true
}

func (hsi *HistogramStatsIterator) getFloatResetHint(hint histogram.CounterResetHint) histogram.CounterResetHint {
	if hint != histogram.UnknownCounterReset {
		return hint
	}
	prevFH := hsi.lastFH
	if prevFH == nil || !hsi.currentSeriesRead {
		if hsi.lastH == nil || !hsi.currentSeriesRead {
			// We don't know if there's a counter reset.
			return histogram.UnknownCounterReset
		}
		prevFH = hsi.lastH.ToFloat(nil)
	}
	if hsi.currentFH.DetectReset(prevFH) {
		return histogram.CounterReset
	}
	return histogram.NotCounterReset
}

func (hsi *HistogramStatsIterator) getResetHint(h *histogram.Histogram) histogram.CounterResetHint {
	if h.CounterResetHint != histogram.UnknownCounterReset {
		return h.CounterResetHint
	}
	var prevFH *histogram.FloatHistogram
	if hsi.lastH == nil || !hsi.currentSeriesRead {
		if hsi.lastFH == nil || !hsi.currentSeriesRead {
			// We don't know if there's a counter reset. Note that
			// this generally will trigger an explicit counter reset
			// detection by the PromQL engine, which in turn isn't
			// as reliable in this case because the PromQL engine
			// will not see the buckets. However, we can assume that
			// in cases where the counter reset detection is
			// relevant, an iteration through the series has
			// happened, and therefore we do not end up here in the
			// first place.
			return histogram.UnknownCounterReset
		}
		prevFH = hsi.lastFH
	} else {
		prevFH = hsi.lastH.ToFloat(nil)
	}
	fh := h.ToFloat(nil)
	if fh.DetectReset(prevFH) {
		return histogram.CounterReset
	}
	return histogram.NotCounterReset
}
