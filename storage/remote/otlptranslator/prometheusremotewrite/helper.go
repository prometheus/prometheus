// DO NOT EDIT. COPIED AS-IS. SEE ../README.md

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewrite // import "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"

import (
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"

	prometheustranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheus"
)

const (
	sumStr        = "_sum"
	countStr      = "_count"
	bucketStr     = "_bucket"
	leStr         = "le"
	quantileStr   = "quantile"
	pInfStr       = "+Inf"
	createdSuffix = "_created"
	// maxExemplarRunes is the maximum number of UTF-8 exemplar characters
	// according to the prometheus specification
	// https://github.com/OpenObservability/OpenMetrics/blob/main/specification/OpenMetrics.md#exemplars
	maxExemplarRunes = 128
	// Trace and Span id keys are defined as part of the spec:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification%2Fmetrics%2Fdatamodel.md#exemplars-2
	traceIDKey       = "trace_id"
	spanIDKey        = "span_id"
	infoType         = "info"
	targetMetricName = "target_info"
)

type bucketBoundsData struct {
	sig   string
	bound float64
}

// byBucketBoundsData enables the usage of sort.Sort() with a slice of bucket bounds
type byBucketBoundsData []bucketBoundsData

func (m byBucketBoundsData) Len() int           { return len(m) }
func (m byBucketBoundsData) Less(i, j int) bool { return m[i].bound < m[j].bound }
func (m byBucketBoundsData) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }

// ByLabelName enables the usage of sort.Sort() with a slice of labels
type ByLabelName []prompb.Label

func (a ByLabelName) Len() int           { return len(a) }
func (a ByLabelName) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a ByLabelName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// addSample finds a TimeSeries in tsMap that corresponds to the label set labels, and add sample to the TimeSeries; it
// creates a new TimeSeries in the map if not found and returns the time series signature.
// tsMap will be unmodified if either labels or sample is nil, but can still be modified if the exemplar is nil.
func addSample(tsMap map[string]*prompb.TimeSeries, sample *prompb.Sample, labels []prompb.Label,
	datatype string) string {
	if sample == nil || labels == nil || tsMap == nil {
		// This shouldn't happen
		return ""
	}

	sig := timeSeriesSignature(datatype, labels)
	ts := tsMap[sig]
	if ts != nil {
		ts.Samples = append(ts.Samples, *sample)
	} else {
		newTs := &prompb.TimeSeries{
			Labels:  labels,
			Samples: []prompb.Sample{*sample},
		}
		tsMap[sig] = newTs
	}

	return sig
}

// addExemplars finds a bucket bound that corresponds to the exemplars value and add the exemplar to the specific sig;
// we only add exemplars if samples are presents
// tsMap is unmodified if either of its parameters is nil and samples are nil.
func addExemplars(tsMap map[string]*prompb.TimeSeries, exemplars []prompb.Exemplar, bucketBoundsData []bucketBoundsData) {
	if len(tsMap) == 0 || len(bucketBoundsData) == 0 || len(exemplars) == 0 {
		return
	}

	sort.Sort(byBucketBoundsData(bucketBoundsData))

	for _, exemplar := range exemplars {
		addExemplar(tsMap, bucketBoundsData, exemplar)
	}
}

func addExemplar(tsMap map[string]*prompb.TimeSeries, bucketBounds []bucketBoundsData, exemplar prompb.Exemplar) {
	for _, bucketBound := range bucketBounds {
		sig := bucketBound.sig
		bound := bucketBound.bound

		ts := tsMap[sig]
		if ts != nil && len(ts.Samples) > 0 && exemplar.Value <= bound {
			ts.Exemplars = append(ts.Exemplars, exemplar)
			return
		}
	}
}

// timeSeries return a string signature in the form of:
//
//	TYPE-label1-value1- ...  -labelN-valueN
//
// the label slice should not contain duplicate label names; this method sorts the slice by label name before creating
// the signature.
func timeSeriesSignature(datatype string, labels []prompb.Label) string {
	length := len(datatype)

	for _, lb := range labels {
		length += 2 + len(lb.GetName()) + len(lb.GetValue())
	}

	b := strings.Builder{}
	b.Grow(length)
	b.WriteString(datatype)

	sort.Sort(ByLabelName(labels))

	for _, lb := range labels {
		b.WriteString("-")
		b.WriteString(lb.GetName())
		b.WriteString("-")
		b.WriteString(lb.GetValue())
	}

	return b.String()
}

// createAttributes creates a slice of Prometheus Labels with OTLP attributes and pairs of string values.
// Unpaired string values are ignored. String pairs overwrite OTLP labels if collisions happen, and overwrites are
// logged. Resulting label names are sanitized.
func createAttributes(resource pcommon.Resource, attributes pcommon.Map, externalLabels map[string]string, extras ...string) []prompb.Label {
	serviceName, haveServiceName := resource.Attributes().Get(conventions.AttributeServiceName)
	instance, haveInstanceID := resource.Attributes().Get(conventions.AttributeServiceInstanceID)

	// Calculate the maximum possible number of labels we could return so we can preallocate l
	maxLabelCount := attributes.Len() + len(externalLabels) + len(extras)/2

	if haveServiceName {
		maxLabelCount++
	}

	if haveInstanceID {
		maxLabelCount++
	}

	// map ensures no duplicate label name
	l := make(map[string]string, maxLabelCount)

	// Ensure attributes are sorted by key for consistent merging of keys which
	// collide when sanitized.
	labels := make([]prompb.Label, 0, attributes.Len())
	attributes.Range(func(key string, value pcommon.Value) bool {
		labels = append(labels, prompb.Label{Name: key, Value: value.AsString()})
		return true
	})
	sort.Stable(ByLabelName(labels))

	for _, label := range labels {
		var finalKey = prometheustranslator.NormalizeLabel(label.Name)
		if existingValue, alreadyExists := l[finalKey]; alreadyExists {
			l[finalKey] = existingValue + ";" + label.Value
		} else {
			l[finalKey] = label.Value
		}
	}

	// Map service.name + service.namespace to job
	if haveServiceName {
		val := serviceName.AsString()
		if serviceNamespace, ok := resource.Attributes().Get(conventions.AttributeServiceNamespace); ok {
			val = fmt.Sprintf("%s/%s", serviceNamespace.AsString(), val)
		}
		l[model.JobLabel] = val
	}
	// Map service.instance.id to instance
	if haveInstanceID {
		l[model.InstanceLabel] = instance.AsString()
	}
	for key, value := range externalLabels {
		// External labels have already been sanitized
		if _, alreadyExists := l[key]; alreadyExists {
			// Skip external labels if they are overridden by metric attributes
			continue
		}
		l[key] = value
	}

	for i := 0; i < len(extras); i += 2 {
		if i+1 >= len(extras) {
			break
		}
		_, found := l[extras[i]]
		if found {
			log.Println("label " + extras[i] + " is overwritten. Check if Prometheus reserved labels are used.")
		}
		// internal labels should be maintained
		name := extras[i]
		if !(len(name) > 4 && name[:2] == "__" && name[len(name)-2:] == "__") {
			name = prometheustranslator.NormalizeLabel(name)
		}
		l[name] = extras[i+1]
	}

	s := make([]prompb.Label, 0, len(l))
	for k, v := range l {
		s = append(s, prompb.Label{Name: k, Value: v})
	}

	return s
}

// isValidAggregationTemporality checks whether an OTel metric has a valid
// aggregation temporality for conversion to a Prometheus metric.
func isValidAggregationTemporality(metric pmetric.Metric) bool {
	//exhaustive:enforce
	switch metric.Type() {
	case pmetric.MetricTypeGauge, pmetric.MetricTypeSummary:
		return true
	case pmetric.MetricTypeSum:
		return metric.Sum().AggregationTemporality() == pmetric.AggregationTemporalityCumulative
	case pmetric.MetricTypeHistogram:
		return metric.Histogram().AggregationTemporality() == pmetric.AggregationTemporalityCumulative
	case pmetric.MetricTypeExponentialHistogram:
		return metric.ExponentialHistogram().AggregationTemporality() == pmetric.AggregationTemporalityCumulative
	}
	return false
}

// addSingleHistogramDataPoint converts pt to 2 + min(len(ExplicitBounds), len(BucketCount)) + 1 samples. It
// ignore extra buckets if len(ExplicitBounds) > len(BucketCounts)
func addSingleHistogramDataPoint(pt pmetric.HistogramDataPoint, resource pcommon.Resource, metric pmetric.Metric, settings Settings, tsMap map[string]*prompb.TimeSeries, baseName string) {
	timestamp := convertTimeStamp(pt.Timestamp())
	baseLabels := createAttributes(resource, pt.Attributes(), settings.ExternalLabels)

	createLabels := func(nameSuffix string, extras ...string) []prompb.Label {
		extraLabelCount := len(extras) / 2
		labels := make([]prompb.Label, len(baseLabels), len(baseLabels)+extraLabelCount+1) // +1 for name
		copy(labels, baseLabels)

		for extrasIdx := 0; extrasIdx < extraLabelCount; extrasIdx++ {
			labels = append(labels, prompb.Label{Name: extras[extrasIdx], Value: extras[extrasIdx+1]})
		}

		// sum, count, and buckets of the histogram should append suffix to baseName
		labels = append(labels, prompb.Label{Name: model.MetricNameLabel, Value: baseName + nameSuffix})

		return labels
	}

	// If the sum is unset, it indicates the _sum metric point should be
	// omitted
	if pt.HasSum() {
		// treat sum as a sample in an individual TimeSeries
		sum := &prompb.Sample{
			Value:     pt.Sum(),
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			sum.Value = math.Float64frombits(value.StaleNaN)
		}

		sumlabels := createLabels(sumStr)
		addSample(tsMap, sum, sumlabels, metric.Type().String())

	}

	// treat count as a sample in an individual TimeSeries
	count := &prompb.Sample{
		Value:     float64(pt.Count()),
		Timestamp: timestamp,
	}
	if pt.Flags().NoRecordedValue() {
		count.Value = math.Float64frombits(value.StaleNaN)
	}

	countlabels := createLabels(countStr)
	addSample(tsMap, count, countlabels, metric.Type().String())

	// cumulative count for conversion to cumulative histogram
	var cumulativeCount uint64

	promExemplars := getPromExemplars[pmetric.HistogramDataPoint](pt)

	var bucketBounds []bucketBoundsData

	// process each bound, based on histograms proto definition, # of buckets = # of explicit bounds + 1
	for i := 0; i < pt.ExplicitBounds().Len() && i < pt.BucketCounts().Len(); i++ {
		bound := pt.ExplicitBounds().At(i)
		cumulativeCount += pt.BucketCounts().At(i)
		bucket := &prompb.Sample{
			Value:     float64(cumulativeCount),
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			bucket.Value = math.Float64frombits(value.StaleNaN)
		}
		boundStr := strconv.FormatFloat(bound, 'f', -1, 64)
		labels := createLabels(bucketStr, leStr, boundStr)
		sig := addSample(tsMap, bucket, labels, metric.Type().String())

		bucketBounds = append(bucketBounds, bucketBoundsData{sig: sig, bound: bound})
	}
	// add le=+Inf bucket
	infBucket := &prompb.Sample{
		Timestamp: timestamp,
	}
	if pt.Flags().NoRecordedValue() {
		infBucket.Value = math.Float64frombits(value.StaleNaN)
	} else {
		infBucket.Value = float64(pt.Count())
	}
	infLabels := createLabels(bucketStr, leStr, pInfStr)
	sig := addSample(tsMap, infBucket, infLabels, metric.Type().String())

	bucketBounds = append(bucketBounds, bucketBoundsData{sig: sig, bound: math.Inf(1)})
	addExemplars(tsMap, promExemplars, bucketBounds)

	// add _created time series if needed
	startTimestamp := pt.StartTimestamp()
	if settings.ExportCreatedMetric && startTimestamp != 0 {
		labels := createLabels(createdSuffix)
		addCreatedTimeSeriesIfNeeded(tsMap, labels, startTimestamp, pt.Timestamp(), metric.Type().String())
	}
}

type exemplarType interface {
	pmetric.ExponentialHistogramDataPoint | pmetric.HistogramDataPoint | pmetric.NumberDataPoint
	Exemplars() pmetric.ExemplarSlice
}

func getPromExemplars[T exemplarType](pt T) []prompb.Exemplar {
	promExemplars := make([]prompb.Exemplar, 0, pt.Exemplars().Len())
	for i := 0; i < pt.Exemplars().Len(); i++ {
		exemplar := pt.Exemplars().At(i)
		exemplarRunes := 0

		promExemplar := prompb.Exemplar{
			Value:     exemplar.DoubleValue(),
			Timestamp: timestamp.FromTime(exemplar.Timestamp().AsTime()),
		}
		if traceID := exemplar.TraceID(); !traceID.IsEmpty() {
			val := hex.EncodeToString(traceID[:])
			exemplarRunes += utf8.RuneCountInString(traceIDKey) + utf8.RuneCountInString(val)
			promLabel := prompb.Label{
				Name:  traceIDKey,
				Value: val,
			}
			promExemplar.Labels = append(promExemplar.Labels, promLabel)
		}
		if spanID := exemplar.SpanID(); !spanID.IsEmpty() {
			val := hex.EncodeToString(spanID[:])
			exemplarRunes += utf8.RuneCountInString(spanIDKey) + utf8.RuneCountInString(val)
			promLabel := prompb.Label{
				Name:  spanIDKey,
				Value: val,
			}
			promExemplar.Labels = append(promExemplar.Labels, promLabel)
		}

		attrs := exemplar.FilteredAttributes()
		labelsFromAttributes := make([]prompb.Label, 0, attrs.Len())
		attrs.Range(func(key string, value pcommon.Value) bool {
			val := value.AsString()
			exemplarRunes += utf8.RuneCountInString(key) + utf8.RuneCountInString(val)
			promLabel := prompb.Label{
				Name:  key,
				Value: val,
			}

			labelsFromAttributes = append(labelsFromAttributes, promLabel)

			return true
		})
		if exemplarRunes <= maxExemplarRunes {
			// only append filtered attributes if it does not cause exemplar
			// labels to exceed the max number of runes
			promExemplar.Labels = append(promExemplar.Labels, labelsFromAttributes...)
		}

		promExemplars = append(promExemplars, promExemplar)
	}

	return promExemplars
}

// mostRecentTimestampInMetric returns the latest timestamp in a batch of metrics
func mostRecentTimestampInMetric(metric pmetric.Metric) pcommon.Timestamp {
	var ts pcommon.Timestamp
	// handle individual metric based on type
	//exhaustive:enforce
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		dataPoints := metric.Gauge().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = maxTimestamp(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeSum:
		dataPoints := metric.Sum().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = maxTimestamp(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeHistogram:
		dataPoints := metric.Histogram().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = maxTimestamp(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeExponentialHistogram:
		dataPoints := metric.ExponentialHistogram().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = maxTimestamp(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeSummary:
		dataPoints := metric.Summary().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = maxTimestamp(ts, dataPoints.At(x).Timestamp())
		}
	}
	return ts
}

func maxTimestamp(a, b pcommon.Timestamp) pcommon.Timestamp {
	if a > b {
		return a
	}
	return b
}

// addSingleSummaryDataPoint converts pt to len(QuantileValues) + 2 samples.
func addSingleSummaryDataPoint(pt pmetric.SummaryDataPoint, resource pcommon.Resource, metric pmetric.Metric, settings Settings,
	tsMap map[string]*prompb.TimeSeries, baseName string) {
	timestamp := convertTimeStamp(pt.Timestamp())
	baseLabels := createAttributes(resource, pt.Attributes(), settings.ExternalLabels)

	createLabels := func(name string, extras ...string) []prompb.Label {
		extraLabelCount := len(extras) / 2
		labels := make([]prompb.Label, len(baseLabels), len(baseLabels)+extraLabelCount+1) // +1 for name
		copy(labels, baseLabels)

		for extrasIdx := 0; extrasIdx < extraLabelCount; extrasIdx++ {
			labels = append(labels, prompb.Label{Name: extras[extrasIdx], Value: extras[extrasIdx+1]})
		}

		labels = append(labels, prompb.Label{Name: model.MetricNameLabel, Value: name})

		return labels
	}

	// treat sum as a sample in an individual TimeSeries
	sum := &prompb.Sample{
		Value:     pt.Sum(),
		Timestamp: timestamp,
	}
	if pt.Flags().NoRecordedValue() {
		sum.Value = math.Float64frombits(value.StaleNaN)
	}
	// sum and count of the summary should append suffix to baseName
	sumlabels := createLabels(baseName + sumStr)
	addSample(tsMap, sum, sumlabels, metric.Type().String())

	// treat count as a sample in an individual TimeSeries
	count := &prompb.Sample{
		Value:     float64(pt.Count()),
		Timestamp: timestamp,
	}
	if pt.Flags().NoRecordedValue() {
		count.Value = math.Float64frombits(value.StaleNaN)
	}
	countlabels := createLabels(baseName + countStr)
	addSample(tsMap, count, countlabels, metric.Type().String())

	// process each percentile/quantile
	for i := 0; i < pt.QuantileValues().Len(); i++ {
		qt := pt.QuantileValues().At(i)
		quantile := &prompb.Sample{
			Value:     qt.Value(),
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			quantile.Value = math.Float64frombits(value.StaleNaN)
		}
		percentileStr := strconv.FormatFloat(qt.Quantile(), 'f', -1, 64)
		qtlabels := createLabels(baseName, quantileStr, percentileStr)
		addSample(tsMap, quantile, qtlabels, metric.Type().String())
	}

	// add _created time series if needed
	startTimestamp := pt.StartTimestamp()
	if settings.ExportCreatedMetric && startTimestamp != 0 {
		createdLabels := createLabels(baseName + createdSuffix)
		addCreatedTimeSeriesIfNeeded(tsMap, createdLabels, startTimestamp, pt.Timestamp(), metric.Type().String())
	}
}

// addCreatedTimeSeriesIfNeeded adds {name}_created time series with a single
// sample. If the series exists, then new samples won't be added.
func addCreatedTimeSeriesIfNeeded(
	series map[string]*prompb.TimeSeries,
	labels []prompb.Label,
	startTimestamp pcommon.Timestamp,
	timestamp pcommon.Timestamp,
	metricType string,
) {
	sig := timeSeriesSignature(metricType, labels)
	if _, ok := series[sig]; !ok {
		series[sig] = &prompb.TimeSeries{
			Labels: labels,
			Samples: []prompb.Sample{
				{ // convert ns to ms
					Value:     float64(convertTimeStamp(startTimestamp)),
					Timestamp: convertTimeStamp(timestamp),
				},
			},
		}
	}
}

// addResourceTargetInfo converts the resource to the target info metric
func addResourceTargetInfo(resource pcommon.Resource, settings Settings, timestamp pcommon.Timestamp, tsMap map[string]*prompb.TimeSeries) {
	if settings.DisableTargetInfo {
		return
	}
	// Use resource attributes (other than those used for job+instance) as the
	// metric labels for the target info metric
	attributes := pcommon.NewMap()
	resource.Attributes().CopyTo(attributes)
	attributes.RemoveIf(func(k string, _ pcommon.Value) bool {
		switch k {
		case conventions.AttributeServiceName, conventions.AttributeServiceNamespace, conventions.AttributeServiceInstanceID:
			// Remove resource attributes used for job + instance
			return true
		default:
			return false
		}
	})
	if attributes.Len() == 0 {
		// If we only have job + instance, then target_info isn't useful, so don't add it.
		return
	}
	// create parameters for addSample
	name := targetMetricName
	if len(settings.Namespace) > 0 {
		name = settings.Namespace + "_" + name
	}
	labels := createAttributes(resource, attributes, settings.ExternalLabels, model.MetricNameLabel, name)
	sample := &prompb.Sample{
		Value: float64(1),
		// convert ns to ms
		Timestamp: convertTimeStamp(timestamp),
	}
	addSample(tsMap, sample, labels, infoType)
}

// convertTimeStamp converts OTLP timestamp in ns to timestamp in ms
func convertTimeStamp(timestamp pcommon.Timestamp) int64 {
	return timestamp.AsTime().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}
