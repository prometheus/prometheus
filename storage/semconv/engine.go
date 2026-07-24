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

	"github.com/prometheus/prometheus/model/labels"
)

// registrySource provides the raw bytes of registry files addressed by their
// registry/<name> path. The embedded registry (embed.FS) satisfies it directly;
// an operator-provided registry is adapted to it via newRegistrySource.
type registrySource interface {
	ReadFile(name string) ([]byte, error)
}

type schemaEngine struct {
	registry registrySource

	otelSchemaCache *staticCache[otelSchema]
	semconvCache    *staticCache[semconv]
}

func newSchemaEngine(registry registrySource) *schemaEngine {
	return &schemaEngine{
		registry:        registry,
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

// buildAttributeRenameMap returns a map from each historical or forward
// attribute alias to its name at anchorVersion, for the attributes in
// canonicalAttrs (the metric's attributes declared by the anchor semconv
// version). It is anchored and walked exactly like generateMatcherVariants
// (backward over versions <= anchor, forward over versions > anchor), so every
// alias a returned series can carry maps back to the queried version's name.
// Identity entries (alias == canonical) are omitted; it returns nil when the
// schema renames none of the attributes.
func buildAttributeRenameMap(anchorVersion string, schema *otelSchema, canonicalAttrs []string) map[string]string {
	if len(schema.versionRenames) == 0 || len(canonicalAttrs) == 0 {
		return nil
	}
	anchorIdx := findVersionAnchorIndex(schema.versionRenames, anchorVersion)
	backward := schema.versionRenames[:anchorIdx+1]
	forward := schema.versionRenames[anchorIdx+1:]

	out := map[string]string{}
	for _, canon := range canonicalAttrs {
		walkAttributeRenames(backward, canon, true, out)
		walkAttributeRenames(forward, canon, false, out)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// walkAttributeRenames threads canon through the versions' attribute renames,
// recording each distinct produced alias → canon in out. With reverse=true it
// walks newest→oldest, otherwise oldest→newest, chaining via a per-canon seen
// set — mirroring walkVersions so the attribute walk stays consistent with the
// matcher fan-out.
func walkAttributeRenames(versions []versionRenames, canon string, reverse bool, out map[string]string) {
	current := canon
	seen := map[string]struct{}{canon: {}}
	for {
		found := false
		var versionsIter iter.Seq2[int, versionRenames]
		if reverse {
			versionsIter = slices.Backward(versions)
		} else {
			versionsIter = slices.All(versions)
		}

		for _, v := range versionsIter {
			next, ok := v.attributes[current]
			if !ok {
				continue
			}
			if _, exists := seen[next]; exists {
				continue
			}
			seen[next] = struct{}{}
			out[next] = canon
			current = next
			found = true
			break
		}
		if !found {
			break
		}
	}
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

// getSemconv returns the semconv parsed from url, fetching it via the
// embedded registry on a cache miss.
func (e *schemaEngine) getSemconv(url string) (semconv, error) {
	if sc, ok := e.semconvCache.get(url); ok {
		return sc, nil
	}
	sc, err := e.fetchSemconv(url)
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
	s, err := e.fetchOTelSchema(url)
	if err != nil {
		return otelSchema{}, err
	}
	e.otelSchemaCache.set(url, s)
	return s, nil
}

// findMatcherVariants returns all variants to match for a single schematized
// metric selection. semconvURL points to a semantic conventions file and is
// always required. In production schemaURL (an OTel schema file with versioned
// renames) is also always set, because classifyMatchers only triggers fan-out
// when both are present; the empty-schemaURL path exists only for the direct
// unit test. It returns one variant per schema-version rename of the metric,
// plus a label mapping for transforming results back to the requested version.
// The returned matchers do not include the reserved schema matchers. It returns
// an error if semconvURL is not provided.
func (e *schemaEngine) findMatcherVariants(semconvURL, schemaURL string, originalMatchers []*labels.Matcher) ([][]*labels.Matcher, queryContext, error) {
	if semconvURL == "" {
		return nil, queryContext{}, errors.New("semconvURL is required")
	}

	// Filter out the wrapper's reserved matchers.
	matchers := stripReservedLabels(originalMatchers)

	// Fetch semantic conventions for the anchor version (also validates the URL).
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
		// renames against; fall through to the underlying querier without
		// applying any rewrite.
		return [][]*labels.Matcher{matchers}, queryContext{}, nil
	}

	// Generate schema-version rename variants. In production schemaURL is always
	// set (classifyMatchers gates fan-out on it); the empty case is reached only
	// by direct unit tests and falls through to the unmodified matchers.
	allVariants := [][]*labels.Matcher{matchers}
	var attrRenames map[string]string
	if schemaURL != "" {
		schema, err := e.getOTelSchema(schemaURL)
		if err != nil {
			return nil, queryContext{}, err
		}
		allVariants = generateMatcherVariants(sc.version, &schema, matchers)
		// Map each historical attribute alias back to its anchor-version name so
		// results from older or newer eras merge under the queried version's
		// labels instead of splitting on the renamed attribute. Recomputed per
		// query on purpose: it is a pure function of the cached schema/semconv and
		// costs only a few map ops, far less than the fan-out it feeds.
		attrRenames = buildAttributeRenameMap(sc.version, &schema, sc.attributesPerMetric[metricName])
	}

	return allVariants, queryContext{labelMapping: buildLabelMapping(metricName, attrRenames)}, nil
}

// transformSeries returns the series labels rewritten to the canonical OTel
// semantic convention names recorded in q.labelMapping. When no mapping
// applies, any stray __schema_url__ label is stripped and the labels are
// returned otherwise unchanged.
func (*schemaEngine) transformSeries(q queryContext, originalLabels labels.Labels) labels.Labels {
	if q.labelMapping != nil {
		return transformOTelSchemaLabels(originalLabels, q.labelMapping)
	}
	if originalLabels.Get(schemaURLLabel) == "" {
		return originalLabels
	}
	builder := labels.NewBuilder(originalLabels)
	builder.Del(schemaURLLabel)
	return builder.Labels()
}

// labelMapping rewrites a returned series' names to the queried semantic-
// conventions version: translatedMetric is the queried (anchor) metric name
// that every variant collapses to, and translatedLabels maps each historical
// attribute alias back to its anchor-version name.
type labelMapping struct {
	translatedLabels map[string]string // historical attribute alias → anchor name, e.g. "user" -> "tenant"
	translatedMetric string
}

// buildLabelMapping creates the mapping used to rewrite result labels back to
// the requested semantic-conventions version: the result metric name maps to
// the queried (anchor) name, and translatedLabels maps each historical
// attribute alias back to its anchor-version name (nil/empty when no attribute
// was renamed).
func buildLabelMapping(metricName string, translatedLabels map[string]string) *labelMapping {
	return &labelMapping{translatedMetric: metricName, translatedLabels: translatedLabels}
}

// aliasesOf returns name together with every historical alias that maps to it,
// i.e. the set of label names a returned series may carry for the canonical
// name. It is the inverse of translatedLabels and is used to fan LabelValues
// out across a renamed attribute's historical names. The metric name has no
// attribute aliases, so it is returned unchanged.
func (m *labelMapping) aliasesOf(name string) []string {
	aliases := []string{name}
	for alias, canonical := range m.translatedLabels {
		if canonical == name {
			aliases = append(aliases, alias)
		}
	}
	return aliases
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
