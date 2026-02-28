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
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

const cacheTTL = 1 * time.Hour

type schemaEngine struct {
	otelSchemaCache *cache[otelSchema]
	semconvCache    *cache[semconv]
}

func newSchemaEngine() *schemaEngine {
	return &schemaEngine{
		otelSchemaCache: newCache[otelSchema](),
		semconvCache:    newCache[semconv](),
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
}

// FindMatcherVariants returns all variants to match for a single schematized metric selection.
// semconvURL points to a semantic conventions file (groups with metric metadata) and is required.
// schemaURL points to an OTel schema file (versions with attribute renames) and is optional.
// Returns variants for all semantic conventions renames and OTLP translation strategies,
// plus a label mapping for transforming results to the current version.
// The returned matchers do not include __semconv_url__ or __schema_url__.
// It returns an error if semconvURL is not provided or if the metric is not found.
func (e *schemaEngine) FindMatcherVariants(semconvURL, schemaURL string, originalMatchers []*labels.Matcher) ([][]*labels.Matcher, queryContext, error) {
	if semconvURL == "" {
		return nil, queryContext{}, errors.New("semconvURL is required")
	}

	// Filter out special matchers used for schema resolution.
	matchers := make([]*labels.Matcher, 0, len(originalMatchers))
	for _, m := range originalMatchers {
		if m.Name != semconvURLLabel && m.Name != schemaURLLabel {
			matchers = append(matchers, m)
		}
	}

	// Fetch semantic conventions for metric metadata.
	sc, ok := e.semconvCache.get(semconvURL)
	if !ok {
		var err error
		sc, err = fetchSemconv(semconvURL)
		if err != nil {
			return nil, queryContext{}, err
		}
		e.semconvCache.set(semconvURL, sc)
	}

	metricName, err := extractMetricName(matchers)
	if err != nil {
		return nil, queryContext{}, err
	}
	// TODO: Handle if extracted metric name is empty.

	// Get metric metadata for OTLP translation (unit, type).
	var meta *metricMeta
	if m, ok := sc.metricMetadata[metricName]; ok {
		meta = &m
	}

	// Stage 1: Generate schema version variants (if schemaURL provided).
	schemaVariants := [][]*labels.Matcher{matchers}
	var attributes []string
	if schemaURL != "" {
		schema, ok := e.otelSchemaCache.get(schemaURL)
		if !ok {
			schema, err = fetchOTelSchema(schemaURL)
			if err != nil {
				return nil, queryContext{}, err
			}
			e.otelSchemaCache.set(schemaURL, schema)
		}
		schemaVariants = generateMatcherVariants(sc.version, &schema, matchers)
		// Merge semconv attributes with schema attributes (which include renamed versions).
		attributes = mergeAttributes(sc.attributesPerMetric[metricName], schema.getAttributesForMetric(metricName))
	} else {
		attributes = sc.attributesPerMetric[metricName]
	}

	// Stage 2: Generate OTLP translation variants for each schema variant (cross-product).
	allVariants, err := generateCombinedVariants(schemaVariants, meta)
	if err != nil {
		return nil, queryContext{}, err
	}

	return allVariants, queryContext{
		labelMapping: buildLabelMapping(metricName, attributes),
	}, nil
}

// generateCombinedVariants generates the cross-product of schema variants and OTLP strategies.
// For each schema version variant, generates all OTLP translation variants and deduplicates.
func generateCombinedVariants(schemaVariants [][]*labels.Matcher, meta *metricMeta) ([][]*labels.Matcher, error) {
	seen := make(map[string]struct{})
	result := make([][]*labels.Matcher, 0, len(schemaVariants)*(len(otelStrategies)+1))

	for _, schemaVariant := range schemaVariants {
		otlpVariants, err := generateOTLPVariants(schemaVariant, meta)
		if err != nil {
			return nil, err
		}
		for _, v := range otlpVariants {
			key := matcherKey(v)
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				result = append(result, v)
			}
		}
	}
	return result, nil
}

// TransformSeries transforms originalLabels to the current semantic conventions version.
// TODO: Fix me.
func (*schemaEngine) TransformSeries(q queryContext, originalLabels labels.Labels) (labels.Labels, error) {
	// If we have a label mapping from a schema, transform back to original names.
	if q.labelMapping != nil {
		return transformOTelSchemaLabels(originalLabels, q.labelMapping), nil
	}

	schemaURL := originalLabels.Get(schemaURLLabel)
	if schemaURL == "" {
		return originalLabels, nil
	}

	// Remove __schema_url__.
	builder := labels.NewBuilder(originalLabels)
	builder.Del(schemaURLLabel)
	return builder.Labels(), nil
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
