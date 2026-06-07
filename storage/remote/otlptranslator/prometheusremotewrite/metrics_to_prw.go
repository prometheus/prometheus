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
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
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

// cachedResourceLabels holds precomputed labels constant for all datapoints in a ResourceMetrics.
// These are computed once per ResourceMetrics boundary and reused for all datapoints.
type cachedResourceLabels struct {
	jobLabel       string        // from service.name + service.namespace.
	instanceLabel  string        // from service.instance.id.
	promotedLabels labels.Labels // promoted resource attributes.
	externalLabels map[string]string
}

// cachedScopeLabels holds precomputed scope metadata labels.
// These are computed once per ScopeMetrics boundary and reused for all datapoints.
type cachedScopeLabels struct {
	scopeName      string
	scopeVersion   string
	scopeSchemaURL string
	scopeAttrs     labels.Labels // otel_scope_* labels.
}

// PrometheusConverter converts from OTel write format to Prometheus remote write format.
type PrometheusConverter struct {
	everyN         everyNTimes
	scratchBuilder labels.ScratchBuilder
	builder        *labels.Builder
	appender       storage.AppenderV2
	// seenTargetInfo tracks target_info samples within a batch to prevent duplicates.
	seenTargetInfo map[targetInfoKey]struct{}

	// Label caching for optimization - computed once per resource/scope boundary.
	resourceLabels *cachedResourceLabels
	scopeLabels    *cachedScopeLabels
	labelNamer     otlptranslator.LabelNamer

	// sanitizedLabels caches the results of label name sanitization within a request.
	// This avoids repeated string allocations for the same label names.
	sanitizedLabels map[string]string
}

// targetInfoKey uniquely identifies a target_info sample by its labelset and timestamp.
type targetInfoKey struct {
	labelsHash uint64
	timestamp  int64
}

func NewPrometheusConverter(appender storage.AppenderV2) *PrometheusConverter {
	return &PrometheusConverter{
		scratchBuilder:  labels.NewScratchBuilder(0),
		builder:         labels.NewBuilder(labels.EmptyLabels()),
		appender:        appender,
		sanitizedLabels: make(map[string]string, 64), // Pre-size for typical label count.
	}
}

// buildLabelName returns a sanitized label name, using the cache to avoid repeated allocations.
func (c *PrometheusConverter) buildLabelName(label string) (string, error) {
	if sanitized, ok := c.sanitizedLabels[label]; ok {
		return sanitized, nil
	}

	sanitized, err := c.labelNamer.Build(label)
	if err != nil {
		return "", err
	}
	c.sanitizedLabels[label] = sanitized
	return sanitized, nil
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

// FromMetrics appends pmetric.Metrics to storage.AppenderV2.
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

	for i := range resourceMetricsSlice.Len() {
		resourceMetrics := resourceMetricsSlice.At(i)
		resource := resourceMetrics.Resource()
		scopeMetricsSlice := resourceMetrics.ScopeMetrics()
		if err := c.setResourceContext(resource, settings); err != nil {
			errs = errors.Join(errs, err)
			continue
		}

		// keep track of the earliest and latest timestamp in the ResourceMetrics for
		// use with the "target" info metric
		earliestTimestamp := pcommon.Timestamp(math.MaxUint64)
		latestTimestamp := pcommon.Timestamp(0)
		for j := range scopeMetricsSlice.Len() {
			scopeMetrics := scopeMetricsSlice.At(j)
			scope := newScopeFromScopeMetrics(scopeMetrics)
			if err := c.setScopeContext(scope, settings); err != nil {
				errs = errors.Join(errs, err)
				continue
			}

			metricSlice := scopeMetrics.Metrics()

			// TODO: decide if instrumentation library information should be exported as labels
			for k := 0; k < metricSlice.Len(); k++ {
				if err := c.everyN.checkContext(ctx); err != nil {
					errs = errors.Join(errs, err)
					return annots, errs
				}

				metric := metricSlice.At(k)
				earliestTimestamp, latestTimestamp = findMinAndMaxTimestamps(metric, earliestTimestamp, latestTimestamp)
				temporality, hasTemporality, err := aggregationTemporality(metric)
				if err != nil {
					errs = errors.Join(errs, err)
					continue
				}

				if hasTemporality &&
					// Cumulative temporality is always valid.
					// Delta temporality is also valid if AllowDeltaTemporality is true.
					// All other temporality values are invalid.
					//nolint:staticcheck // QF1001 Applying De Morgan’s law would make the conditions harder to read.
					!(temporality == pmetric.AggregationTemporalityCumulative ||
						(settings.AllowDeltaTemporality && temporality == pmetric.AggregationTemporalityDelta)) {
					errs = errors.Join(errs, fmt.Errorf("invalid temporality and type combination for metric %q", metric.Name()))
					continue
				}

				promName, err := namer.Build(TranslatorMetricFromOtelMetric(metric))
				if err != nil {
					errs = errors.Join(errs, err)
					continue
				}

				appOpts := storage.AOptions{
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
						errs = errors.Join(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addGaugeNumberDataPoints(ctx, dataPoints, settings, appOpts); err != nil {
						errs = errors.Join(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return annots, errs
						}
					}
				case pmetric.MetricTypeSum:
					dataPoints := metric.Sum().DataPoints()
					if dataPoints.Len() == 0 {
						errs = errors.Join(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addSumNumberDataPoints(ctx, dataPoints, settings, appOpts); err != nil {
						errs = errors.Join(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return annots, errs
						}
					}
				case pmetric.MetricTypeHistogram:
					dataPoints := metric.Histogram().DataPoints()
					if dataPoints.Len() == 0 {
						errs = errors.Join(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if settings.ConvertHistogramsToNHCB {
						ws, err := c.addCustomBucketsHistogramDataPoints(
							ctx, dataPoints, settings, temporality, appOpts,
						)
						annots.Merge(ws)
						if err != nil {
							errs = errors.Join(errs, err)
							if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
								return annots, errs
							}
						}
					} else {
						if err := c.addHistogramDataPoints(ctx, dataPoints, settings, appOpts); err != nil {
							errs = errors.Join(errs, err)
							if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
								return annots, errs
							}
						}
					}
				case pmetric.MetricTypeExponentialHistogram:
					dataPoints := metric.ExponentialHistogram().DataPoints()
					if dataPoints.Len() == 0 {
						errs = errors.Join(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					ws, err := c.addExponentialHistogramDataPoints(
						ctx,
						dataPoints,
						settings,
						temporality,
						appOpts,
					)
					annots.Merge(ws)
					if err != nil {
						errs = errors.Join(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return annots, errs
						}
					}
				case pmetric.MetricTypeSummary:
					dataPoints := metric.Summary().DataPoints()
					if dataPoints.Len() == 0 {
						errs = errors.Join(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addSummaryDataPoints(ctx, dataPoints, settings, appOpts); err != nil {
						errs = errors.Join(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return annots, errs
						}
					}
				default:
					errs = errors.Join(errs, errors.New("unsupported metric type"))
				}
			}
		}
		if earliestTimestamp < pcommon.Timestamp(math.MaxUint64) {
			// We have at least one metric sample for this resource.
			// Generate a corresponding target_info series.
			if err := c.addResourceTargetInfo(resource, settings, earliestTimestamp.AsTime(), latestTimestamp.AsTime()); err != nil {
				errs = errors.Join(errs, err)
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

// LabelNameBuilder is a function that builds/sanitizes label names.
type LabelNameBuilder func(string) (string, error)

// addPromotedAttributes adds labels for promoted resourceAttributes to the builder.
func (s *PromoteResourceAttributes) addPromotedAttributes(builder *labels.Builder, resourceAttributes pcommon.Map, buildLabelName LabelNameBuilder) error {
	if s == nil {
		return nil
	}

	if s.promoteAll {
		var err error
		resourceAttributes.Range(func(name string, value pcommon.Value) bool {
			if _, exists := s.attrs[name]; !exists {
				var normalized string
				normalized, err = buildLabelName(name)
				if err != nil {
					return false
				}
				builder.Set(normalized, value.AsString())
			}
			return true
		})
		return err
	}
	var err error
	resourceAttributes.Range(func(name string, value pcommon.Value) bool {
		if _, exists := s.attrs[name]; exists {
			var normalized string
			normalized, err = buildLabelName(name)
			if err != nil {
				return false
			}
			builder.Set(normalized, value.AsString())
		}
		return true
	})
	return err
}

// setResourceContext precomputes and caches resource-level labels.
// Called once per ResourceMetrics boundary, before processing any datapoints.
// If an error is returned, resource level cache is reset.
func (c *PrometheusConverter) setResourceContext(resource pcommon.Resource, settings Settings) error {
	resourceAttrs := resource.Attributes()
	c.resourceLabels = &cachedResourceLabels{
		externalLabels: settings.ExternalLabels,
	}

	c.labelNamer = otlptranslator.LabelNamer{
		UTF8Allowed:                 settings.AllowUTF8,
		UnderscoreLabelSanitization: settings.LabelNameUnderscoreSanitization,
		PreserveMultipleUnderscores: settings.LabelNamePreserveMultipleUnderscores,
	}

	if serviceName, ok := resourceAttrs.Get(string(semconv.ServiceNameKey)); ok {
		val := serviceName.AsString()
		if serviceNamespace, ok := resourceAttrs.Get(string(semconv.ServiceNamespaceKey)); ok {
			val = serviceNamespace.AsString() + "/" + val
		}
		c.resourceLabels.jobLabel = val
	}

	if instance, ok := resourceAttrs.Get(string(semconv.ServiceInstanceIDKey)); ok {
		c.resourceLabels.instanceLabel = instance.AsString()
	}

	if settings.PromoteResourceAttributes != nil {
		c.builder.Reset(labels.EmptyLabels())
		if err := settings.PromoteResourceAttributes.addPromotedAttributes(c.builder, resourceAttrs, c.buildLabelName); err != nil {
			c.clearResourceContext()
			return err
		}
		c.resourceLabels.promotedLabels = c.builder.Labels()
	}
	return nil
}

// setScopeContext precomputes and caches scope-level labels.
// Called once per ScopeMetrics boundary, before processing any metrics.
// If an error is returned, scope level cache is reset.
func (c *PrometheusConverter) setScopeContext(scope scope, settings Settings) error {
	if !settings.PromoteScopeMetadata || scope.name == "" {
		c.scopeLabels = nil
		return nil
	}

	c.scopeLabels = &cachedScopeLabels{
		scopeName:      scope.name,
		scopeVersion:   scope.version,
		scopeSchemaURL: scope.schemaURL,
	}
	c.builder.Reset(labels.EmptyLabels())
	var err error
	scope.attributes.Range(func(k string, v pcommon.Value) bool {
		var name string
		name, err = c.buildLabelName("otel_scope_" + k)
		if err != nil {
			return false
		}
		c.builder.Set(name, v.AsString())
		return true
	})
	if err != nil {
		c.scopeLabels = nil
		return err
	}

	c.scopeLabels.scopeAttrs = c.builder.Labels()
	return nil
}

// clearResourceContext clears cached labels between ResourceMetrics.
func (c *PrometheusConverter) clearResourceContext() {
	c.resourceLabels = nil
	c.scopeLabels = nil
}
