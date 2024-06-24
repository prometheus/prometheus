// Copyright 2020 The Prometheus Authors
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

package writev2

import (
	"math"
	"slices"

	"github.com/prometheus/prometheus/model/histogram"
)

func (m Sample) T() int64   { return m.Timestamp }
func (m Sample) V() float64 { return m.Value }

// IsFloatHistogram returns true if the histogram is float.
func (h Histogram) IsFloatHistogram() bool {
	_, ok := h.GetCount().(*Histogram_CountFloat)
	return ok
}

// ToIntHistogram returns Integer Prometheus histogram from remote implementation
// of integer. It's a caller responsibility to check if it's not Float histogram.
func (h Histogram) ToIntHistogram() *histogram.Histogram {
	return &histogram.Histogram{
		CounterResetHint: histogram.CounterResetHint(h.ResetHint),
		Schema:           h.Schema,
		ZeroThreshold:    h.ZeroThreshold,
		ZeroCount:        h.GetZeroCountInt(),
		Count:            h.GetCountInt(),
		Sum:              h.Sum,
		PositiveSpans:    spansProtoToSpans(h.GetPositiveSpans()),
		PositiveBuckets:  h.GetPositiveDeltas(),
		NegativeSpans:    spansProtoToSpans(h.GetNegativeSpans()),
		NegativeBuckets:  h.GetNegativeDeltas(),
	}
}

// ToFloatHistogram returns Float Prometheus histogram from remote implementation
// of float (or integer).
func (h Histogram) ToFloatHistogram() *histogram.FloatHistogram {
	if h.IsFloatHistogram() {
		return &histogram.FloatHistogram{
			CounterResetHint: histogram.CounterResetHint(h.ResetHint),
			Schema:           h.Schema,
			ZeroThreshold:    h.ZeroThreshold,
			ZeroCount:        h.GetZeroCountFloat(),
			Count:            h.GetCountFloat(),
			Sum:              h.Sum,
			PositiveSpans:    spansProtoToSpans(h.GetPositiveSpans()),
			PositiveBuckets:  h.GetPositiveCounts(),
			NegativeSpans:    spansProtoToSpans(h.GetNegativeSpans()),
			NegativeBuckets:  h.GetNegativeCounts(),
		}
	}
	// Conversion.
	return &histogram.FloatHistogram{
		CounterResetHint: histogram.CounterResetHint(h.ResetHint),
		Schema:           h.Schema,
		ZeroThreshold:    h.ZeroThreshold,
		ZeroCount:        float64(h.GetZeroCountInt()),
		Count:            float64(h.GetCountInt()),
		Sum:              h.Sum,
		PositiveSpans:    spansProtoToSpans(h.GetPositiveSpans()),
		PositiveBuckets:  deltasToCounts(h.GetPositiveDeltas()),
		NegativeSpans:    spansProtoToSpans(h.GetNegativeSpans()),
		NegativeBuckets:  deltasToCounts(h.GetNegativeDeltas()),
	}
}

func spansProtoToSpans(s []BucketSpan) []histogram.Span {
	spans := make([]histogram.Span, len(s))
	for i := 0; i < len(s); i++ {
		spans[i] = histogram.Span{Offset: s[i].Offset, Length: s[i].Length}
	}

	return spans
}

func deltasToCounts(deltas []int64) []float64 {
	counts := make([]float64, len(deltas))
	var cur float64
	for i, d := range deltas {
		cur += float64(d)
		counts[i] = cur
	}
	return counts
}

// FromIntHistogram returns remote Histogram from the Integer Histogram.
func FromIntHistogram(timestamp int64, h *histogram.Histogram) Histogram {
	return Histogram{
		Count:          &Histogram_CountInt{CountInt: h.Count},
		Sum:            h.Sum,
		Schema:         h.Schema,
		ZeroThreshold:  h.ZeroThreshold,
		ZeroCount:      &Histogram_ZeroCountInt{ZeroCountInt: h.ZeroCount},
		NegativeSpans:  spansToSpansProto(h.NegativeSpans),
		NegativeDeltas: h.NegativeBuckets,
		PositiveSpans:  spansToSpansProto(h.PositiveSpans),
		PositiveDeltas: h.PositiveBuckets,
		ResetHint:      Histogram_ResetHint(h.CounterResetHint),
		Timestamp:      timestamp,
	}
}

// FromFloatHistogram returns remote Histogram from the Float Histogram.
func FromFloatHistogram(timestamp int64, fh *histogram.FloatHistogram) Histogram {
	return Histogram{
		Count:          &Histogram_CountFloat{CountFloat: fh.Count},
		Sum:            fh.Sum,
		Schema:         fh.Schema,
		ZeroThreshold:  fh.ZeroThreshold,
		ZeroCount:      &Histogram_ZeroCountFloat{ZeroCountFloat: fh.ZeroCount},
		NegativeSpans:  spansToSpansProto(fh.NegativeSpans),
		NegativeCounts: fh.NegativeBuckets,
		PositiveSpans:  spansToSpansProto(fh.PositiveSpans),
		PositiveCounts: fh.PositiveBuckets,
		ResetHint:      Histogram_ResetHint(fh.CounterResetHint),
		Timestamp:      timestamp,
	}
}

func spansToSpansProto(s []histogram.Span) []BucketSpan {
	spans := make([]BucketSpan, len(s))
	for i := 0; i < len(s); i++ {
		spans[i] = BucketSpan{Offset: s[i].Offset, Length: s[i].Length}
	}

	return spans
}

func (m *Request) OptimizedMarshal(dst []byte) ([]byte, error) {
	siz := m.Size()
	if cap(dst) < siz {
		dst = make([]byte, siz)
	}
	dst = dst[:siz]

	n, err := m.OptimizedMarshalToSizedBuffer(dst)
	if err != nil {
		return nil, err
	}
	return (dst)[:n], nil
}

// OptimizedMarshalToSizedBuffer is mostly a copy of the generated MarshalToSizedBuffer,
// but calls OptimizedMarshalToSizedBuffer on the timeseries.
func (m *Request) OptimizedMarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	if len(m.Timeseries) > 0 {
		for iNdEx := len(m.Timeseries) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Timeseries[iNdEx].OptimizedMarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x2a
		}
	}
	if len(m.Symbols) > 0 {
		for iNdEx := len(m.Symbols) - 1; iNdEx >= 0; iNdEx-- {
			i -= len(m.Symbols[iNdEx])
			copy(dAtA[i:], m.Symbols[iNdEx])
			i = encodeVarintTypes(dAtA, i, uint64(len(m.Symbols[iNdEx])))
			i--
			dAtA[i] = 0x22
		}
	}
	return len(dAtA) - i, nil
}

// OptimizedMarshalToSizedBuffer is mostly a copy of the generated MarshalToSizedBuffer,
// but marshals m.LabelsRefs in place without extra allocations.
func (m *TimeSeries) OptimizedMarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	if m.CreatedTimestamp != 0 {
		i = encodeVarintTypes(dAtA, i, uint64(m.CreatedTimestamp))
		i--
		dAtA[i] = 0x30
	}
	{
		size, err := m.Metadata.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintTypes(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x2a
	if len(m.Histograms) > 0 {
		for iNdEx := len(m.Histograms) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Histograms[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x1a
		}
	}
	if len(m.Exemplars) > 0 {
		for iNdEx := len(m.Exemplars) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Exemplars[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x22
		}
	}
	if len(m.Samples) > 0 {
		for iNdEx := len(m.Samples) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Samples[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x12
		}
	}

	if len(m.LabelsRefs) > 0 {
		// This is the trick: encode the varints in reverse order to make it easier
		// to do it in place. Then reverse the whole thing.
		var j10 int
		start := i
		for _, num := range m.LabelsRefs {
			for num >= 1<<7 {
				dAtA[i-1] = uint8(uint64(num)&0x7f | 0x80)
				num >>= 7
				i--
				j10++
			}
			dAtA[i-1] = uint8(num)
			i--
			j10++
		}
		slices.Reverse(dAtA[i:start])
		// --- end of trick

		i = encodeVarintTypes(dAtA, i, uint64(j10))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

// FilterTimeSeries returns filtered times series with filtering and timestamp statistics.
func FilterTimeSeries(timeSeries []TimeSeries, filter func(TimeSeries) bool) (highest, lowest int64, filtered []TimeSeries, droppedSeries, droppedSamples, droppedExemplars, droppedHistograms int) {
	keepIdx := 0
	lowest = math.MaxInt64
	for i, ts := range timeSeries {
		if filter != nil && filter(ts) {
			droppedSeries++
			if len(ts.Samples) > 0 {
				droppedSamples = +len(ts.Samples)
			}
			if len(ts.Histograms) > 0 {
				droppedHistograms = +len(ts.Histograms)
			}
			if len(ts.Exemplars) > 0 {
				droppedExemplars = +len(ts.Exemplars)
			}
			continue
		}

		// At the moment we only ever append a TimeSeries with a single sample or exemplar in it.
		// TODO(bwplotka): Still true?
		if len(ts.Samples) > 0 && ts.Samples[0].Timestamp > highest {
			highest = ts.Samples[0].Timestamp
		}
		if len(ts.Exemplars) > 0 && ts.Exemplars[0].Timestamp > highest {
			highest = ts.Exemplars[0].Timestamp
		}
		if len(ts.Histograms) > 0 && ts.Histograms[0].Timestamp > highest {
			highest = ts.Histograms[0].Timestamp
		}

		// Get the lowest timestamp.
		if len(ts.Samples) > 0 && ts.Samples[0].Timestamp < lowest {
			lowest = ts.Samples[0].Timestamp
		}
		if len(ts.Exemplars) > 0 && ts.Exemplars[0].Timestamp < lowest {
			lowest = ts.Exemplars[0].Timestamp
		}
		if len(ts.Histograms) > 0 && ts.Histograms[0].Timestamp < lowest {
			lowest = ts.Histograms[0].Timestamp
		}

		// Move the current element to the write position and increment the write pointer
		timeSeries[keepIdx] = timeSeries[i]
		keepIdx++
	}

	timeSeries = timeSeries[:keepIdx]
	return highest, lowest, timeSeries, droppedSeries, droppedSamples, droppedHistograms, droppedExemplars
}
