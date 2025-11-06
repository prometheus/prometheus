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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheusremotewrite/metrics_to_prw.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/multierr"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/util/annotations"
)

type PromoteResourceAttributes struct {
	promoteAll bool
	attrs      map[string]struct{}
}

type Settings struct {
	Namespace                         string
	ExternalLabels                    map[string]string
	DisableTargetInfo                 bool
	AddMetricSuffixes                 bool
	AllowUTF8                         bool
	PromoteResourceAttributes         *PromoteResourceAttributes
	KeepIdentifyingResourceAttributes bool
	ConvertHistogramsToNHCB           bool
	AllowDeltaTemporality             bool
	// LookbackDelta is the PromQL engine lookback delta.
	LookbackDelta time.Duration
	// PromoteScopeMetadata controls whether to promote OTel scope metadata to metric labels.
	PromoteScopeMetadata    bool
	EnableTypeAndUnitLabels bool
	// LabelNameUnderscoreSanitization controls whether to enable prepending of 'key' to labels
	// starting with '_'. Reserved labels starting with `__` are not modified.
	LabelNameUnderscoreSanitization bool
	// LabelNamePreserveMultipleUnderscores enables preserving of multiple
	// consecutive underscores in label names when AllowUTF8 is false.
	LabelNamePreserveMultipleUnderscores bool
}

// PrometheusConverter converts from OTel write format to Prometheus remote write format.
type PrometheusConverter struct {
	everyN         everyNTimes
	scratchBuilder labels.ScratchBuilder
	builder        *labels.Builder
	appender       CombinedAppender
	// seenTargetInfo tracks target_info samples within a batch to prevent duplicates.
	seenTargetInfo map[targetInfoKey]struct{}
}

// targetInfoKey uniquely identifies a target_info sample by its labelset and timestamp.
type targetInfoKey struct {
	labelsHash uint64
	timestamp  int64
}

func NewPrometheusConverter(appender CombinedAppender) *PrometheusConverter {
	return &PrometheusConverter{
		scratchBuilder: labels.NewScratchBuilder(0),
		builder:        labels.NewBuilder(labels.EmptyLabels()),
		appender:       appender,
	}
}

func TranslatorMetricFromOtelMetric(metric pmetric.Metric) otlptranslator.Metric {
	m := otlptranslator.Metric{
		Name: metric.Name(),
		Unit: metric.Unit(),
		Type: otlptranslator.MetricTypeUnknown,
	}
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		m.Type = otlptranslator.MetricTypeGauge
	case pmetric.MetricTypeSum:
		if metric.Sum().IsMonotonic() {
			m.Type = otlptranslator.MetricTypeMonotonicCounter
		} else {
			m.Type = otlptranslator.MetricTypeNonMonotonicCounter
		}
	case pmetric.MetricTypeSummary:
		m.Type = otlptranslator.MetricTypeSummary
	case pmetric.MetricTypeHistogram:
		m.Type = otlptranslator.MetricTypeHistogram
	case pmetric.MetricTypeExponentialHistogram:
		m.Type = otlptranslator.MetricTypeExponentialHistogram
	}
	return m
}

type scope struct {
	name       string
	version    string
	schemaURL  string
	attributes pcommon.Map
}

func newScopeFromScopeMetrics(scopeMetrics pmetric.ScopeMetrics) scope {
	s := scopeMetrics.Scope()
	return scope{
		name:       s.Name(),
		version:    s.Version(),
		schemaURL:  scopeMetrics.SchemaUrl(),
		attributes: s.Attributes(),
	}
}

// FromMetrics converts pmetric.Metrics to Prometheus remote write format.
func (c *PrometheusConverter) FromMetrics(ctx context.Context, md pmetric.Metrics, settings Settings) (annots annotations.Annotations, errs error) {
	namer := otlptranslator.MetricNamer{
		Namespace:          settings.Namespace,
		WithMetricSuffixes: settings.AddMetricSuffixes,
		UTF8Allowed:        settings.AllowUTF8,
	}
	unitNamer := otlptranslator.UnitNamer{}
	c.everyN = everyNTimes{n: 128}
	c.seenTargetInfo = make(map[targetInfoKey]struct{})
	resourceMetricsSlice := md.ResourceMetrics()

	numMetrics := 0
	for i := 0; i < resourceMetricsSlice.Len(); i++ {
		scopeMetricsSlice := resourceMetricsSlice.At(i).ScopeMetrics()
		for j := 0; j < scopeMetricsSlice.Len(); j++ {
			numMetrics += scopeMetricsSlice.At(j).Metrics().Len()
		}
	}

	for i := 0; i < resourceMetricsSlice.Len(); i++ {
		resourceMetrics := resourceMetricsSlice.At(i)
		resource := resourceMetrics.Resource()
		scopeMetricsSlice := resourceMetrics.ScopeMetrics()
		// keep track of the earliest and latest timestamp in the ResourceMetrics for
		// use with the "target" info metric
		earliestTimestamp := pcommon.Timestamp(math.MaxUint64)
		latestTimestamp := pcommon.Timestamp(0)
		for j := 0; j < scopeMetricsSlice.Len(); j++ {
			scopeMetrics := scopeMetricsSlice.At(j)
			scope := newScopeFromScopeMetrics(scopeMetrics)
			metricSlice := scopeMetrics.Metrics()

			// TODO: decide if instrumentation library information should be exported as labels
			for k := 0; k < metricSlice.Len(); k++ {
				if err := c.everyN.checkContext(ctx); err != nil {
					errs = multierr.Append(errs, err)
					return annots, errs
				}

				metric := metricSlice.At(k)
				earliestTimestamp, latestTimestamp = findMinAndMaxTimestamps(metric, earliestTimestamp, latestTimestamp)
				temporality, hasTemporality, err := aggregationTemporality(metric)
				if err != nil {
					errs = multierr.Append(errs, err)
					continue
				}

				if hasTemporality &&
					// Cumulative temporality is always valid.
					// Delta temporality is also valid if AllowDeltaTemporality is true.
					// All other temporality values are invalid.
					//nolint:staticcheck // QF1001 Applying De Morgan’s law would make the conditions harder to read.
					!(temporality == pmetric.AggregationTemporalityCumulative ||
						(settings.AllowDeltaTemporality && temporality == pmetric.AggregationTemporalityDelta)) {
					errs = multierr.Append(errs, fmt.Errorf("invalid temporality and type combination for metric %q", metric.Name()))
					continue
				}

				promName, err := namer.Build(TranslatorMetricFromOtelMetric(metric))
				if err != nil {
					errs = multierr.Append(errs, err)
					continue
				}
				meta := Metadata{
					Metadata: metadata.Metadata{
						Type: otelMetricTypeToPromMetricType(metric),
						Unit: unitNamer.Build(metric.Unit()),
						Help: metric.Description(),
					},
					MetricFamilyName: promName,
				}

				// handle individual metrics based on type
				//exhaustive:enforce
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					dataPoints := metric.Gauge().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addGaugeNumberDataPoints(ctx, dataPoints, resource, settings, scope, meta); err != nil {
						errs = multierr.Append(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return annots, errs
						}
					}
				case pmetric.MetricTypeSum:
					dataPoints := metric.Sum().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addSumNumberDataPoints(ctx, dataPoints, resource, settings, scope, meta); err != nil {
						errs = multierr.Append(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return annots, errs
						}
					}
				case pmetric.MetricTypeHistogram:
					dataPoints := metric.Histogram().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if settings.ConvertHistogramsToNHCB {
						ws, err := c.addCustomBucketsHistogramDataPoints(
							ctx, dataPoints, resource, settings, temporality, scope, meta,
						)
						annots.Merge(ws)
						if err != nil {
							errs = multierr.Append(errs, err)
							if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
								return annots, errs
							}
						}
					} else {
						if err := c.addHistogramDataPoints(ctx, dataPoints, resource, settings, scope, meta); err != nil {
							errs = multierr.Append(errs, err)
							if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
								return annots, errs
							}
						}
					}
				case pmetric.MetricTypeExponentialHistogram:
					dataPoints := metric.ExponentialHistogram().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					ws, err := c.addExponentialHistogramDataPoints(
						ctx,
						dataPoints,
						resource,
						settings,
						temporality,
						scope,
						meta,
					)
					annots.Merge(ws)
					if err != nil {
						errs = multierr.Append(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return annots, errs
						}
					}
				case pmetric.MetricTypeSummary:
					dataPoints := metric.Summary().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addSummaryDataPoints(ctx, dataPoints, resource, settings, scope, meta); err != nil {
						errs = multierr.Append(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return annots, errs
						}
					}
				default:
					errs = multierr.Append(errs, errors.New("unsupported metric type"))
				}
			}
		}
		if earliestTimestamp < pcommon.Timestamp(math.MaxUint64) {
			// We have at least one metric sample for this resource.
			// Generate a corresponding target_info series.
			if err := c.addResourceTargetInfo(resource, settings, earliestTimestamp.AsTime(), latestTimestamp.AsTime()); err != nil {
				errs = multierr.Append(errs, err)
			}
		}
	}

	return annots, errs
}

func NewPromoteResourceAttributes(otlpCfg config.OTLPConfig) *PromoteResourceAttributes {
	attrs := otlpCfg.PromoteResourceAttributes
	if otlpCfg.PromoteAllResourceAttributes {
		attrs = otlpCfg.IgnoreResourceAttributes
	}
	attrsMap := make(map[string]struct{}, len(attrs))
	for _, s := range attrs {
		attrsMap[s] = struct{}{}
	}
	return &PromoteResourceAttributes{
		promoteAll: otlpCfg.PromoteAllResourceAttributes,
		attrs:      attrsMap,
	}
}

// addPromotedAttributes adds labels for promoted resourceAttributes to the builder.
func (s *PromoteResourceAttributes) addPromotedAttributes(builder *labels.Builder, resourceAttributes pcommon.Map, labelNamer otlptranslator.LabelNamer) error {
	if s == nil {
		return nil
	}

	if s.promoteAll {
		var err error
		resourceAttributes.Range(func(name string, value pcommon.Value) bool {
			if _, exists := s.attrs[name]; !exists {
				var normalized string
				normalized, err = labelNamer.Build(name)
				if err != nil {
					return false
				}
				if builder.Get(normalized) == "" {
					builder.Set(normalized, value.AsString())
				}
			}
			return true
		})
		return err
	}
	var err error
	resourceAttributes.Range(func(name string, value pcommon.Value) bool {
		if _, exists := s.attrs[name]; exists {
			var normalized string
			normalized, err = labelNamer.Build(name)
			if err != nil {
				return false
			}
			if builder.Get(normalized) == "" {
				builder.Set(normalized, value.AsString())
			}
		}
		return true
	})
	return err
}
