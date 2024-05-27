package promql

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

type histogramStatsIterator struct {
	chunkenc.Iterator

	hReader *histogram.Histogram
	lastH   *histogram.Histogram

	fhReader *histogram.FloatHistogram
	lastFH   *histogram.FloatHistogram
}

func NewHistogramStatsIterator(it chunkenc.Iterator) chunkenc.Iterator {
	return &histogramStatsIterator{
		Iterator: it,
		hReader:  &histogram.Histogram{},
		fhReader: &histogram.FloatHistogram{},
	}
}

func (f *histogramStatsIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	var t int64
	t, f.hReader = f.Iterator.AtHistogram(f.hReader)
	if value.IsStaleNaN(f.hReader.Sum) {
		f.setLastH(f.hReader)
		h = &histogram.Histogram{Sum: f.hReader.Sum}
		return t, h
	}

	if h == nil {
		h = &histogram.Histogram{
			CounterResetHint: f.getResetHint(f.hReader),
			Count:            f.hReader.Count,
			Sum:              f.hReader.Sum,
		}
		f.setLastH(f.hReader)
		return t, h
	}

	h.CounterResetHint = f.getResetHint(f.hReader)
	h.Count = f.hReader.Count
	h.Sum = f.hReader.Sum
	f.setLastH(f.hReader)
	return t, h
}

func (f *histogramStatsIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	var t int64
	t, f.fhReader = f.Iterator.AtFloatHistogram(f.fhReader)
	if value.IsStaleNaN(f.fhReader.Sum) {
		f.setLastFH(f.fhReader)
		return t, &histogram.FloatHistogram{Sum: f.fhReader.Sum}
	}

	if fh == nil {
		fh = &histogram.FloatHistogram{
			CounterResetHint: f.getFloatResetHint(f.fhReader.CounterResetHint),
			Count:            f.fhReader.Count,
			Sum:              f.fhReader.Sum,
		}
		f.setLastFH(f.fhReader)
		return t, fh
	}

	fh.CounterResetHint = f.getFloatResetHint(f.fhReader.CounterResetHint)
	fh.Count = f.fhReader.Count
	fh.Sum = f.fhReader.Sum
	f.setLastFH(f.fhReader)
	return t, fh
}

func (f *histogramStatsIterator) setLastH(h *histogram.Histogram) {
	if f.lastH == nil {
		f.lastH = h.Copy()
	} else {
		h.CopyTo(f.lastH)
	}
}

func (f *histogramStatsIterator) setLastFH(fh *histogram.FloatHistogram) {
	if f.lastFH == nil {
		f.lastFH = fh.Copy()
	} else {
		fh.CopyTo(f.lastFH)
	}
}

func (f *histogramStatsIterator) getFloatResetHint(hint histogram.CounterResetHint) histogram.CounterResetHint {
	if hint != histogram.UnknownCounterReset {
		return hint
	}
	if f.lastFH == nil {
		return histogram.NotCounterReset
	}

	if f.fhReader.DetectReset(f.lastFH) {
		return histogram.CounterReset
	} else {
		return histogram.NotCounterReset
	}
}

func (f *histogramStatsIterator) getResetHint(h *histogram.Histogram) histogram.CounterResetHint {
	if h.CounterResetHint != histogram.UnknownCounterReset {
		return h.CounterResetHint
	}
	if f.lastH == nil {
		return histogram.NotCounterReset
	}

	fh, prevFH := h.ToFloat(nil), f.lastH.ToFloat(nil)
	if fh.DetectReset(prevFH) {
		return histogram.CounterReset
	} else {
		return histogram.NotCounterReset
	}
}
