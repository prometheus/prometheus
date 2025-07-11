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
func (f *HistogramStatsIterator) Reset(it chunkenc.Iterator) {
	f.Iterator = it
	f.currentSeriesRead = false
}

// AtHistogram returns the next timestamp/histogram pair. The counter reset
// detection is guaranteed to be correct only when the caller does not switch
// between AtHistogram and AtFloatHistogram calls.
func (f *HistogramStatsIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	var t int64
	t, f.currentH = f.Iterator.AtHistogram(f.currentH)
	if value.IsStaleNaN(f.currentH.Sum) {
		h = &histogram.Histogram{Sum: f.currentH.Sum}
		return t, h
	}

	if h == nil {
		h = &histogram.Histogram{
			CounterResetHint: f.getResetHint(f.currentH),
			Count:            f.currentH.Count,
			Sum:              f.currentH.Sum,
		}
		f.setLastH(f.currentH)
		return t, h
	}

	returnValue := histogram.Histogram{
		CounterResetHint: f.getResetHint(f.currentH),
		Count:            f.currentH.Count,
		Sum:              f.currentH.Sum,
	}
	returnValue.CopyTo(h)

	f.setLastH(f.currentH)
	return t, h
}

// AtFloatHistogram returns the next timestamp/float histogram pair. The counter
// reset detection is guaranteed to be correct only when the caller does not
// switch between AtHistogram and AtFloatHistogram calls.
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

func (f *HistogramStatsIterator) setLastH(h *histogram.Histogram) {
	f.lastFH = nil
	if f.lastH == nil {
		f.lastH = h.Copy()
	} else {
		h.CopyTo(f.lastH)
	}

	f.currentSeriesRead = true
}

func (f *HistogramStatsIterator) setLastFH(fh *histogram.FloatHistogram) {
	f.lastH = nil
	if f.lastFH == nil {
		f.lastFH = fh.Copy()
	} else {
		fh.CopyTo(f.lastFH)
	}

	f.currentSeriesRead = true
}

func (f *HistogramStatsIterator) getFloatResetHint(hint histogram.CounterResetHint) histogram.CounterResetHint {
	if hint != histogram.UnknownCounterReset {
		return hint
	}
	prevFH := f.lastFH
	if prevFH == nil || !f.currentSeriesRead {
		if f.lastH == nil || !f.currentSeriesRead {
			// We don't know if there's a counter reset.
			return histogram.UnknownCounterReset
		}
		prevFH = f.lastH.ToFloat(nil)
	}
	if f.currentFH.DetectReset(prevFH) {
		return histogram.CounterReset
	}
	return histogram.NotCounterReset
}

func (f *HistogramStatsIterator) getResetHint(h *histogram.Histogram) histogram.CounterResetHint {
	if h.CounterResetHint != histogram.UnknownCounterReset {
		return h.CounterResetHint
	}
	var prevFH *histogram.FloatHistogram
	if f.lastH == nil || !f.currentSeriesRead {
		if f.lastFH == nil || !f.currentSeriesRead {
			// We don't know if there's a counter reset.
			return histogram.UnknownCounterReset
		}
		prevFH = f.lastFH
	} else {
		prevFH = f.lastH.ToFloat(nil)
	}
	fh := h.ToFloat(nil)
	if fh.DetectReset(prevFH) {
		return histogram.CounterReset
	}
	return histogram.NotCounterReset
}
