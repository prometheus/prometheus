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

package prompb

import (
	"fmt"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

// NOTE(bwplotka): This file's code is tested in /prompb/rwcommon.

// ToLabels return model labels.Labels from timeseries' remote labels.
func (m TimeSeries) ToLabels(b *labels.ScratchBuilder, _ []string) labels.Labels {
	return labelProtosToLabels(b, m.GetLabels())
}

// ToLabels return model labels.Labels from timeseries' remote labels.
func (m ChunkedSeries) ToLabels(b *labels.ScratchBuilder, _ []string) labels.Labels {
	return labelProtosToLabels(b, m.GetLabels())
}

// ToLabelsWithLimits return model labels.Labels from timeseries' remote labels,
// rejecting the series when a label name length exceeds nameLimit or a label
// value length exceeds valueLimit. A limit of 0 means no limit. Checking before
// the labels are materialized keeps oversized values away from label storage
// allocations.
func (m TimeSeries) ToLabelsWithLimits(b *labels.ScratchBuilder, nameLimit, valueLimit int) (labels.Labels, error) {
	for _, l := range m.GetLabels() {
		if err := CheckLabelLength(l.Name, l.Value, nameLimit, valueLimit); err != nil {
			return labels.EmptyLabels(), err
		}
	}
	return labelProtosToLabels(b, m.GetLabels()), nil
}

// CheckLabelLengthError is returned when a label name or value exceeds the
// configured length limits.
type CheckLabelLengthError struct {
	Name, Value           string
	NameLen, ValueLen     int
	NameLimit, ValueLimit int
}

func (e CheckLabelLengthError) Error() string {
	if e.NameLen > e.NameLimit && e.NameLimit > 0 {
		return fmt.Sprintf("label name length %d exceeds limit %d (label name: %.50s)", e.NameLen, e.NameLimit, e.Name)
	}
	return fmt.Sprintf("label value length %d exceeds limit %d for label %q (value: %.50q)", e.ValueLen, e.ValueLimit, e.Name, e.Value)
}

// CheckLabelLength validates label name and value lengths against the given
// limits. A limit of 0 disables the check for that dimension.
func CheckLabelLength(name, value string, nameLimit, valueLimit int) error {
	if nameLimit > 0 && len(name) > nameLimit {
		return CheckLabelLengthError{Name: name, Value: value, NameLen: len(name), ValueLen: len(value), NameLimit: nameLimit, ValueLimit: valueLimit}
	}
	if valueLimit > 0 && len(value) > valueLimit {
		return CheckLabelLengthError{Name: name, Value: value, NameLen: len(name), ValueLen: len(value), NameLimit: nameLimit, ValueLimit: valueLimit}
	}
	return nil
}

func labelProtosToLabels(b *labels.ScratchBuilder, labelPairs []Label) labels.Labels {
	b.Reset()
	for _, l := range labelPairs {
		b.Add(l.Name, l.Value)
	}
	b.Sort()
	return b.Labels()
}

// FromLabels transforms labels into prompb labels. The buffer slice
// will be used to avoid allocations if it is big enough to store the labels.
func FromLabels(lbls labels.Labels, buf []Label) []Label {
	result := buf[:0]
	lbls.Range(func(l labels.Label) {
		result = append(result, Label{
			Name:  l.Name,
			Value: l.Value,
		})
	})
	return result
}

// FromMetadataType transforms a Prometheus metricType into prompb metricType. Since the former is a string we need to transform it to an enum.
func FromMetadataType(t model.MetricType) MetricMetadata_MetricType {
	mt := strings.ToUpper(string(t))
	v, ok := MetricMetadata_MetricType_value[mt]
	if !ok {
		return MetricMetadata_UNKNOWN
	}
	return MetricMetadata_MetricType(v)
}

// IsFloatHistogram returns true if the histogram is float.
func (h Histogram) IsFloatHistogram() bool {
	_, ok := h.GetCount().(*Histogram_CountFloat)
	return ok
}

// ToIntHistogram returns integer Prometheus histogram from the remote implementation
// of integer histogram. If it's a float histogram, the method returns nil.
func (h Histogram) ToIntHistogram() *histogram.Histogram {
	if h.IsFloatHistogram() {
		return nil
	}
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
		CustomValues:     h.CustomValues,
	}
}

// ToFloatHistogram returns float Prometheus histogram from the remote implementation
// of float histogram. If the underlying implementation is an integer histogram, a
// conversion is performed.
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
			CustomValues:     h.CustomValues, // CustomValues are immutable.
		}
	}
	// Conversion from integer histogram.
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
		CustomValues:     h.CustomValues, // CustomValues are immutable.
	}
}

func spansProtoToSpans(s []BucketSpan) []histogram.Span {
	spans := make([]histogram.Span, len(s))
	for i := range s {
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

// FromIntHistogram returns remote Histogram from the integer Histogram.
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
		CustomValues:   h.CustomValues, // CustomValues are immutable.
	}
}

// FromFloatHistogram returns remote Histogram from the float Histogram.
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
		CustomValues:   fh.CustomValues, // CustomValues are immutable.
	}
}

func spansToSpansProto(s []histogram.Span) []BucketSpan {
	spans := make([]BucketSpan, len(s))
	for i := range s {
		spans[i] = BucketSpan{Offset: s[i].Offset, Length: s[i].Length}
	}

	return spans
}

// ToExemplar converts remote exemplar to model exemplar.
func (m Exemplar) ToExemplar(b *labels.ScratchBuilder, _ []string) exemplar.Exemplar {
	timestamp := m.Timestamp

	return exemplar.Exemplar{
		Labels: labelProtosToLabels(b, m.GetLabels()),
		Value:  m.Value,
		Ts:     timestamp,
		HasTs:  timestamp != 0,
	}
}
