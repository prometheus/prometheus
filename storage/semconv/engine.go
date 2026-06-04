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
	"errors"
	"iter"
	"slices"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"

	"github.com/prometheus/prometheus/model/labels"
)

// errStrategyUnresolved is returned by findMatcherVariants when an
// __otlp_strategy__ is supplied but the metric name does not resolve to any
// metric in the referenced semconv registry under that strategy.
var errStrategyUnresolved = errors.New("__otlp_strategy__ set but no matching metric found in the semconv registry")

type schemaEngine struct {
	otelSchemaCache *staticCache[otelSchema]
	semconvCache    *staticCache[semconv]
}

func newSchemaEngine() *schemaEngine {
	return &schemaEngine{
		otelSchemaCache: newStaticCache[otelSchema](),
		semconvCache:    newStaticCache[semconv](),
	}
}

func extractMetricName(matchers []*labels.Matcher) (string, error) {
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			if m.Type != labels.MatchEqual {
				return "", errors.New("__name__ matcher must be equal")
			}
			return m.Value, nil
		}
	}
	return "", nil
}

// mergeAttributes merges two attribute slices, removing duplicates from b that exist in a.
func mergeAttributes(a, b []string) []string {
	if len(b) == 0 {
		return a
	}
	if len(a) == 0 {
		return b
	}

	seen := make(map[string]struct{}, len(a))
	for _, attr := range a {
		seen[attr] = struct{}{}
	}

	result := make([]string, len(a), len(a)+len(b))
	copy(result, a)
	for _, attr := range b {
		if _, exists := seen[attr]; !exists {
			result = append(result, attr)
		}
	}
	return result
}

// findVersionAnchorIndex returns the index of the largest version <= targetVersion.
// The versions slice must be sorted in ascending semver order.
func findVersionAnchorIndex(versions []versionRenames, targetVersion string) int {
	target := strings.TrimPrefix(targetVersion, "v")
	anchorIdx := 0
	for i, v := range versions {
		if compareSemver(v.version, target) > 0 {
			break
		}
		anchorIdx = i
	}
	return anchorIdx
}

// generateMatcherVariants generates matcher sets for schema version renames,
// anchored at the specified version.
// For each version, applies both metric and attribute renames together.
// Walks backward through versions <= version to find older name variants,
// and forward through versions > version to find newer name variants.
func generateMatcherVariants(version string, schema *otelSchema, matchers []*labels.Matcher) [][]*labels.Matcher {
	if len(schema.versionRenames) == 0 {
		return [][]*labels.Matcher{matchers}
	}

	variants := [][]*labels.Matcher{matchers}
	seen := map[string]struct{}{matcherKey(matchers): {}}
	anchorIdx := findVersionAnchorIndex(schema.versionRenames, version)

	// Backward for older names.
	variants = walkVersions(schema.versionRenames[:anchorIdx+1], matchers, seen, variants, true)

	// Forward for newer names.
	variants = walkVersions(schema.versionRenames[anchorIdx+1:], matchers, seen, variants, false)

	return variants
}

// walkVersions walks through versions applying renames, chaining results until no new variants.
// If reverse is false, walks oldest→newest; if true, walks newest→oldest.
func walkVersions(
	versions []versionRenames,
	matchers []*labels.Matcher,
	seen map[string]struct{},
	result [][]*labels.Matcher,
	reverse bool,
) [][]*labels.Matcher {
	current := matchers
	for {
		found := false
		var versionsIter iter.Seq2[int, versionRenames]
		if reverse {
			versionsIter = slices.Backward(versions)
		} else {
			versionsIter = slices.All(versions)
		}

		for _, v := range versionsIter {
			transformed := applyVersionRenames(current, v)
			if transformed == nil {
				continue
			}

			key := matcherKey(transformed)
			if _, exists := seen[key]; exists {
				continue
			}

			seen[key] = struct{}{}
			result = append(result, transformed)
			current = transformed
			found = true
			break
		}
		if !found {
			break
		}
	}
	return result
}

// matcherKey generates a string key for a matcher set to detect duplicates.
func matcherKey(matchers []*labels.Matcher) string {
	var b strings.Builder
	for i, m := range matchers {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(m.Name)
		b.WriteByte('=')
		b.WriteString(m.Value)
	}
	return b.String()
}

// applyVersionRenames applies a version's metric and attribute renames to matchers.
// Returns nil if no renames apply. Uses lazy allocation to avoid allocating when no changes are made.
func applyVersionRenames(matchers []*labels.Matcher, renames versionRenames) []*labels.Matcher {
	var result []*labels.Matcher
	for i, m := range matchers {
		var newMatcher *labels.Matcher
		if m.Name == model.MetricNameLabel {
			if variant, ok := renames.metrics[m.Value]; ok {
				newMatcher = labels.MustNewMatcher(m.Type, m.Name, variant)
			}
		} else if variant, ok := renames.attributes[m.Name]; ok {
			newMatcher = labels.MustNewMatcher(m.Type, variant, m.Value)
		}
		if newMatcher != nil {
			if result == nil {
				// Lazy allocate and copy preceding unchanged matchers.
				result = make([]*labels.Matcher, len(matchers))
				copy(result[:i], matchers[:i])
			}
			result[i] = newMatcher
		} else if result != nil {
			result[i] = m
		}
	}

	return result
}

type queryContext struct {
	// labelMapping is a mapping to the requested OTel semantic conventions version.
	labelMapping *labelMapping

	// outputStrategy, when non-nil, renders result labels in that OTLP
	// strategy's dialect instead of canonical OTel names. outputMeta carries
	// the canonical metric's unit/type so the metric name can be suffixed.
	outputStrategy *otlptranslator.TranslationStrategyOption
	outputMeta     *metricMeta

	// warning, when non-empty, is an advisory message the wrapper surfaces
	// alongside the (still-produced) results.
	warning string
}

// getSemconv returns the semconv parsed from url, fetching it via the
// embedded registry on a cache miss.
func (e *schemaEngine) getSemconv(url string) (semconv, error) {
	if sc, ok := e.semconvCache.get(url); ok {
		return sc, nil
	}
	sc, err := fetchSemconv(url)
	if err != nil {
		return semconv{}, err
	}
	e.semconvCache.set(url, sc)
	return sc, nil
}

// getOTelSchema returns the OTel schema parsed from url, fetching it via the
// embedded registry on a cache miss.
func (e *schemaEngine) getOTelSchema(url string) (otelSchema, error) {
	if s, ok := e.otelSchemaCache.get(url); ok {
		return s, nil
	}
	s, err := fetchOTelSchema(url)
	if err != nil {
		return otelSchema{}, err
	}
	e.otelSchemaCache.set(url, s)
	return s, nil
}

// findMatcherVariants returns all variants to match for a single schematized metric selection.
// semconvURL points to a semantic conventions file (groups with metric metadata) and is required.
// schemaURL points to an OTel schema file (versions with attribute renames) and is optional.
// Returns variants for all semantic conventions renames and OTLP translation strategies,
// plus a label mapping for transforming results to the current version.
// The returned matchers do not include __semconv_url__ or __schema_url__.
// It returns an error if semconvURL is not provided or if the metric is not found.
func (e *schemaEngine) findMatcherVariants(semconvURL, schemaURL string, strategy *otlptranslator.TranslationStrategyOption, originalMatchers []*labels.Matcher) ([][]*labels.Matcher, queryContext, error) {
	if semconvURL == "" {
		return nil, queryContext{}, errors.New("semconvURL is required")
	}

	// Filter out the wrapper's reserved matchers.
	matchers := stripReservedLabels(originalMatchers)

	// Fetch semantic conventions for metric metadata.
	sc, err := e.getSemconv(semconvURL)
	if err != nil {
		return nil, queryContext{}, err
	}

	metricName, err := extractMetricName(matchers)
	if err != nil {
		return nil, queryContext{}, err
	}
	if metricName == "" {
		// Without an explicit __name__ matcher we have no anchor to resolve
		// attribute or metric renames against; fall through to the underlying
		// querier without applying any rewrite.
		return [][]*labels.Matcher{matchers}, queryContext{}, nil
	}

	// When an OTLP strategy is declared, the query is expressed in that
	// strategy's dialect. Reverse-resolve the metric name and label matchers to
	// canonical OTel names so the schema/strategy fan-out below anchors on the
	// canonical metric. A name that matches no registry metric is reported so
	// the wrapper can warn and pass through.
	var outputMeta *metricMeta
	var dialectWarning string
	if strategy != nil {
		canonical, ok := resolveCanonicalMetric(sc, metricName, *strategy)
		if !ok {
			return nil, queryContext{}, errStrategyUnresolved
		}
		// Detect mismatched-dialect label matchers before canonicalizing, while
		// the names are still as the caller wrote them.
		dialectWarning = dialectMismatchWarning(sc, canonical, *strategy, matchers)
		matchers = canonicalizeMatchers(sc, canonical, *strategy, matchers)
		metricName = canonical
		meta := sc.metricMetadata[canonical]
		outputMeta = &meta
	}

	// Stage 1: Generate schema version variants (if schemaURL provided).
	schemaVariants := [][]*labels.Matcher{matchers}
	var attributes []string
	var schema otelSchema
	haveSchema := false
	if schemaURL != "" {
		schema, err = e.getOTelSchema(schemaURL)
		if err != nil {
			return nil, queryContext{}, err
		}
		haveSchema = true
		schemaVariants = generateMatcherVariants(sc.version, &schema, matchers)
		// Merge semconv attributes with schema attributes (which include renamed versions).
		attributes = mergeAttributes(sc.attributesPerMetric[metricName], schema.getAttributesForMetric(metricName))
	} else {
		attributes = sc.attributesPerMetric[metricName]
	}

	// Build a metric-name → meta lookup that covers every version a schema
	// variant may resolve to, not just the anchor. Without this, OTLP
	// variants for historical metric names would be escaped using the
	// anchor's unit/type, which is incorrect once a metric's unit or
	// instrument changes across schema versions.
	metaLookup := map[string]*metricMeta{}
	for n, m := range sc.metricMetadata {
		metaLookup[n] = &m
	}
	if haveSchema {
		// Walk every version listed in the schema, not just the ones with
		// renames: a baseline version (e.g. 1.0.0) may have no rename
		// changes and therefore not appear in versionRenames, yet still
		// own the historical metric metadata (unit/type) needed for OTLP
		// translation of its metric names. Walk in sorted order so the
		// first-writer-wins selection below is deterministic.
		versions := make([]string, 0, len(schema.Versions))
		for versionStr := range schema.Versions {
			versions = append(versions, versionStr)
		}
		slices.SortFunc(versions, compareSemver)
		for _, versionStr := range versions {
			if versionStr == sc.version {
				continue
			}
			verSC, err := e.getSemconv("registry/" + versionStr)
			if err != nil {
				// Best-effort: missing version-specific semconv files just
				// mean we fall back to the anchor's meta for those variants.
				continue
			}
			for n, m := range verSC.metricMetadata {
				if _, exists := metaLookup[n]; exists {
					continue
				}
				metaLookup[n] = &m
			}
		}
	}

	// Stage 2: Generate OTLP translation variants for each schema variant,
	// using each variant's own metric metadata where available. OTLP-strategy
	// fan-out only runs when an __otlp_strategy__ was supplied; otherwise the
	// schema variants are queried under their raw OTel names.
	seen := map[string]struct{}{}
	allVariants := make([][]*labels.Matcher, 0, len(schemaVariants)*(len(otelStrategies)+1))
	for _, sv := range schemaVariants {
		variants := [][]*labels.Matcher{sv}
		if strategy != nil {
			svMetric, _ := extractMetricName(sv)
			variants = generateOTLPVariants(sv, metaLookup[svMetric]) // nil meta for unknown metrics
		}
		for _, v := range variants {
			key := matcherKey(v)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			allVariants = append(allVariants, v)
		}
	}

	qc := queryContext{labelMapping: buildLabelMapping(metricName, attributes)}
	if strategy != nil {
		qc.outputStrategy = strategy
		qc.outputMeta = outputMeta
		qc.warning = dialectWarning
	}
	return allVariants, qc, nil
}

// transformSeries returns the series labels rewritten to the canonical OTel
// semantic convention names recorded in q.labelMapping. When no mapping
// applies, any stray __schema_url__ label is stripped and the labels are
// returned otherwise unchanged.
func (*schemaEngine) transformSeries(q queryContext, originalLabels labels.Labels) labels.Labels {
	if q.labelMapping != nil {
		canonical := transformOTelSchemaLabels(originalLabels, q.labelMapping)
		if q.outputStrategy == nil {
			return canonical
		}
		// Render the canonical labels in the requested strategy's dialect so
		// every era converges on the same names and merges into one series.
		return forwardTranslateLabels(canonical, q.labelMapping.translatedMetric, q.outputMeta, *q.outputStrategy)
	}
	if originalLabels.Get(schemaURLLabel) == "" {
		return originalLabels
	}
	builder := labels.NewBuilder(originalLabels)
	builder.Del(schemaURLLabel)
	return builder.Labels()
}

// labelMapping maps translated Prometheus names back to original OTLP names.
type labelMapping struct {
	translatedLabels map[string]string // e.g., "http_method" -> "http.method"
	translatedMetric string
}

// buildLabelMapping creates a mapping from translated label names back to original OTLP names.
// For each attribute, generates all possible Prometheus translations and maps them back.
// Translation errors are silently ignored to be lenient with malformed attribute names.
func buildLabelMapping(metricName string, attributes []string) *labelMapping {
	mapping := &labelMapping{
		translatedLabels: make(map[string]string, len(attributes)*len(otelStrategies)),
		translatedMetric: metricName,
	}

	for _, attr := range attributes {
		for _, strategy := range otelStrategies {
			translatedName, err := translateLabelName(attr, strategy)
			if err != nil {
				// Skip attributes that cannot be translated (e.g., invalid names).
				continue
			}
			// Map translated Prometheus name back to original OTel name.
			mapping.translatedLabels[translatedName] = attr
		}
	}

	return mapping
}

// transformOTelSchemaLabels transforms series labels to the current semantic conventions version
// using the label mapping.
func transformOTelSchemaLabels(originalLabels labels.Labels, mapping *labelMapping) labels.Labels {
	builder := labels.NewScratchBuilder(originalLabels.Len())
	originalLabels.Range(func(l labels.Label) {
		switch l.Name {
		case semconvURLLabel, schemaURLLabel:
			// Skip.
		case model.MetricNameLabel:
			builder.Add(l.Name, mapping.translatedMetric)
		default:
			if originalName, ok := mapping.translatedLabels[l.Name]; ok {
				builder.Add(originalName, l.Value)
			} else {
				builder.Add(l.Name, l.Value)
			}
		}
	})
	return builder.Labels()
}
