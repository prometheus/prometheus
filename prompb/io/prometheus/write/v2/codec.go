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

package writev2

import (
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
)

// NOTE(bwplotka): This file's code is tested in /prompb/rwcommon.

// ToLabels return model labels.Labels from timeseries' remote labels.
func (m TimeSeries) ToLabels(b *labels.ScratchBuilder, symbols []string) (labels.Labels, error) {
	return desymbolizeLabels(b, m.GetLabelsRefs(), symbols)
}

// ToMetadata return model metadata from timeseries' remote metadata.
func (m TimeSeries) ToMetadata(symbols []string) metadata.Metadata {
	typ := model.MetricTypeUnknown
	switch m.Metadata.Type {
	case Metadata_METRIC_TYPE_COUNTER:
		typ = model.MetricTypeCounter
	case Metadata_METRIC_TYPE_GAUGE:
		typ = model.MetricTypeGauge
	case Metadata_METRIC_TYPE_HISTOGRAM:
		typ = model.MetricTypeHistogram
	case Metadata_METRIC_TYPE_GAUGEHISTOGRAM:
		typ = model.MetricTypeGaugeHistogram
	case Metadata_METRIC_TYPE_SUMMARY:
		typ = model.MetricTypeSummary
	case Metadata_METRIC_TYPE_INFO:
		typ = model.MetricTypeInfo
	case Metadata_METRIC_TYPE_STATESET:
		typ = model.MetricTypeStateset
	}
	return metadata.Metadata{
		Type: typ,
		Unit: symbols[m.Metadata.UnitRef],
		Help: symbols[m.Metadata.HelpRef],
	}
}

// FromMetadataType transforms a Prometheus metricType into writev2 metricType.
// Since the former is a string we need to transform it to an enum.
func FromMetadataType(t model.MetricType) Metadata_MetricType {
	switch t {
	case model.MetricTypeCounter:
		return Metadata_METRIC_TYPE_COUNTER
	case model.MetricTypeGauge:
		return Metadata_METRIC_TYPE_GAUGE
	case model.MetricTypeHistogram:
		return Metadata_METRIC_TYPE_HISTOGRAM
	case model.MetricTypeGaugeHistogram:
		return Metadata_METRIC_TYPE_GAUGEHISTOGRAM
	case model.MetricTypeSummary:
		return Metadata_METRIC_TYPE_SUMMARY
	case model.MetricTypeInfo:
		return Metadata_METRIC_TYPE_INFO
	case model.MetricTypeStateset:
		return Metadata_METRIC_TYPE_STATESET
	default:
		return Metadata_METRIC_TYPE_UNSPECIFIED
	}
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
		CustomValues:     h.GetCustomValues(),
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
			CustomValues:     h.GetCustomValues(),
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
		CustomValues:     h.GetCustomValues(),
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
		CustomValues:   h.CustomValues,
		Timestamp:      timestamp,
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
		CustomValues:   fh.CustomValues,
		Timestamp:      timestamp,
	}
}

func spansToSpansProto(s []histogram.Span) []BucketSpan {
	if len(s) == 0 {
		return nil
	}
	spans := make([]BucketSpan, len(s))
	for i := range s {
		spans[i] = BucketSpan{Offset: s[i].Offset, Length: s[i].Length}
	}

	return spans
}

func (m Exemplar) ToExemplar(b *labels.ScratchBuilder, symbols []string) (exemplar.Exemplar, error) {
	timestamp := m.Timestamp

	lbls, err := desymbolizeLabels(b, m.LabelsRefs, symbols)
	if err != nil {
		return exemplar.Exemplar{}, err
	}

	return exemplar.Exemplar{
		Labels: lbls,
		Value:  m.Value,
		Ts:     timestamp,
		HasTs:  timestamp != 0,
	}, nil
}
