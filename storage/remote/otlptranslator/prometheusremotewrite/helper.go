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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheusremotewrite/helper.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"

	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
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
	// https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#exemplars
	maxExemplarRunes = 128
	// Trace and Span id keys are defined as part of the spec:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification%2Fmetrics%2Fdatamodel.md#exemplars-2
	traceIDKey           = "trace_id"
	spanIDKey            = "span_id"
	infoType             = "info"
	targetMetricName     = "target_info"
	defaultLookbackDelta = 5 * time.Minute
)

type bucketBoundsData struct {
	ts    *prompb.TimeSeries
	bound float64
}

// byBucketBoundsData enables the usage of sort.Sort() with a slice of bucket bounds.
type byBucketBoundsData []bucketBoundsData

func (m byBucketBoundsData) Len() int           { return len(m) }
func (m byBucketBoundsData) Less(i, j int) bool { return m[i].bound < m[j].bound }
func (m byBucketBoundsData) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }

// ByLabelName enables the usage of sort.Sort() with a slice of labels.
type ByLabelName []prompb.Label

func (a ByLabelName) Len() int           { return len(a) }
func (a ByLabelName) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a ByLabelName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// timeSeriesSignature returns a hashed label set signature.
// The label slice should not contain duplicate label names; this method sorts the slice by label name before creating
// the signature.
// The algorithm is the same as in Prometheus' labels.StableHash function.
func timeSeriesSignature(labels []prompb.Label) uint64 {
	sort.Sort(ByLabelName(labels))

	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024)
	for i, v := range labels {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			for _, v := range labels[i:] {
				_, _ = h.WriteString(v.Name)
				_, _ = h.Write(seps)
				_, _ = h.WriteString(v.Value)
				_, _ = h.Write(seps)
			}
			return h.Sum64()
		}

		b = append(b, v.Name...)
		b = append(b, seps[0])
		b = append(b, v.Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b)
}

var seps = []byte{'\xff'}

// createAttributes creates a slice of Prometheus Labels with OTLP attributes and pairs of string values.
// Unpaired string values are ignored. String pairs overwrite OTLP labels if collisions happen and
// if logOnOverwrite is true, the overwrite is logged. Resulting label names are sanitized.
// If settings.PromoteResourceAttributes is not empty, it's a set of resource attributes that should be promoted to labels.
func createAttributes(resource pcommon.Resource, attributes pcommon.Map, scope scope, settings Settings,
	ignoreAttrs []string, logOnOverwrite bool, extras ...string,
) []prompb.Label {
	resourceAttrs := resource.Attributes()
	serviceName, haveServiceName := resourceAttrs.Get(conventions.AttributeServiceName)
	instance, haveInstanceID := resourceAttrs.Get(conventions.AttributeServiceInstanceID)

	promotedAttrs := settings.PromoteResourceAttributes.promotedAttributes(resourceAttrs)

	promoteScope := settings.PromoteScopeMetadata && scope.name != ""
	scopeLabelCount := 0
	if promoteScope {
		// Include name, version and schema URL.
		scopeLabelCount = scope.attributes.Len() + 3
	}
	// Calculate the maximum possible number of labels we could return so we can preallocate l.
	maxLabelCount := attributes.Len() + len(settings.ExternalLabels) + len(promotedAttrs) + scopeLabelCount + len(extras)/2

	if haveServiceName {
		maxLabelCount++
	}
	if haveInstanceID {
		maxLabelCount++
	}

	// Ensure attributes are sorted by key for consistent merging of keys which
	// collide when sanitized.
	labels := make([]prompb.Label, 0, maxLabelCount)
	// XXX: Should we always drop service namespace/service name/service instance ID from the labels
	// (as they get mapped to other Prometheus labels)?
	attributes.Range(func(key string, value pcommon.Value) bool {
		if !slices.Contains(ignoreAttrs, key) {
			labels = append(labels, prompb.Label{Name: key, Value: value.AsString()})
		}
		return true
	})
	sort.Stable(ByLabelName(labels))

	// map ensures no duplicate label names.
	l := make(map[string]string, maxLabelCount)
	labelNamer := otlptranslator.LabelNamer{UTF8Allowed: settings.AllowUTF8}
	for _, label := range labels {
		finalKey := labelNamer.Build(label.Name)
		if existingValue, alreadyExists := l[finalKey]; alreadyExists {
			l[finalKey] = existingValue + ";" + label.Value
		} else {
			l[finalKey] = label.Value
		}
	}

	for _, lbl := range promotedAttrs {
		normalized := labelNamer.Build(lbl.Name)
		if _, exists := l[normalized]; !exists {
			l[normalized] = lbl.Value
		}
	}
	if promoteScope {
		l["otel_scope_name"] = scope.name
		l["otel_scope_version"] = scope.version
		l["otel_scope_schema_url"] = scope.schemaURL
		scope.attributes.Range(func(k string, v pcommon.Value) bool {
			name := "otel_scope_" + k
			name = labelNamer.Build(name)
			l[name] = v.AsString()
			return true
		})
	}

	// Map service.name + service.namespace to job.
	if haveServiceName {
		val := serviceName.AsString()
		if serviceNamespace, ok := resourceAttrs.Get(conventions.AttributeServiceNamespace); ok {
			val = fmt.Sprintf("%s/%s", serviceNamespace.AsString(), val)
		}
		l[model.JobLabel] = val
	}
	// Map service.instance.id to instance.
	if haveInstanceID {
		l[model.InstanceLabel] = instance.AsString()
	}
	for key, value := range settings.ExternalLabels {
		// External labels have already been sanitized.
		if _, alreadyExists := l[key]; alreadyExists {
			// Skip external labels if they are overridden by metric attributes.
			continue
		}
		l[key] = value
	}

	for i := 0; i < len(extras); i += 2 {
		if i+1 >= len(extras) {
			break
		}

		name := extras[i]
		_, found := l[name]
		if found && logOnOverwrite {
			log.Println("label " + name + " is overwritten. Check if Prometheus reserved labels are used.")
		}
		// internal labels should be maintained.
		if len(name) <= 4 || name[:2] != "__" || name[len(name)-2:] != "__" {
			name = labelNamer.Build(name)
		}
		l[name] = extras[i+1]
	}

	labels = labels[:0]
	for k, v := range l {
		labels = append(labels, prompb.Label{Name: k, Value: v})
	}

	return labels
}

func aggregationTemporality(metric pmetric.Metric) (pmetric.AggregationTemporality, bool, error) {
	//exhaustive:enforce
	switch metric.Type() {
	case pmetric.MetricTypeGauge, pmetric.MetricTypeSummary:
		return 0, false, nil
	case pmetric.MetricTypeSum:
		return metric.Sum().AggregationTemporality(), true, nil
	case pmetric.MetricTypeHistogram:
		return metric.Histogram().AggregationTemporality(), true, nil
	case pmetric.MetricTypeExponentialHistogram:
		return metric.ExponentialHistogram().AggregationTemporality(), true, nil
	}
	return 0, false, fmt.Errorf("could not get aggregation temporality for %s as it has unsupported metric type %s", metric.Name(), metric.Type())
}

// addHistogramDataPoints adds OTel histogram data points to the corresponding Prometheus time series
// as classical histogram samples.
//
// Note that we can't convert to native histograms, since these have exponential buckets and don't line up
// with the user defined bucket boundaries of non-exponential OTel histograms.
// However, work is under way to resolve this shortcoming through a feature called native histograms custom buckets:
// https://github.com/prometheus/prometheus/issues/13485.
func (c *PrometheusConverter) addHistogramDataPoints(ctx context.Context, dataPoints pmetric.HistogramDataPointSlice,
	resource pcommon.Resource, settings Settings, metadata prompb.MetricMetadata, scope scope,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		baseLabels := createAttributes(resource, pt.Attributes(), scope, settings, nil, false)

		if settings.AddTypeAndUnitLabels {
			baseLabels = addTypeAndUnitLabels(baseLabels, metadata, settings)
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

			sumlabels := createLabels(metadata.MetricFamilyName+sumStr, baseLabels)
			c.addSample(sum, sumlabels)
		}

		// treat count as a sample in an individual TimeSeries
		count := &prompb.Sample{
			Value:     float64(pt.Count()),
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			count.Value = math.Float64frombits(value.StaleNaN)
		}

		countlabels := createLabels(metadata.MetricFamilyName+countStr, baseLabels)
		c.addSample(count, countlabels)

		// cumulative count for conversion to cumulative histogram
		var cumulativeCount uint64

		var bucketBounds []bucketBoundsData

		// process each bound, based on histograms proto definition, # of buckets = # of explicit bounds + 1
		for i := 0; i < pt.ExplicitBounds().Len() && i < pt.BucketCounts().Len(); i++ {
			if err := c.everyN.checkContext(ctx); err != nil {
				return err
			}

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
			labels := createLabels(metadata.MetricFamilyName+bucketStr, baseLabels, leStr, boundStr)
			ts := c.addSample(bucket, labels)

			bucketBounds = append(bucketBounds, bucketBoundsData{ts: ts, bound: bound})
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
		infLabels := createLabels(metadata.MetricFamilyName+bucketStr, baseLabels, leStr, pInfStr)
		ts := c.addSample(infBucket, infLabels)

		bucketBounds = append(bucketBounds, bucketBoundsData{ts: ts, bound: math.Inf(1)})
		if err := c.addExemplars(ctx, pt, bucketBounds); err != nil {
			return err
		}

		startTimestamp := pt.StartTimestamp()
		if settings.ExportCreatedMetric && startTimestamp != 0 {
			labels := createLabels(metadata.MetricFamilyName+createdSuffix, baseLabels)
			c.addTimeSeriesIfNeeded(labels, startTimestamp, pt.Timestamp())
		}
	}

	return nil
}

type exemplarType interface {
	pmetric.ExponentialHistogramDataPoint | pmetric.HistogramDataPoint | pmetric.NumberDataPoint
	Exemplars() pmetric.ExemplarSlice
}

func getPromExemplars[T exemplarType](ctx context.Context, everyN *everyNTimes, pt T) ([]prompb.Exemplar, error) {
	promExemplars := make([]prompb.Exemplar, 0, pt.Exemplars().Len())
	for i := 0; i < pt.Exemplars().Len(); i++ {
		if err := everyN.checkContext(ctx); err != nil {
			return nil, err
		}

		exemplar := pt.Exemplars().At(i)
		exemplarRunes := 0

		promExemplar := prompb.Exemplar{
			Timestamp: timestamp.FromTime(exemplar.Timestamp().AsTime()),
		}
		switch exemplar.ValueType() {
		case pmetric.ExemplarValueTypeInt:
			promExemplar.Value = float64(exemplar.IntValue())
		case pmetric.ExemplarValueTypeDouble:
			promExemplar.Value = exemplar.DoubleValue()
		default:
			return nil, fmt.Errorf("unsupported exemplar value type: %v", exemplar.ValueType())
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

	return promExemplars, nil
}

// findMinAndMaxTimestamps returns the minimum of minTimestamp and the earliest timestamp in metric and
// the maximum of maxTimestamp and the latest timestamp in metric, respectively.
func findMinAndMaxTimestamps(metric pmetric.Metric, minTimestamp, maxTimestamp pcommon.Timestamp) (pcommon.Timestamp, pcommon.Timestamp) {
	// handle individual metric based on type
	//exhaustive:enforce
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		dataPoints := metric.Gauge().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts := dataPoints.At(x).Timestamp()
			minTimestamp = min(minTimestamp, ts)
			maxTimestamp = max(maxTimestamp, ts)
		}
	case pmetric.MetricTypeSum:
		dataPoints := metric.Sum().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts := dataPoints.At(x).Timestamp()
			minTimestamp = min(minTimestamp, ts)
			maxTimestamp = max(maxTimestamp, ts)
		}
	case pmetric.MetricTypeHistogram:
		dataPoints := metric.Histogram().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts := dataPoints.At(x).Timestamp()
			minTimestamp = min(minTimestamp, ts)
			maxTimestamp = max(maxTimestamp, ts)
		}
	case pmetric.MetricTypeExponentialHistogram:
		dataPoints := metric.ExponentialHistogram().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts := dataPoints.At(x).Timestamp()
			minTimestamp = min(minTimestamp, ts)
			maxTimestamp = max(maxTimestamp, ts)
		}
	case pmetric.MetricTypeSummary:
		dataPoints := metric.Summary().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts := dataPoints.At(x).Timestamp()
			minTimestamp = min(minTimestamp, ts)
			maxTimestamp = max(maxTimestamp, ts)
		}
	}
	return minTimestamp, maxTimestamp
}

func (c *PrometheusConverter) addSummaryDataPoints(ctx context.Context, dataPoints pmetric.SummaryDataPointSlice, resource pcommon.Resource,
	settings Settings, metadata prompb.MetricMetadata, scope scope,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		baseLabels := createAttributes(resource, pt.Attributes(), scope, settings, nil, false)

		if settings.AddTypeAndUnitLabels {
			baseLabels = addTypeAndUnitLabels(baseLabels, metadata, settings)
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
		sumlabels := createLabels(metadata.MetricFamilyName+sumStr, baseLabels)
		c.addSample(sum, sumlabels)

		// treat count as a sample in an individual TimeSeries
		count := &prompb.Sample{
			Value:     float64(pt.Count()),
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			count.Value = math.Float64frombits(value.StaleNaN)
		}
		countlabels := createLabels(metadata.MetricFamilyName+countStr, baseLabels)
		c.addSample(count, countlabels)

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
			qtlabels := createLabels(metadata.MetricFamilyName, baseLabels, quantileStr, percentileStr)
			c.addSample(quantile, qtlabels)
		}

		startTimestamp := pt.StartTimestamp()
		if settings.ExportCreatedMetric && startTimestamp != 0 {
			createdLabels := createLabels(metadata.MetricFamilyName+createdSuffix, baseLabels)
			c.addTimeSeriesIfNeeded(createdLabels, startTimestamp, pt.Timestamp())
		}
	}

	return nil
}

// createLabels returns a copy of baseLabels, adding to it the pair model.MetricNameLabel=name.
// If extras are provided, corresponding label pairs are also added to the returned slice.
// If extras is uneven length, the last (unpaired) extra will be ignored.
func createLabels(name string, baseLabels []prompb.Label, extras ...string) []prompb.Label {
	extraLabelCount := len(extras) / 2
	labels := make([]prompb.Label, len(baseLabels), len(baseLabels)+extraLabelCount+1) // +1 for name
	copy(labels, baseLabels)

	n := len(extras)
	n -= n % 2
	for extrasIdx := 0; extrasIdx < n; extrasIdx += 2 {
		labels = append(labels, prompb.Label{Name: extras[extrasIdx], Value: extras[extrasIdx+1]})
	}

	labels = append(labels, prompb.Label{Name: model.MetricNameLabel, Value: name})
	return labels
}

// addTypeAndUnitLabels appends type and unit labels to the given labels slice.
func addTypeAndUnitLabels(labels []prompb.Label, metadata prompb.MetricMetadata, settings Settings) []prompb.Label {
	unitNamer := otlptranslator.UnitNamer{UTF8Allowed: settings.AllowUTF8}

	var hasTypeLabel, hasUnitLabel bool
	for _, l := range labels {
		if l.Name == "__type__" {
			hasTypeLabel = true
		}
		if l.Name == "__unit__" {
			hasUnitLabel = true
		}
	}
	if !hasTypeLabel {
		labels = append(labels, prompb.Label{Name: "__type__", Value: strings.ToLower(metadata.Type.String())})
	}
	if !hasUnitLabel {
		labels = append(labels, prompb.Label{Name: "__unit__", Value: unitNamer.Build(metadata.Unit)})
	}

	return labels
}

// getOrCreateTimeSeries returns the time series corresponding to the label set if existent, and false.
// Otherwise it creates a new one and returns that, and true.
func (c *PrometheusConverter) getOrCreateTimeSeries(lbls []prompb.Label) (*prompb.TimeSeries, bool) {
	h := timeSeriesSignature(lbls)
	ts := c.unique[h]
	if ts != nil {
		if isSameMetric(ts, lbls) {
			// We already have this metric
			return ts, false
		}

		// Look for a matching conflict
		for _, cTS := range c.conflicts[h] {
			if isSameMetric(cTS, lbls) {
				// We already have this metric
				return cTS, false
			}
		}

		// New conflict
		ts = &prompb.TimeSeries{
			Labels: lbls,
		}
		c.conflicts[h] = append(c.conflicts[h], ts)
		return ts, true
	}

	// This metric is new
	ts = &prompb.TimeSeries{
		Labels: lbls,
	}
	c.unique[h] = ts
	return ts, true
}

// addTimeSeriesIfNeeded adds a corresponding time series if it doesn't already exist.
// If the time series doesn't already exist, it gets added with startTimestamp for its value and timestamp for its timestamp,
// both converted to milliseconds.
func (c *PrometheusConverter) addTimeSeriesIfNeeded(lbls []prompb.Label, startTimestamp, timestamp pcommon.Timestamp) {
	ts, created := c.getOrCreateTimeSeries(lbls)
	if created {
		ts.Samples = []prompb.Sample{
			{
				// convert ns to ms
				Value:     float64(convertTimeStamp(startTimestamp)),
				Timestamp: convertTimeStamp(timestamp),
			},
		}
	}
}

// addResourceTargetInfo converts the resource to the target info metric.
func addResourceTargetInfo(resource pcommon.Resource, settings Settings, earliestTimestamp, latestTimestamp time.Time, converter *PrometheusConverter) {
	if settings.DisableTargetInfo {
		return
	}

	attributes := resource.Attributes()
	identifyingAttrs := []string{
		conventions.AttributeServiceNamespace,
		conventions.AttributeServiceName,
		conventions.AttributeServiceInstanceID,
	}
	nonIdentifyingAttrsCount := attributes.Len()
	for _, a := range identifyingAttrs {
		_, haveAttr := attributes.Get(a)
		if haveAttr {
			nonIdentifyingAttrsCount--
		}
	}
	if nonIdentifyingAttrsCount == 0 {
		// If we only have job + instance, then target_info isn't useful, so don't add it.
		return
	}

	name := targetMetricName
	if len(settings.Namespace) > 0 {
		name = settings.Namespace + "_" + name
	}

	settings.PromoteResourceAttributes = nil
	if settings.KeepIdentifyingResourceAttributes {
		// Do not pass identifying attributes as ignoreAttrs below.
		identifyingAttrs = nil
	}
	labels := createAttributes(resource, attributes, scope{}, settings, identifyingAttrs, false, model.MetricNameLabel, name)
	haveIdentifier := false
	for _, l := range labels {
		if l.Name == model.JobLabel || l.Name == model.InstanceLabel {
			haveIdentifier = true
			break
		}
	}

	if !haveIdentifier {
		// We need at least one identifying label to generate target_info.
		return
	}

	// Generate target_info samples starting at earliestTimestamp and ending at latestTimestamp,
	// with a sample at every interval between them.
	// Use an interval corresponding to half of the lookback delta, to ensure that target_info samples are found
	// for the entirety of the relevant period.
	if settings.LookbackDelta == 0 {
		settings.LookbackDelta = defaultLookbackDelta
	}
	interval := settings.LookbackDelta / 2
	ts, _ := converter.getOrCreateTimeSeries(labels)
	for timestamp := earliestTimestamp; timestamp.Before(latestTimestamp); timestamp = timestamp.Add(interval) {
		ts.Samples = append(ts.Samples, prompb.Sample{
			Value:     float64(1),
			Timestamp: timestamp.UnixMilli(),
		})
	}
	if len(ts.Samples) == 0 || ts.Samples[len(ts.Samples)-1].Timestamp < latestTimestamp.UnixMilli() {
		ts.Samples = append(ts.Samples, prompb.Sample{
			Value:     float64(1),
			Timestamp: latestTimestamp.UnixMilli(),
		})
	}
}

// convertTimeStamp converts OTLP timestamp in ns to timestamp in ms.
func convertTimeStamp(timestamp pcommon.Timestamp) int64 {
	return int64(timestamp) / 1_000_000
}
