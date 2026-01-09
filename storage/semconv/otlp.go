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

package semconv

import (
	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"

	"github.com/prometheus/prometheus/model/labels"
)

// otelStrategies defines the OTLP translation strategies to generate variants for.
// These are the strategies that OTLP data may have been written with.
var otelStrategies = []otlptranslator.TranslationStrategyOption{
	otlptranslator.UnderscoreEscapingWithSuffixes,
	otlptranslator.UnderscoreEscapingWithoutSuffixes,
	otlptranslator.NoUTF8EscapingWithSuffixes,
	otlptranslator.NoTranslation,
}

// generateOTLPVariants generates matcher variants for all OTLP translation strategies.
// Takes matchers with OTel semantic names and produces variants with Prometheus-translated names.
// The first variant is always the original matchers (identity).
func generateOTLPVariants(matchers []*labels.Matcher, meta *metricMeta) ([][]*labels.Matcher, error) {
	variants := make([][]*labels.Matcher, 0, len(otelStrategies)+1)
	variants = append(variants, matchers) // Identity variant first.

	for _, strategy := range otelStrategies {
		variant, err := translateMatchers(matchers, meta, strategy)
		if err != nil {
			return nil, err
		}
		variants = append(variants, variant)
	}

	return variants, nil
}

// translateMatchers translates all matchers using the given OTLP strategy.
func translateMatchers(matchers []*labels.Matcher, meta *metricMeta, strategy otlptranslator.TranslationStrategyOption) ([]*labels.Matcher, error) {
	result := make([]*labels.Matcher, 0, len(matchers))

	for _, m := range matchers {
		switch m.Name {
		case model.MetricNameLabel:
			translatedName, err := translateMetricName(m.Value, meta, strategy)
			if err != nil {
				return nil, err
			}
			result = append(result, labels.MustNewMatcher(m.Type, m.Name, translatedName))
		case semconvURLLabel, schemaURLLabel:
			// Skip schema/semconv labels.
		default:
			translatedLabel, err := translateLabelName(m.Name, strategy)
			if err != nil {
				return nil, err
			}
			result = append(result, labels.MustNewMatcher(m.Type, translatedLabel, m.Value))
		}
	}

	return result, nil
}

// translateMetricName translates an OTel metric name to Prometheus format using the given strategy.
func translateMetricName(name string, meta *metricMeta, strategy otlptranslator.TranslationStrategyOption) (string, error) {
	metric := otlptranslator.Metric{
		Name: name,
		Unit: "",
		Type: otlptranslator.MetricTypeUnknown,
	}
	if meta != nil {
		metric.Unit = meta.Unit
		metric.Type = promTypeToOTelType(meta.Type)
	}

	namer := otlptranslator.NewMetricNamer("", strategy)
	return namer.Build(metric)
}

// translateLabelName translates an OTel attribute name to Prometheus label format.
func translateLabelName(name string, strategy otlptranslator.TranslationStrategyOption) (string, error) {
	namer := otlptranslator.LabelNamer{
		UTF8Allowed:                 !strategy.ShouldEscape(),
		UnderscoreLabelSanitization: strategy.ShouldEscape(),
	}
	return namer.Build(name)
}

// promTypeToOTelType converts a Prometheus metric type to an OTel metric type.
func promTypeToOTelType(t model.MetricType) otlptranslator.MetricType {
	switch t {
	case model.MetricTypeCounter:
		return otlptranslator.MetricTypeMonotonicCounter
	case model.MetricTypeGauge:
		return otlptranslator.MetricTypeGauge
	case model.MetricTypeHistogram:
		return otlptranslator.MetricTypeHistogram
	case model.MetricTypeSummary:
		return otlptranslator.MetricTypeSummary
	default:
		return otlptranslator.MetricTypeUnknown
	}
}
