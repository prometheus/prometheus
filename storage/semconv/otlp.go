// Copyright 2025 The Prometheus Authors
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
	"fmt"
	"path"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	otlpSchemaBase          = "https://prometheus.io/schema/otlp"
	otlpVersionUntranslated = "untranslated"
)

// otelStrategies defines the translation strategies to generate variants for.
// These are the strategies that OTLP data may have been written with.
var otelStrategies = []otlptranslator.TranslationStrategyOption{
	otlptranslator.UnderscoreEscapingWithSuffixes,
	otlptranslator.UnderscoreEscapingWithoutSuffixes,
	otlptranslator.NoUTF8EscapingWithSuffixes,
}

// isOTLPSchema checks if the schemaURL is the intrinsic OTLP schema.
func isOTLPSchema(schemaURL string) bool {
	return strings.HasPrefix(schemaURL, otlpSchemaBase+"/")
}

// findOTLPMatcherVariants generates matcher variants for all OTLP translation strategies.
// This is called when the schema URL is https://prometheus.io/schema/otlp/untranslated.
func findOTLPMatcherVariants(schemaURL string, originalMatchers []*labels.Matcher) ([][]*labels.Matcher, queryContext, error) {
	version := path.Base(schemaURL)
	if version != otlpVersionUntranslated {
		return nil, queryContext{}, fmt.Errorf("unsupported OTLP schema version %q, only %q is supported", version, otlpVersionUntranslated)
	}

	matchers, err := newMatcherBuilder(originalMatchers)
	if err != nil {
		return nil, queryContext{}, err
	}

	variants, err := generateOTLPVariants(originalMatchers, otlptranslator.Metric{
		Name: matchers.metadata.Name,
		Unit: matchers.metadata.Unit,
		Type: promTypeToOTelType(matchers.metadata.Type),
	})
	// Mark that we're doing OTLP transformation.
	// isOTLP indicates that matched series won't have __schema_url__ stored.
	return variants, queryContext{
		changes: []change{{}},
		isOTLP:  true,
	}, err
}

// commonOTLPUnits contains the most common OTLP units that result in suffixes.
// When unit is unknown, we generate variants for these to find metrics.
var commonOTLPUnits = []string{
	"",   // No unit suffix
	"s",  // _seconds
	"ms", // _milliseconds
	"By", // _bytes
	"1",  // _ratio (for gauges)
}

// generateOTLPVariants generates matcher variants for all OTLP translation strategies.
func generateOTLPVariants(matchers []*labels.Matcher, metric otlptranslator.Metric) ([][]*labels.Matcher, error) {
	var variants [][]*labels.Matcher

	// When type is unknown and strategy adds suffixes, we need to generate
	// variants for all possible type suffixes (_total, _ratio, etc.).
	// This ensures we find metrics regardless of their actual type.
	typesToTry := []otlptranslator.MetricType{metric.Type}
	if metric.Type == otlptranslator.MetricTypeUnknown {
		typesToTry = []otlptranslator.MetricType{
			otlptranslator.MetricTypeUnknown,          // No suffix
			otlptranslator.MetricTypeMonotonicCounter, // _total suffix
			otlptranslator.MetricTypeGauge,            // _ratio suffix (for unit "1")
		}
	}

	// When unit is unknown and strategy adds suffixes, we need to generate
	// variants for common unit suffixes (_seconds, _bytes, etc.).
	unitsToTry := []string{metric.Unit}
	if metric.Unit == "" {
		unitsToTry = commonOTLPUnits
	}

	for _, strategy := range otelStrategies {
		labelNamer := otlptranslator.LabelNamer{
			UTF8Allowed:                 !strategy.ShouldEscape(),
			UnderscoreLabelSanitization: strategy.ShouldEscape(),
		}

		for _, metricType := range typesToTry {
			// For strategies without suffixes, only need one variant per strategy.
			if !strategy.ShouldAddSuffixes() && metricType != metric.Type {
				continue
			}

			for _, unit := range unitsToTry {
				// For strategies without suffixes, only need one variant per strategy.
				if !strategy.ShouldAddSuffixes() && unit != metric.Unit {
					continue
				}

				namer := otlptranslator.NewMetricNamer("", strategy)
				translatedName, err := namer.Build(otlptranslator.Metric{
					Name: metric.Name,
					Unit: unit,
					Type: metricType,
				})
				if err != nil {
					continue // Skip invalid translations
				}

				variant := make([]*labels.Matcher, 0, len(matchers))
				for _, m := range matchers {
					switch m.Name {
					case model.MetricNameLabel:
						variant = append(variant, labels.MustNewMatcher(m.Type, m.Name, translatedName))
					case schemaURLLabel:
						// Skip __schema_url__ - we're querying across all variants.
					default:
						newName, err := labelNamer.Build(m.Name)
						if err != nil {
							variant = append(variant, m)
						} else {
							variant = append(variant, labels.MustNewMatcher(m.Type, newName, m.Value))
						}
					}
				}
				variants = append(variants, variant)
			}
		}
	}

	return variants, nil
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
