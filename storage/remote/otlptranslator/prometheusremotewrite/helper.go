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
	"strconv"
	"unicode/utf8"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
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
	traceIDKey       = "trace_id"
	spanIDKey        = "span_id"
	infoType         = "info"
	targetMetricName = "target_info"
)

type bucketBoundsData struct {
	ts    *writev2.TimeSeries
	bound float64
}

// byBucketBoundsData enables the usage of sort.Sort() with a slice of bucket bounds.
type byBucketBoundsData []bucketBoundsData

func (m byBucketBoundsData) Len() int           { return len(m) }
func (m byBucketBoundsData) Less(i, j int) bool { return m[i].bound < m[j].bound }
func (m byBucketBoundsData) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }

// createAttributes creates a slice of Prometheus Labels with OTLP attributes and pairs of string values.
// Unpaired string values are ignored. String pairs overwrite OTLP labels if collisions happen and
// if logOnOverwrite is true, the overwrite is logged. Resulting label names are sanitized.
// If settings.PromoteResourceAttributes is not empty, it's a set of resource attributes that should be promoted to labels.
func (c *PrometheusConverter) createAttributes(resource pcommon.Resource, attributes pcommon.Map, scope scope, settings Settings,
	ignoreAttrs []string, logOnOverwrite bool, extras ...string,
) labels.Labels {
	resourceAttrs := resource.Attributes()
	serviceName, haveServiceName := resourceAttrs.Get(conventions.AttributeServiceName)
	instance, haveInstanceID := resourceAttrs.Get(conventions.AttributeServiceInstanceID)

	promotedAttrs := settings.PromoteResourceAttributes.promotedAttributes(resourceAttrs)

	promoteScope := settings.PromoteScopeMetadata && scope.name != ""

	// Ensure attributes are sorted by key for consistent merging of keys which
	// collide when sanitized.
	c.scratchBuilder.Reset()

	// XXX: Should we always drop service namespace/service name/service instance ID from the labels
	// (as they get mapped to other Prometheus labels)?
	attributes.Range(func(key string, value pcommon.Value) bool {
		if !slices.Contains(ignoreAttrs, key) {
			c.scratchBuilder.Add(key, value.AsString())
		}
		return true
	})
	c.scratchBuilder.Sort()
	sortedLabels := c.scratchBuilder.Labels()

	// Now that we have sorted and filtered the labels, build the actual list
	// of labels, and handle conflicts by appending values.
	c.builder.Reset(labels.EmptyLabels())
	sortedLabels.Range(func(l labels.Label) {
		finalKey := l.Name
		if !settings.AllowUTF8 {
			finalKey = otlptranslator.NormalizeLabel(finalKey)
		}
		if existingValue := c.builder.Get(finalKey); existingValue != "" {
			c.builder.Set(finalKey, existingValue+";"+l.Value)
		} else {
			c.builder.Set(finalKey, l.Value)
		}
	})

	promotedAttrs.Range(func(l labels.Label) {
		normalized := l.Name
		if !settings.AllowUTF8 {
			normalized = otlptranslator.NormalizeLabel(normalized)
		}
		if existingValue := c.builder.Get(normalized); existingValue == "" {
			c.builder.Set(normalized, l.Value)
		}
	})

	if promoteScope {
		c.builder.Set("otel_scope_name", scope.name)
		c.builder.Set("otel_scope_version", scope.version)
		c.builder.Set("otel_scope_schema_url", scope.schemaURL)
		scope.attributes.Range(func(k string, v pcommon.Value) bool {
			name := "otel_scope_" + k
			if !settings.AllowUTF8 {
				name = otlptranslator.NormalizeLabel(name)
			}
			c.builder.Set(name, v.AsString())
			return true
		})
	}

	// Map service.name + service.namespace to job.
	if haveServiceName {
		val := serviceName.AsString()
		if serviceNamespace, ok := resourceAttrs.Get(conventions.AttributeServiceNamespace); ok {
			val = fmt.Sprintf("%s/%s", serviceNamespace.AsString(), val)
		}
		c.builder.Set(model.JobLabel, val)
	}
	// Map service.instance.id to instance.
	if haveInstanceID {
		c.builder.Set(model.InstanceLabel, instance.AsString())
	}
	for key, value := range settings.ExternalLabels {
		// External labels have already been sanitized.
		if existingValue := c.builder.Get(key); existingValue != "" {
			// Skip external labels if they are overridden by metric attributes.
			continue
		}
		c.builder.Set(key, value)
	}

	for i := 0; i < len(extras); i += 2 {
		if i+1 >= len(extras) {
			break
		}

		name := extras[i]
		if existingValue := c.builder.Get(name); existingValue != "" && logOnOverwrite {
			log.Println("label " + name + " is overwritten. Check if Prometheus reserved labels are used.")
		}
		// internal labels should be maintained.
		if !settings.AllowUTF8 && (len(name) <= 4 || name[:2] != "__" || name[len(name)-2:] != "__") {
			name = otlptranslator.NormalizeLabel(name)
		}
		c.builder.Set(name, extras[i+1])
	}

	return c.builder.Labels()
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
	resource pcommon.Resource, settings Settings, baseName string, scope scope, metadata writev2.Metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		baseLabels := c.createAttributes(resource, pt.Attributes(), scope, settings, nil, false)

		// If the sum is unset, it indicates the _sum metric point should be
		// omitted.
		if pt.HasSum() {
			// treat sum as a sample in an individual TimeSeries.
			sum := &writev2.Sample{
				Value:     pt.Sum(),
				Timestamp: timestamp,
			}
			if pt.Flags().NoRecordedValue() {
				sum.Value = math.Float64frombits(value.StaleNaN)
			}

			sumlabels := c.createLabels(baseName+sumStr, baseLabels)
			c.addSample(sum, sumlabels, metadata)
		}

		// treat count as a sample in an individual TimeSeries.
		count := &writev2.Sample{
			Value:     float64(pt.Count()),
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			count.Value = math.Float64frombits(value.StaleNaN)
		}

		countlabels := c.createLabels(baseName+countStr, baseLabels)
		c.addSample(count, countlabels, metadata)

		// cumulative count for conversion to cumulative histogram.
		var cumulativeCount uint64

		var bucketBounds []bucketBoundsData

		// process each bound, based on histograms proto definition, # of buckets = # of explicit bounds + 1.
		for i := 0; i < pt.ExplicitBounds().Len() && i < pt.BucketCounts().Len(); i++ {
			if err := c.everyN.checkContext(ctx); err != nil {
				return err
			}

			bound := pt.ExplicitBounds().At(i)
			cumulativeCount += pt.BucketCounts().At(i)
			bucket := &writev2.Sample{
				Value:     float64(cumulativeCount),
				Timestamp: timestamp,
			}
			if pt.Flags().NoRecordedValue() {
				bucket.Value = math.Float64frombits(value.StaleNaN)
			}
			boundStr := strconv.FormatFloat(bound, 'f', -1, 64)
			labels := c.createLabels(baseName+bucketStr, baseLabels, leStr, boundStr)
			ts := c.addSample(bucket, labels, metadata)

			bucketBounds = append(bucketBounds, bucketBoundsData{ts: ts, bound: bound})
		}
		// add le=+Inf bucket.
		infBucket := &writev2.Sample{
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			infBucket.Value = math.Float64frombits(value.StaleNaN)
		} else {
			infBucket.Value = float64(pt.Count())
		}
		infLabels := c.createLabels(baseName+bucketStr, baseLabels, leStr, pInfStr)
		ts := c.addSample(infBucket, infLabels, metadata)

		bucketBounds = append(bucketBounds, bucketBoundsData{ts: ts, bound: math.Inf(1)})
		if err := c.addExemplars(ctx, pt, bucketBounds); err != nil {
			return err
		}

		startTimestamp := pt.StartTimestamp()
		if settings.ExportCreatedMetric && startTimestamp != 0 {
			labels := c.createLabels(baseName+createdSuffix, baseLabels)
			c.addTimeSeriesIfNeeded(labels, startTimestamp, pt.Timestamp(), metadata)
		}
	}

	return nil
}

func (c *PrometheusConverter) getPromExemplars(ctx context.Context, exemplars pmetric.ExemplarSlice) ([]writev2.Exemplar, error) {
	promExemplars := make([]writev2.Exemplar, 0, exemplars.Len())
	for i := 0; i < exemplars.Len(); i++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return nil, err
		}

		exemplar := exemplars.At(i)
		exemplarRunes := 0

		promExemplar := writev2.Exemplar{
			Timestamp: timestamp.FromTime(exemplar.Timestamp().AsTime()),
		}
		c.scratchBuilder.Reset()
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
			c.scratchBuilder.Add(traceIDKey, val)
		}
		if spanID := exemplar.SpanID(); !spanID.IsEmpty() {
			val := hex.EncodeToString(spanID[:])
			exemplarRunes += utf8.RuneCountInString(spanIDKey) + utf8.RuneCountInString(val)
			c.scratchBuilder.Add(spanIDKey, val)
		}

		attrs := exemplar.FilteredAttributes()
		attrs.Range(func(key string, value pcommon.Value) bool {
			exemplarRunes += utf8.RuneCountInString(key) + utf8.RuneCountInString(value.AsString())
			return true
		})

		// Only append filtered attributes if it does not cause exemplar
		// labels to exceed the max number of runes.
		if exemplarRunes <= maxExemplarRunes {
			attrs.Range(func(key string, value pcommon.Value) bool {
				c.scratchBuilder.Add(key, value.AsString())
				return true
			})
		}
		c.scratchBuilder.Sort()
		promExemplar.LabelsRefs = c.symbolTable.SymbolizeLabels(c.scratchBuilder.Labels(), promExemplar.LabelsRefs)

		promExemplars = append(promExemplars, promExemplar)
	}

	return promExemplars, nil
}

// mostRecentTimestampInMetric returns the latest timestamp in a batch of metrics.
func mostRecentTimestampInMetric(metric pmetric.Metric) pcommon.Timestamp {
	var ts pcommon.Timestamp
	// handle individual metric based on type.
	//exhaustive:enforce
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		dataPoints := metric.Gauge().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeSum:
		dataPoints := metric.Sum().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeHistogram:
		dataPoints := metric.Histogram().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeExponentialHistogram:
		dataPoints := metric.ExponentialHistogram().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeSummary:
		dataPoints := metric.Summary().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	}
	return ts
}

func (c *PrometheusConverter) addSummaryDataPoints(ctx context.Context, dataPoints pmetric.SummaryDataPointSlice, resource pcommon.Resource,
	settings Settings, baseName string, scope scope, metadata writev2.Metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		baseLabels := c.createAttributes(resource, pt.Attributes(), scope, settings, nil, false)

		// treat sum as a sample in an individual TimeSeries.
		sum := &writev2.Sample{
			Value:     pt.Sum(),
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			sum.Value = math.Float64frombits(value.StaleNaN)
		}
		// sum and count of the summary should append suffix to baseName.
		sumlabels := c.createLabels(baseName+sumStr, baseLabels)
		c.addSample(sum, sumlabels, metadata)

		// treat count as a sample in an individual TimeSeries.
		count := &writev2.Sample{
			Value:     float64(pt.Count()),
			Timestamp: timestamp,
		}
		if pt.Flags().NoRecordedValue() {
			count.Value = math.Float64frombits(value.StaleNaN)
		}
		countlabels := c.createLabels(baseName+countStr, baseLabels)
		c.addSample(count, countlabels, metadata)

		// process each percentile/quantile.
		for i := 0; i < pt.QuantileValues().Len(); i++ {
			qt := pt.QuantileValues().At(i)
			quantile := &writev2.Sample{
				Value:     qt.Value(),
				Timestamp: timestamp,
			}
			if pt.Flags().NoRecordedValue() {
				quantile.Value = math.Float64frombits(value.StaleNaN)
			}
			percentileStr := strconv.FormatFloat(qt.Quantile(), 'f', -1, 64)
			qtlabels := c.createLabels(baseName, baseLabels, quantileStr, percentileStr)
			c.addSample(quantile, qtlabels, metadata)
		}

		startTimestamp := pt.StartTimestamp()
		if settings.ExportCreatedMetric && startTimestamp != 0 {
			createdLabels := c.createLabels(baseName+createdSuffix, baseLabels)
			c.addTimeSeriesIfNeeded(createdLabels, startTimestamp, pt.Timestamp(), metadata)
		}
	}

	return nil
}

// createLabels returns a copy of baseLabels, adding to it the pair model.MetricNameLabel=name.
// If extras are provided, corresponding label pairs are also added to the returned slice.
// If extras is uneven length, the last (unpaired) extra will be ignored.
func (c *PrometheusConverter) createLabels(name string, baseLabels labels.Labels, extras ...string) labels.Labels {
	c.builder.Reset(baseLabels)

	n := len(extras)
	n -= n % 2
	for extrasIdx := 0; extrasIdx < n; extrasIdx += 2 {
		c.builder.Set(extras[extrasIdx], extras[extrasIdx+1])
	}
	c.builder.Set(model.MetricNameLabel, name)
	return c.builder.Labels()
}

// getOrCreateTimeSeries returns the time series corresponding to the label set if existent, and false.
// Otherwise it creates a new one and returns that, and true.
func (c *PrometheusConverter) getOrCreateTimeSeries(lbls labels.Labels, metadata writev2.Metadata) (*writev2.TimeSeries, bool) {
	h := lbls.Hash()
	ts := c.unique[h]
	if ts != nil {
		if labels.Equal(ts.ToLabels(&c.scratchBuilder, c.symbolTable.Symbols()), lbls) {
			// We already have this metric.
			return ts, false
		}

		// Look for a matching conflict.
		for _, cTS := range c.conflicts[h] {
			if labels.Equal(cTS.ToLabels(&c.scratchBuilder, c.symbolTable.Symbols()), lbls) {
				// We already have this metric.
				return cTS, false
			}
		}

		// New conflict.
		ts = &writev2.TimeSeries{
			LabelsRefs: c.symbolTable.SymbolizeLabels(lbls, nil),
			Metadata:   metadata,
		}
		c.conflicts[h] = append(c.conflicts[h], ts)
		return ts, true
	}

	// This metric is new.
	ts = &writev2.TimeSeries{
		LabelsRefs: c.symbolTable.SymbolizeLabels(lbls, nil),
		Metadata:   metadata,
	}
	c.unique[h] = ts
	return ts, true
}

// addTimeSeriesIfNeeded adds a corresponding time series if it doesn't already exist.
// If the time series doesn't already exist, it gets added with startTimestamp for its value and timestamp for its timestamp,
// both converted to milliseconds.
func (c *PrometheusConverter) addTimeSeriesIfNeeded(lbls labels.Labels, startTimestamp, timestamp pcommon.Timestamp, metadata writev2.Metadata) {
	ts, created := c.getOrCreateTimeSeries(lbls, metadata)
	if created {
		ts.Samples = []writev2.Sample{
			{
				// convert ns to ms.
				Value:     float64(convertTimeStamp(startTimestamp)),
				Timestamp: convertTimeStamp(timestamp),
			},
		}
	}
}

// addResourceTargetInfo converts the resource to the target info metric.
func (c *PrometheusConverter) addResourceTargetInfo(resource pcommon.Resource, settings Settings, timestamp pcommon.Timestamp) {
	if settings.DisableTargetInfo || timestamp == 0 {
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
	lbls := c.createAttributes(resource, attributes, scope{}, settings, identifyingAttrs, false, model.MetricNameLabel, name)
	haveIdentifier := false
	lbls.Range(func(l labels.Label) {
		if l.Name == model.JobLabel || l.Name == model.InstanceLabel {
			haveIdentifier = true
		}
	})

	if !haveIdentifier {
		// We need at least one identifying label to generate target_info.
		return
	}

	sample := &writev2.Sample{
		Value: float64(1),
		// convert ns to ms
		Timestamp: convertTimeStamp(timestamp),
	}
	metadata := writev2.Metadata{
		Type:    writev2.Metadata_METRIC_TYPE_GAUGE,
		HelpRef: c.symbolTable.Symbolize("Target metadata"),
	}
	c.addSample(sample, lbls, metadata)
}

// convertTimeStamp converts OTLP timestamp in ns to timestamp in ms.
func convertTimeStamp(timestamp pcommon.Timestamp) int64 {
	return int64(timestamp) / 1_000_000
}
