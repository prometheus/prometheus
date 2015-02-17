// Copyright 2013 The Prometheus Authors
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

package extraction

import (
	"fmt"
	"io"

	dto "github.com/prometheus/client_model/go"

	"github.com/matttproud/golang_protobuf_extensions/ext"

	"github.com/prometheus/client_golang/model"
)

type metricFamilyProcessor struct{}

// MetricFamilyProcessor decodes varint encoded record length-delimited streams
// of io.prometheus.client.MetricFamily.
//
// See http://godoc.org/github.com/matttproud/golang_protobuf_extensions/ext for
// more details.
var MetricFamilyProcessor = &metricFamilyProcessor{}

func (m *metricFamilyProcessor) ProcessSingle(i io.Reader, out Ingester, o *ProcessOptions) error {
	family := &dto.MetricFamily{}

	for {
		family.Reset()

		if _, err := ext.ReadDelimited(i, family); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if err := extractMetricFamily(out, o, family); err != nil {
			return err
		}
	}
}

func extractMetricFamily(out Ingester, o *ProcessOptions, family *dto.MetricFamily) error {
	switch family.GetType() {
	case dto.MetricType_COUNTER:
		if err := extractCounter(out, o, family); err != nil {
			return err
		}
	case dto.MetricType_GAUGE:
		if err := extractGauge(out, o, family); err != nil {
			return err
		}
	case dto.MetricType_SUMMARY:
		if err := extractSummary(out, o, family); err != nil {
			return err
		}
	case dto.MetricType_UNTYPED:
		if err := extractUntyped(out, o, family); err != nil {
			return err
		}
	case dto.MetricType_HISTOGRAM:
		if err := extractHistogram(out, o, family); err != nil {
			return err
		}
	}
	return nil
}

func extractCounter(out Ingester, o *ProcessOptions, f *dto.MetricFamily) error {
	samples := make(model.Samples, 0, len(f.Metric))

	for _, m := range f.Metric {
		if m.Counter == nil {
			continue
		}

		sample := new(model.Sample)
		samples = append(samples, sample)

		if m.TimestampMs != nil {
			sample.Timestamp = model.TimestampFromUnixNano(*m.TimestampMs * 1000000)
		} else {
			sample.Timestamp = o.Timestamp
		}
		sample.Metric = model.Metric{}
		metric := sample.Metric

		for _, p := range m.Label {
			metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
		}

		metric[model.MetricNameLabel] = model.LabelValue(f.GetName())

		sample.Value = model.SampleValue(m.Counter.GetValue())
	}

	return out.Ingest(samples)
}

func extractGauge(out Ingester, o *ProcessOptions, f *dto.MetricFamily) error {
	samples := make(model.Samples, 0, len(f.Metric))

	for _, m := range f.Metric {
		if m.Gauge == nil {
			continue
		}

		sample := new(model.Sample)
		samples = append(samples, sample)

		if m.TimestampMs != nil {
			sample.Timestamp = model.TimestampFromUnixNano(*m.TimestampMs * 1000000)
		} else {
			sample.Timestamp = o.Timestamp
		}
		sample.Metric = model.Metric{}
		metric := sample.Metric

		for _, p := range m.Label {
			metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
		}

		metric[model.MetricNameLabel] = model.LabelValue(f.GetName())

		sample.Value = model.SampleValue(m.Gauge.GetValue())
	}

	return out.Ingest(samples)
}

func extractSummary(out Ingester, o *ProcessOptions, f *dto.MetricFamily) error {
	samples := make(model.Samples, 0, len(f.Metric))

	for _, m := range f.Metric {
		if m.Summary == nil {
			continue
		}

		timestamp := o.Timestamp
		if m.TimestampMs != nil {
			timestamp = model.TimestampFromUnixNano(*m.TimestampMs * 1000000)
		}

		for _, q := range m.Summary.Quantile {
			sample := new(model.Sample)
			samples = append(samples, sample)

			sample.Timestamp = timestamp
			sample.Metric = model.Metric{}
			metric := sample.Metric

			for _, p := range m.Label {
				metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
			}
			// BUG(matt): Update other names to "quantile".
			metric[model.LabelName("quantile")] = model.LabelValue(fmt.Sprint(q.GetQuantile()))

			metric[model.MetricNameLabel] = model.LabelValue(f.GetName())

			sample.Value = model.SampleValue(q.GetValue())
		}

		if m.Summary.SampleSum != nil {
			sum := new(model.Sample)
			sum.Timestamp = timestamp
			metric := model.Metric{}
			for _, p := range m.Label {
				metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
			}
			metric[model.MetricNameLabel] = model.LabelValue(f.GetName() + "_sum")
			sum.Metric = metric
			sum.Value = model.SampleValue(m.Summary.GetSampleSum())
			samples = append(samples, sum)
		}

		if m.Summary.SampleCount != nil {
			count := new(model.Sample)
			count.Timestamp = timestamp
			metric := model.Metric{}
			for _, p := range m.Label {
				metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
			}
			metric[model.MetricNameLabel] = model.LabelValue(f.GetName() + "_count")
			count.Metric = metric
			count.Value = model.SampleValue(m.Summary.GetSampleCount())
			samples = append(samples, count)
		}
	}

	return out.Ingest(samples)
}

func extractUntyped(out Ingester, o *ProcessOptions, f *dto.MetricFamily) error {
	samples := make(model.Samples, 0, len(f.Metric))

	for _, m := range f.Metric {
		if m.Untyped == nil {
			continue
		}

		sample := new(model.Sample)
		samples = append(samples, sample)

		if m.TimestampMs != nil {
			sample.Timestamp = model.TimestampFromUnixNano(*m.TimestampMs * 1000000)
		} else {
			sample.Timestamp = o.Timestamp
		}
		sample.Metric = model.Metric{}
		metric := sample.Metric

		for _, p := range m.Label {
			metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
		}

		metric[model.MetricNameLabel] = model.LabelValue(f.GetName())

		sample.Value = model.SampleValue(m.Untyped.GetValue())
	}

	return out.Ingest(samples)
}

func extractHistogram(out Ingester, o *ProcessOptions, f *dto.MetricFamily) error {
	samples := make(model.Samples, 0, len(f.Metric))

	for _, m := range f.Metric {
		if m.Histogram == nil {
			continue
		}

		timestamp := o.Timestamp
		if m.TimestampMs != nil {
			timestamp = model.TimestampFromUnixNano(*m.TimestampMs * 1000000)
		}

		for _, q := range m.Histogram.Bucket {
			sample := new(model.Sample)
			samples = append(samples, sample)

			sample.Timestamp = timestamp
			sample.Metric = model.Metric{}
			metric := sample.Metric

			for _, p := range m.Label {
				metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
			}
			metric[model.LabelName("le")] = model.LabelValue(fmt.Sprint(q.GetUpperBound()))

			metric[model.MetricNameLabel] = model.LabelValue(f.GetName() + "_bucket")

			sample.Value = model.SampleValue(q.GetCumulativeCount())
		}
		// TODO: If +Inf bucket is missing, add it.

		if m.Histogram.SampleSum != nil {
			sum := new(model.Sample)
			sum.Timestamp = timestamp
			metric := model.Metric{}
			for _, p := range m.Label {
				metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
			}
			metric[model.MetricNameLabel] = model.LabelValue(f.GetName() + "_sum")
			sum.Metric = metric
			sum.Value = model.SampleValue(m.Histogram.GetSampleSum())
			samples = append(samples, sum)
		}

		if m.Histogram.SampleCount != nil {
			count := new(model.Sample)
			count.Timestamp = timestamp
			metric := model.Metric{}
			for _, p := range m.Label {
				metric[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
			}
			metric[model.MetricNameLabel] = model.LabelValue(f.GetName() + "_count")
			count.Metric = metric
			count.Value = model.SampleValue(m.Histogram.GetSampleCount())
			samples = append(samples, count)
		}
	}

	return out.Ingest(samples)
}
