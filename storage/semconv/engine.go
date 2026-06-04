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

// findMatcherVariants returns all variants to match for a single schematized
// metric selection. semconvURL points to a semantic conventions file and is
// required; schemaURL points to an OTel schema file (versions with renames)
// and is optional. It returns one variant per schema-version rename of the
// metric, plus a label mapping for transforming results back to the requested
// version. The returned matchers do not include the reserved schema matchers.
// It returns an error if semconvURL is not provided.
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

	// Generate schema-version rename variants (if a schema URL is provided).
	allVariants := [][]*labels.Matcher{matchers}
	if schemaURL != "" {
		schema, err := e.getOTelSchema(schemaURL)
		if err != nil {
			return nil, queryContext{}, err
		}
		allVariants = generateMatcherVariants(sc.version, &schema, matchers)
	}

	return allVariants, queryContext{labelMapping: buildLabelMapping(metricName)}, nil
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

// labelMapping maps translated Prometheus names back to original OTLP names.
type labelMapping struct {
	translatedLabels map[string]string // e.g., "http_method" -> "http.method"
	translatedMetric string
}

// buildLabelMapping creates the mapping used to rewrite result labels back to
// the requested semantic-conventions version: the schema-version fan-out maps
// the result metric name to the queried (anchor) name, while attribute label
// names pass through unchanged.
func buildLabelMapping(metricName string) *labelMapping {
	return &labelMapping{translatedMetric: metricName}
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
