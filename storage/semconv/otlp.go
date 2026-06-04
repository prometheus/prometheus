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
	"fmt"
	"slices"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"

	"github.com/prometheus/prometheus/model/labels"
)

// otelStrategies defines the OTLP translation strategies to generate variants for.
// These are the strategies that OTLP data may have been written with. The
// identity variant (no translation) is prepended separately by
// generateOTLPVariants, so otlptranslator.NoTranslation is intentionally not
// in this list — it would only produce a duplicate that the wrapper then
// has to dedup.
var otelStrategies = []otlptranslator.TranslationStrategyOption{
	otlptranslator.UnderscoreEscapingWithSuffixes,
	otlptranslator.UnderscoreEscapingWithoutSuffixes,
	otlptranslator.NoUTF8EscapingWithSuffixes,
}

// generateOTLPVariants generates matcher variants for all OTLP translation
// strategies. Takes matchers with OTel semantic names and produces variants
// with Prometheus-translated names. The first variant is always the original
// matchers (identity). Strategies whose translator rejects the input are
// silently skipped so a single bad strategy can't deny the user the rest of
// the fan-out.
func generateOTLPVariants(matchers []*labels.Matcher, meta *metricMeta) [][]*labels.Matcher {
	variants := make([][]*labels.Matcher, 0, len(otelStrategies)+1)
	variants = append(variants, matchers) // Identity variant first.
	for _, strategy := range otelStrategies {
		variant, err := translateMatchers(matchers, meta, strategy)
		if err != nil {
			continue
		}
		variants = append(variants, variant)
	}

	return variants
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

// translateLabelName translates an OTel attribute name to Prometheus label
// format. Label translation depends only on the strategy's escape behaviour,
// so unlike translateMetricName it does not need a metricMeta argument.
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

// parseOTLPStrategy maps a user-facing __otlp_strategy__ value to an OTLP
// translation strategy. The accepted names match the strategy identifiers used
// throughout the OTLP translation docs. ok is false for an unrecognised name.
func parseOTLPStrategy(s string) (strategy otlptranslator.TranslationStrategyOption, ok bool) {
	switch s {
	case "UnderscoreEscapingWithSuffixes":
		return otlptranslator.UnderscoreEscapingWithSuffixes, true
	case "UnderscoreEscapingWithoutSuffixes":
		return otlptranslator.UnderscoreEscapingWithoutSuffixes, true
	case "NoUTF8EscapingWithSuffixes":
		return otlptranslator.NoUTF8EscapingWithSuffixes, true
	case "NoTranslation":
		return otlptranslator.NoTranslation, true
	default:
		var zero otlptranslator.TranslationStrategyOption
		return zero, false
	}
}

// resolveCanonicalMetric maps a metric name written in the given strategy's
// dialect back to the canonical metric declared in sc: it returns the canonical
// metric whose translation under strategy equals name. The queried name must
// therefore be expressed in that dialect — e.g. "test_bytes_total" under
// UnderscoreEscapingWithSuffixes, or the OTel name itself under NoTranslation
// (whose translation is the identity). A name matching no metric's rendering
// yields ok=false. Metrics are scanned in sorted order so a (rare) collision
// resolves deterministically; the scan is bounded by the small embedded
// registry, so it is intentionally left uncached.
func resolveCanonicalMetric(sc semconv, name string, strategy otlptranslator.TranslationStrategyOption) (canonical string, ok bool) {
	names := make([]string, 0, len(sc.metricMetadata))
	for n := range sc.metricMetadata {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		meta := sc.metricMetadata[n]
		if translated, err := translateMetricName(n, &meta, strategy); err == nil && translated == name {
			return n, true
		}
	}
	return "", false
}

// canonicalizeMatchers rewrites matchers expressed in a strategy's dialect back
// to canonical OTel names: the __name__ matcher becomes canonicalMetric, and
// any label matcher whose name is the strategy translation of one of the
// metric's canonical attributes is rewritten to that attribute. Matchers that
// don't correspond to a known attribute are passed through unchanged.
func canonicalizeMatchers(sc semconv, canonicalMetric string, strategy otlptranslator.TranslationStrategyOption, matchers []*labels.Matcher) []*labels.Matcher {
	reverse := make(map[string]string, len(sc.attributesPerMetric[canonicalMetric]))
	for _, attr := range sc.attributesPerMetric[canonicalMetric] {
		if translated, err := translateLabelName(attr, strategy); err == nil {
			reverse[translated] = attr
		}
	}

	out := make([]*labels.Matcher, 0, len(matchers))
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			out = append(out, labels.MustNewMatcher(m.Type, m.Name, canonicalMetric))
			continue
		}
		if attr, found := reverse[m.Name]; found {
			out = append(out, labels.MustNewMatcher(m.Type, attr, m.Value))
			continue
		}
		out = append(out, m)
	}
	return out
}

// dialectMismatchWarning returns a warning when a label matcher is written in a
// different escaping than the declared strategy's dialect but still looks like
// one of the metric's attributes — a likely mistake, since such a matcher is
// applied verbatim and would miss series stored under other escapings. It
// returns "" when every label matcher is either in the declared dialect or not
// a recognized attribute (so genuine non-semconv labels like "instance" don't
// warn). It is a heuristic: a real non-attribute label that coincidentally
// equals an attribute's other escaping would also be flagged.
func dialectMismatchWarning(sc semconv, canonicalMetric string, strategy otlptranslator.TranslationStrategyOption, matchers []*labels.Matcher) string {
	// Label escaping has two modes (escape vs not); the "other" form is the
	// opposite of the declared strategy's mode.
	other := otlptranslator.NoTranslation
	if !strategy.ShouldEscape() {
		other = otlptranslator.UnderscoreEscapingWithSuffixes
	}

	declared := make(map[string]struct{})
	wrongForm := make(map[string]struct{})
	for _, attr := range sc.attributesPerMetric[canonicalMetric] {
		d, err1 := translateLabelName(attr, strategy)
		o, err2 := translateLabelName(attr, other)
		if err1 != nil || err2 != nil {
			continue
		}
		declared[d] = struct{}{}
		if o != d {
			wrongForm[o] = struct{}{}
		}
	}

	var mismatched []string
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			continue
		}
		if _, ok := declared[m.Name]; ok {
			continue
		}
		if _, ok := wrongForm[m.Name]; ok {
			mismatched = append(mismatched, m.Name)
		}
	}
	if len(mismatched) == 0 {
		return ""
	}
	return fmt.Sprintf("label matcher(s) %v are not written in the declared __otlp_strategy__ dialect; they look like this metric's attributes in a different escaping and are matched verbatim, so they may miss some translation eras", mismatched)
}

// forwardTranslateLabels renders canonical series labels into a strategy's
// dialect: the metric name via translateMetricName (using meta for unit/type
// suffixes) and every other label name via translateLabelName. Names that fail
// translation are kept as-is.
func forwardTranslateLabels(canonical labels.Labels, metric string, meta *metricMeta, strategy otlptranslator.TranslationStrategyOption) labels.Labels {
	b := labels.NewScratchBuilder(canonical.Len())
	// Escaping is many-to-one, so two distinct names on the series could
	// translate to the same dialect name; keep the first to avoid emitting an
	// invalid duplicate-name labelset.
	seen := make(map[string]struct{}, canonical.Len())
	add := func(name, value string) {
		if _, dup := seen[name]; dup {
			return
		}
		seen[name] = struct{}{}
		b.Add(name, value)
	}
	canonical.Range(func(l labels.Label) {
		switch l.Name {
		case model.MetricNameLabel:
			if translated, err := translateMetricName(metric, meta, strategy); err == nil {
				add(l.Name, translated)
			} else {
				add(l.Name, l.Value)
			}
		default:
			if translated, err := translateLabelName(l.Name, strategy); err == nil {
				add(translated, l.Value)
			} else {
				add(l.Name, l.Value)
			}
		}
	})
	b.Sort()
	return b.Labels()
}
