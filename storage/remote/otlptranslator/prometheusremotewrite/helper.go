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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
)

const (
	sumStr      = "_sum"
	countStr    = "_count"
	bucketStr   = "_bucket"
	leStr       = "le"
	quantileStr = "quantile"
	pInfStr     = "+Inf"
	// maxExemplarRunes is the maximum number of UTF-8 exemplar characters
	// according to the prometheus specification
	// https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#exemplars
	maxExemplarRunes = 128
	// Trace and Span id keys are defined as part of the spec:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification%2Fmetrics%2Fdatamodel.md#exemplars-2
	traceIDKey           = "trace_id"
	spanIDKey            = "span_id"
	targetMetricName     = "target_info"
	defaultLookbackDelta = 5 * time.Minute
)

// createAttributes creates a slice of Prometheus Labels with OTLP attributes and pairs of string values.
// Unpaired string values are ignored. String pairs overwrite OTLP labels if collisions happen and
// if logOnOverwrite is true, the overwrite is logged. Resulting label names are sanitized.
// If settings.PromoteResourceAttributes is not empty, it's a set of resource attributes that should be promoted to labels.
func (c *PrometheusConverter) createAttributes(resource pcommon.Resource, attributes pcommon.Map, scope scope, settings Settings,
	ignoreAttrs []string, logOnOverwrite bool, meta Metadata, extras ...string,
) (labels.Labels, error) {
	resourceAttrs := resource.Attributes()
	serviceName, haveServiceName := resourceAttrs.Get(conventions.AttributeServiceName)
	instance, haveInstanceID := resourceAttrs.Get(conventions.AttributeServiceInstanceID)

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

	labelNamer := otlptranslator.LabelNamer{
		UTF8Allowed:                 settings.AllowUTF8,
		UnderscoreLabelSanitization: settings.LabelNameUnderscoreSanitization,
		PreserveMultipleUnderscores: settings.LabelNamePreserveMultipleUnderscores,
	}

	if settings.AllowUTF8 {
		// UTF8 is allowed, so conflicts aren't possible.
		c.builder.Reset(sortedLabels)
	} else {
		// Now that we have sorted and filtered the labels, build the actual list
		// of labels, and handle conflicts by appending values.
		c.builder.Reset(labels.EmptyLabels())
		var sortErr error
		sortedLabels.Range(func(l labels.Label) {
			if sortErr != nil {
				return
			}
			finalKey, err := labelNamer.Build(l.Name)
			if err != nil {
				sortErr = err
				return
			}
			if existingValue := c.builder.Get(finalKey); existingValue != "" {
				c.builder.Set(finalKey, existingValue+";"+l.Value)
			} else {
				c.builder.Set(finalKey, l.Value)
			}
		})
		if sortErr != nil {
			return labels.EmptyLabels(), sortErr
		}
	}

	err := settings.PromoteResourceAttributes.addPromotedAttributes(c.builder, resourceAttrs, labelNamer)
	if err != nil {
		return labels.EmptyLabels(), err
	}
	if promoteScope {
		var rangeErr error
		scope.attributes.Range(func(k string, v pcommon.Value) bool {
			name, err := labelNamer.Build("otel_scope_" + k)
			if err != nil {
				rangeErr = err
				return false
			}
			c.builder.Set(name, v.AsString())
			return true
		})
		if rangeErr != nil {
			return labels.EmptyLabels(), rangeErr
		}
		// Scope Name, Version and Schema URL are added after attributes to ensure they are not overwritten by attributes.
		c.builder.Set("otel_scope_name", scope.name)
		c.builder.Set("otel_scope_version", scope.version)
		c.builder.Set("otel_scope_schema_url", scope.schemaURL)
	}

	if settings.EnableTypeAndUnitLabels {
		unitNamer := otlptranslator.UnitNamer{UTF8Allowed: settings.AllowUTF8}
		if meta.Type != model.MetricTypeUnknown {
			c.builder.Set(model.MetricTypeLabel, strings.ToLower(string(meta.Type)))
		}
		if meta.Unit != "" {
			c.builder.Set(model.MetricUnitLabel, unitNamer.Build(meta.Unit))
		}
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
		if len(name) <= 4 || name[:2] != "__" || name[len(name)-2:] != "__" {
			var err error
			name, err = labelNamer.Build(name)
			if err != nil {
				return labels.EmptyLabels(), err
			}
		}
		c.builder.Set(name, extras[i+1])
	}

	return c.builder.Labels(), nil
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
	resource pcommon.Resource, settings Settings, scope scope, meta Metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		startTimestamp := convertTimeStamp(pt.StartTimestamp())
		baseLabels, err := c.createAttributes(resource, pt.Attributes(), scope, settings, nil, false, meta)
		if err != nil {
			return err
		}

		baseName := meta.MetricFamilyName

		// If the sum is unset, it indicates the _sum metric point should be
		// omitted
		if pt.HasSum() {
			// treat sum as a sample in an individual TimeSeries
			val := pt.Sum()
			if pt.Flags().NoRecordedValue() {
				val = math.Float64frombits(value.StaleNaN)
			}

			sumlabels := c.addLabels(baseName+sumStr, baseLabels)
			if err := c.appender.AppendSample(sumlabels, meta, startTimestamp, timestamp, val, nil); err != nil {
				return err
			}
		}

		// treat count as a sample in an individual TimeSeries
		val := float64(pt.Count())
		if pt.Flags().NoRecordedValue() {
			val = math.Float64frombits(value.StaleNaN)
		}

		countlabels := c.addLabels(baseName+countStr, baseLabels)
		if err := c.appender.AppendSample(countlabels, meta, startTimestamp, timestamp, val, nil); err != nil {
			return err
		}
		exemplars, err := c.getPromExemplars(ctx, pt.Exemplars())
		if err != nil {
			return err
		}
		nextExemplarIdx := 0

		// cumulative count for conversion to cumulative histogram
		var cumulativeCount uint64

		// process each bound, based on histograms proto definition, # of buckets = # of explicit bounds + 1
		for i := 0; i < pt.ExplicitBounds().Len() && i < pt.BucketCounts().Len(); i++ {
			if err := c.everyN.checkContext(ctx); err != nil {
				return err
			}

			bound := pt.ExplicitBounds().At(i)
			cumulativeCount += pt.BucketCounts().At(i)

			// Find exemplars that belong to this bucket. Both exemplars and
			// buckets are sorted in ascending order.
			var currentBucketExemplars []exemplar.Exemplar
			for ; nextExemplarIdx < len(exemplars); nextExemplarIdx++ {
				ex := exemplars[nextExemplarIdx]
				if ex.Value > bound {
					// This exemplar belongs in a higher bucket.
					break
				}
				currentBucketExemplars = append(currentBucketExemplars, ex)
			}
			val := float64(cumulativeCount)
			if pt.Flags().NoRecordedValue() {
				val = math.Float64frombits(value.StaleNaN)
			}
			boundStr := strconv.FormatFloat(bound, 'f', -1, 64)
			labels := c.addLabels(baseName+bucketStr, baseLabels, leStr, boundStr)
			if err := c.appender.AppendSample(labels, meta, startTimestamp, timestamp, val, currentBucketExemplars); err != nil {
				return err
			}
		}
		// add le=+Inf bucket
		val = float64(pt.Count())
		if pt.Flags().NoRecordedValue() {
			val = math.Float64frombits(value.StaleNaN)
		}
		infLabels := c.addLabels(baseName+bucketStr, baseLabels, leStr, pInfStr)
		if err := c.appender.AppendSample(infLabels, meta, startTimestamp, timestamp, val, exemplars[nextExemplarIdx:]); err != nil {
			return err
		}
	}

	return nil
}

func (c *PrometheusConverter) getPromExemplars(ctx context.Context, exemplars pmetric.ExemplarSlice) ([]exemplar.Exemplar, error) {
	if exemplars.Len() == 0 {
		return nil, nil
	}
	outputExemplars := make([]exemplar.Exemplar, 0, exemplars.Len())
	for i := 0; i < exemplars.Len(); i++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return nil, err
		}

		ex := exemplars.At(i)
		exemplarRunes := 0

		ts := timestamp.FromTime(ex.Timestamp().AsTime())
		newExemplar := exemplar.Exemplar{
			Ts:    ts,
			HasTs: ts != 0,
		}
		c.scratchBuilder.Reset()
		switch ex.ValueType() {
		case pmetric.ExemplarValueTypeInt:
			newExemplar.Value = float64(ex.IntValue())
		case pmetric.ExemplarValueTypeDouble:
			newExemplar.Value = ex.DoubleValue()
		default:
			return nil, fmt.Errorf("unsupported exemplar value type: %v", ex.ValueType())
		}

		if traceID := ex.TraceID(); !traceID.IsEmpty() {
			val := hex.EncodeToString(traceID[:])
			exemplarRunes += utf8.RuneCountInString(traceIDKey) + utf8.RuneCountInString(val)
			c.scratchBuilder.Add(traceIDKey, val)
		}
		if spanID := ex.SpanID(); !spanID.IsEmpty() {
			val := hex.EncodeToString(spanID[:])
			exemplarRunes += utf8.RuneCountInString(spanIDKey) + utf8.RuneCountInString(val)
			c.scratchBuilder.Add(spanIDKey, val)
		}

		attrs := ex.FilteredAttributes()
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
		newExemplar.Labels = c.scratchBuilder.Labels()
		outputExemplars = append(outputExemplars, newExemplar)
	}

	return outputExemplars, nil
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
	settings Settings, scope scope, meta Metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		startTimestamp := convertTimeStamp(pt.StartTimestamp())
		baseLabels, err := c.createAttributes(resource, pt.Attributes(), scope, settings, nil, false, meta)
		if err != nil {
			return err
		}

		baseName := meta.MetricFamilyName

		// treat sum as a sample in an individual TimeSeries
		val := pt.Sum()
		if pt.Flags().NoRecordedValue() {
			val = math.Float64frombits(value.StaleNaN)
		}
		// sum and count of the summary should append suffix to baseName
		sumlabels := c.addLabels(baseName+sumStr, baseLabels)
		if err := c.appender.AppendSample(sumlabels, meta, startTimestamp, timestamp, val, nil); err != nil {
			return err
		}

		// treat count as a sample in an individual TimeSeries
		val = float64(pt.Count())
		if pt.Flags().NoRecordedValue() {
			val = math.Float64frombits(value.StaleNaN)
		}
		countlabels := c.addLabels(baseName+countStr, baseLabels)
		if err := c.appender.AppendSample(countlabels, meta, startTimestamp, timestamp, val, nil); err != nil {
			return err
		}

		// process each percentile/quantile
		for i := 0; i < pt.QuantileValues().Len(); i++ {
			qt := pt.QuantileValues().At(i)
			val = qt.Value()
			if pt.Flags().NoRecordedValue() {
				val = math.Float64frombits(value.StaleNaN)
			}
			percentileStr := strconv.FormatFloat(qt.Quantile(), 'f', -1, 64)
			qtlabels := c.addLabels(baseName, baseLabels, quantileStr, percentileStr)
			if err := c.appender.AppendSample(qtlabels, meta, startTimestamp, timestamp, val, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// addLabels returns a copy of baseLabels, adding to it the pair model.MetricNameLabel=name.
// If extras are provided, corresponding label pairs are also added to the returned slice.
// If extras is uneven length, the last (unpaired) extra will be ignored.
func (c *PrometheusConverter) addLabels(name string, baseLabels labels.Labels, extras ...string) labels.Labels {
	c.builder.Reset(baseLabels)

	n := len(extras)
	n -= n % 2
	for extrasIdx := 0; extrasIdx < n; extrasIdx += 2 {
		c.builder.Set(extras[extrasIdx], extras[extrasIdx+1])
	}
	c.builder.Set(model.MetricNameLabel, name)
	return c.builder.Labels()
}

// addResourceTargetInfo converts the resource to the target info metric.
func (c *PrometheusConverter) addResourceTargetInfo(resource pcommon.Resource, settings Settings, earliestTimestamp, latestTimestamp time.Time) error {
	if settings.DisableTargetInfo {
		return nil
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
		return nil
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
	meta := Metadata{
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "Target metadata",
		},
		MetricFamilyName: name,
	}
	// TODO: should target info have the __type__ metadata label?
	lbls, err := c.createAttributes(resource, attributes, scope{}, settings, identifyingAttrs, false, Metadata{}, model.MetricNameLabel, name)
	if err != nil {
		return err
	}
	haveIdentifier := false
	lbls.Range(func(l labels.Label) {
		if l.Name == model.JobLabel || l.Name == model.InstanceLabel {
			haveIdentifier = true
		}
	})

	if !haveIdentifier {
		// We need at least one identifying label to generate target_info.
		return nil
	}

	// Generate target_info samples starting at earliestTimestamp and ending at latestTimestamp,
	// with a sample at every interval between them.
	// Use an interval corresponding to half of the lookback delta, to ensure that target_info samples are found
	// for the entirety of the relevant period.
	if settings.LookbackDelta == 0 {
		settings.LookbackDelta = defaultLookbackDelta
	}
	interval := settings.LookbackDelta / 2
	for timestamp := earliestTimestamp; timestamp.Before(latestTimestamp); timestamp = timestamp.Add(interval) {
		if err := c.appender.AppendSample(lbls, meta, 0, timestamp.UnixMilli(), float64(1), nil); err != nil {
			return err
		}
	}
	return c.appender.AppendSample(lbls, meta, 0, latestTimestamp.UnixMilli(), float64(1), nil)
}

// convertTimeStamp converts OTLP timestamp in ns to timestamp in ms.
func convertTimeStamp(timestamp pcommon.Timestamp) int64 {
	return int64(timestamp) / 1_000_000
}
