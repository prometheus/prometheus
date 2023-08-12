// Copyright 2023 The Prometheus Authors
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

package fmtutil

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/prompb"
)

const (
	sumStr    = "_sum"
	countStr  = "_count"
	bucketStr = "_bucket"
)

var MetricMetadataTypeValue = map[string]int32{
	"UNKNOWN":        0,
	"COUNTER":        1,
	"GAUGE":          2,
	"HISTOGRAM":      3,
	"GAUGEHISTOGRAM": 4,
	"SUMMARY":        5,
	"INFO":           6,
	"STATESET":       7,
}

// MetricTextToWriteRequest consumes an io.Reader and return the data in write request format.
func MetricTextToWriteRequest(input io.Reader, labels map[string]string) (*prompb.WriteRequest, error) {
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(input)
	if err != nil {
		return nil, err
	}
	return MetricFamiliesToWriteRequest(mf, labels)
}

// MetricFamiliesToWriteRequest convert metric family to a writerequest.
func MetricFamiliesToWriteRequest(mf map[string]*dto.MetricFamily, extraLabels map[string]string) (*prompb.WriteRequest, error) {
	wr := &prompb.WriteRequest{}

	// build metric list
	sortedMetricNames := make([]string, 0, len(mf))
	for metric := range mf {
		sortedMetricNames = append(sortedMetricNames, metric)
	}
	// sort metrics name in lexicographical order
	sort.Strings(sortedMetricNames)

	for _, metricName := range sortedMetricNames {
		// Set metadata writerequest
		mtype := MetricMetadataTypeValue[mf[metricName].Type.String()]
		metadata := prompb.MetricMetadata{
			MetricFamilyName: mf[metricName].GetName(),
			Type:             prompb.MetricMetadata_MetricType(mtype),
			Help:             mf[metricName].GetHelp(),
		}
		wr.Metadata = append(wr.Metadata, metadata)

		for _, metric := range mf[metricName].Metric {
			labels := makeLabelsMap(metric, metricName, extraLabels)
			if err := makeTimeseries(wr, labels, metric); err != nil {
				return wr, err
			}
		}
	}
	return wr, nil
}

func toTimeseries(wr *prompb.WriteRequest, labels map[string]string, timestamp int64, value float64) {
	var ts prompb.TimeSeries
	ts.Labels = makeLabels(labels)
	ts.Samples = []prompb.Sample{
		{
			Timestamp: timestamp,
			Value:     value,
		},
	}
	wr.Timeseries = append(wr.Timeseries, ts)
}

func makeTimeseries(wr *prompb.WriteRequest, labels map[string]string, m *dto.Metric) error {
	var err error

	timestamp := m.GetTimestampMs()
	if timestamp == 0 {
		timestamp = time.Now().UnixNano() / int64(time.Millisecond)
	}

	switch {
	case m.Gauge != nil:
		toTimeseries(wr, labels, timestamp, m.GetGauge().GetValue())
	case m.Counter != nil:
		toTimeseries(wr, labels, timestamp, m.GetCounter().GetValue())
	case m.Summary != nil:
		metricName := labels[model.MetricNameLabel]
		// Preserve metric name order with first quantile labels timeseries then sum suffix timeserie and finally count suffix timeserie
		// Add Summary quantile timeseries
		quantileLabels := make(map[string]string, len(labels)+1)
		for key, value := range labels {
			quantileLabels[key] = value
		}

		for _, q := range m.GetSummary().Quantile {
			quantileLabels[model.QuantileLabel] = fmt.Sprint(q.GetQuantile())
			toTimeseries(wr, quantileLabels, timestamp, q.GetValue())
		}
		// Overwrite label model.MetricNameLabel for count and sum metrics
		// Add Summary sum timeserie
		labels[model.MetricNameLabel] = metricName + sumStr
		toTimeseries(wr, labels, timestamp, m.GetSummary().GetSampleSum())
		// Add Summary count timeserie
		labels[model.MetricNameLabel] = metricName + countStr
		toTimeseries(wr, labels, timestamp, float64(m.GetSummary().GetSampleCount()))

	case m.Histogram != nil:
		h := m.GetHistogram()

		if isNativeHistogram(h) {
			addNativeHistogramToWriteRequest(wr, h, labels, timestamp)
		}

		// While a quantile-less summary is preferred over a bucket-less histogram, the
		// latter could happen in principle. The other two properties of a classic histogram
		// (sum and count) are also set for native histograms, so the only signal we have to
		// decide whether the classic histogram should be included in the request is having
		// a list of buckets.
		//
		// Note that this comes _after_ native histogams because calling
		// addClassicHistogramToWriteRequest modifies the labels map.
		if len(h.Bucket) > 0 {
			addClassicHistogramToWriteRequest(wr, h, labels, timestamp)
		}

	case m.Untyped != nil:
		toTimeseries(wr, labels, timestamp, m.GetUntyped().GetValue())
	default:
		err = errors.New("unsupported metric type")
	}
	return err
}

func makeLabels(labelsMap map[string]string) []prompb.Label {
	// build labels name list
	sortedLabelNames := make([]string, 0, len(labelsMap))
	for label := range labelsMap {
		sortedLabelNames = append(sortedLabelNames, label)
	}
	// sort labels name in lexicographical order
	sort.Strings(sortedLabelNames)

	var labels []prompb.Label
	for _, label := range sortedLabelNames {
		labels = append(labels, prompb.Label{
			Name:  label,
			Value: labelsMap[label],
		})
	}
	return labels
}

func makeLabelsMap(m *dto.Metric, metricName string, extraLabels map[string]string) map[string]string {
	// build labels map
	labels := make(map[string]string, len(m.Label)+len(extraLabels))
	labels[model.MetricNameLabel] = metricName

	// add extra labels
	for key, value := range extraLabels {
		labels[key] = value
	}

	// add metric labels
	for _, label := range m.Label {
		labelname := label.GetName()
		if labelname == model.JobLabel {
			labelname = fmt.Sprintf("%s%s", model.ExportedLabelPrefix, labelname)
		}
		labels[labelname] = label.GetValue()
	}

	return labels
}

func makeBucketSpans(spans []*dto.BucketSpan) []prompb.BucketSpan {
	out := make([]prompb.BucketSpan, 0, len(spans))
	for _, span := range spans {
		out = append(out, prompb.BucketSpan{Offset: span.GetOffset(), Length: span.GetLength()})
	}
	return out
}

func addClassicHistogramToWriteRequest(wr *prompb.WriteRequest, h *dto.Histogram, labels map[string]string, timestamp int64) {
	metricName := labels[model.MetricNameLabel]

	// Preserve metric name order with first bucket suffix timeseries then
	// count suffix timeseries and finally sum suffix timeserie

	// Add histogram bucket timeseries.
	bucketLabels := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		bucketLabels[key] = value
	}
	for _, b := range h.Bucket {
		bucketLabels[model.MetricNameLabel] = metricName + bucketStr
		bucketLabels[model.BucketLabel] = fmt.Sprint(b.GetUpperBound())
		toTimeseries(wr, bucketLabels, timestamp, float64(b.GetCumulativeCount()))
	}

	// Overwrite label model.MetricNameLabel for count and sum metrics.

	// Add histogram count timeseries.
	labels[model.MetricNameLabel] = metricName + countStr
	toTimeseries(wr, labels, timestamp, float64(h.GetSampleCount()))

	// Add histogram sum timeseries.
	labels[model.MetricNameLabel] = metricName + sumStr
	toTimeseries(wr, labels, timestamp, h.GetSampleSum())
}

func addNativeHistogramToWriteRequest(wr *prompb.WriteRequest, h *dto.Histogram, labels map[string]string, timestamp int64) {
	// Note that for float histograms:
	// - Count should use CountFloat
	// - NegativeCounts should bet used instead of NegativeDeltas
	// - PositiveCounts should bet used instead of PositiveDeltas
	//
	// For float histograms:
	// - Count should use CountInt
	// - NegativeDeltas should bet used instead of NegativeCounts
	// - PositiveDeltas should bet used instead of PositiveCounts
	//
	// *Counts and *Deltas are fine because the functions return nil if
	// these are not set.
	//
	// The necessary interfaces for Count and ZeroCount are impossible to
	// implement because they require a private method, this means it's not
	// possible to create makeCount function that returns an instance of
	// that interface.

	hist := prompb.Histogram{
		Sum:            h.GetSampleSum(),
		Schema:         h.GetSchema(),
		ZeroThreshold:  h.GetZeroThreshold(),
		NegativeSpans:  makeBucketSpans(h.GetNegativeSpan()),
		NegativeDeltas: h.GetNegativeDelta(),
		NegativeCounts: h.GetNegativeCount(),
		PositiveSpans:  makeBucketSpans(h.GetPositiveSpan()),
		PositiveDeltas: h.GetPositiveDelta(),
		PositiveCounts: h.GetPositiveCount(),
		Timestamp:      timestamp,
		// ResetHint: what is this?
	}

	if h.SampleCountFloat != nil {
		// Assume float histogram
		hist.Count = &prompb.Histogram_CountFloat{CountFloat: h.GetSampleCountFloat()}
		hist.ZeroCount = &prompb.Histogram_ZeroCountFloat{ZeroCountFloat: h.GetZeroCountFloat()}
	} else {
		hist.Count = &prompb.Histogram_CountInt{CountInt: h.GetSampleCount()}
		hist.ZeroCount = &prompb.Histogram_ZeroCountInt{ZeroCountInt: h.GetZeroCount()}
	}

	wr.Timeseries = append(wr.Timeseries, prompb.TimeSeries{
		Labels:     makeLabels(labels),
		Histograms: []prompb.Histogram{hist},
	})
}

// isNativeHistogram returns false iff the provided histograms has no spans at
// all (neither positive nor negative) and a zero threshold of 0 and a zero
// count of 0. In principle, this could still be meant to be a native histogram
// with a zero threshold of 0 and no observations yet. In that case,
// instrumentation libraries should add a "no-op" span (e.g. length zero, offset
// zero) to signal that the histogram is meant to be parsed as a native
// histogram. Failing to do so will cause Prometheus to parse it as a classic
// histogram as long as no observations have happened.
func isNativeHistogram(h *dto.Histogram) bool {
	return len(h.GetPositiveSpan()) > 0 ||
		len(h.GetNegativeSpan()) > 0 ||
		h.GetZeroThreshold() > 0 ||
		h.GetZeroCount() > 0
}
