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
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	// maxSchemaExpansion caps each bounded resolver collection.
	maxSchemaExpansion = 256

	// maxSchemaExpansionWork bounds cumulative resolver work across collections.
	maxSchemaExpansionWork = maxSchemaExpansion * maxSchemaExpansion

	// maxSchemaExpansionKeyBytes bounds cumulative synthesized deduplication keys.
	maxSchemaExpansionKeyBytes = maxRegistryDecompressedBytes
)

var (
	errMetricNameAnchor      = errors.New("schema-aware query requires a non-empty equality matcher on __name__")
	errSchemaExpansion       = errors.New("semconv schema expansion limit exceeded")
	errAmbiguousSchemaRename = errors.New("semconv schema rename is ambiguous")
	errUnsafeSchemaMatcher   = errors.New("semconv schema matcher cannot be expanded safely")
	errSemconvURLRequired    = errors.New("semconvURL is required")
)

func schemaExpansionError(kind string) error {
	return fmt.Errorf("%w: %s would exceed %d", errSchemaExpansion, kind, maxSchemaExpansion)
}

func schemaExpansionLimitError(kind string, limit uint64) error {
	return fmt.Errorf("%w: %s would exceed %d", errSchemaExpansion, kind, limit)
}

type schemaExpansionLimits struct {
	work     uint64
	keyBytes uint64
}

func productionSchemaExpansionLimits() schemaExpansionLimits {
	return schemaExpansionLimits{
		work:     maxSchemaExpansionWork,
		keyBytes: maxSchemaExpansionKeyBytes,
	}
}

// schemaExpansionBudget is shared by one resolver invocation. It charges
// attempted work, including candidates later discarded as duplicates.
type schemaExpansionBudget struct {
	limits   schemaExpansionLimits
	work     uint64
	keyBytes uint64
}

func newSchemaExpansionBudget(limits schemaExpansionLimits) *schemaExpansionBudget {
	return &schemaExpansionBudget{limits: limits}
}

func (b *schemaExpansionBudget) reserveWork(n uint64) error {
	if b == nil || n == 0 {
		return nil
	}
	if b.work > b.limits.work || n > b.limits.work-b.work {
		return schemaExpansionLimitError("resolver work", b.limits.work)
	}
	b.work += n
	return nil
}

func (b *schemaExpansionBudget) reserveKeyBytes(n uint64) error {
	if b == nil || n == 0 {
		return nil
	}
	if b.keyBytes > b.limits.keyBytes || n > b.limits.keyBytes-b.keyBytes {
		return schemaExpansionLimitError("deduplication key bytes", b.limits.keyBytes)
	}
	b.keyBytes += n
	return nil
}

func (b *schemaExpansionBudget) remainingKeyBytes() uint64 {
	if b == nil {
		return ^uint64(0)
	}
	if b.keyBytes > b.limits.keyBytes {
		return 0
	}
	return b.limits.keyBytes - b.keyBytes
}

// registrySource provides the raw bytes of registry files addressed by their
// registry/<name> path.
type registrySource interface {
	ReadFile(name string) ([]byte, error)
}

type schemaEngine struct {
	registry        registrySource
	limits          schemaExpansionLimits
	otelSchemaCache *staticCache[otelSchema]
}

func newSchemaEngine(registry registrySource) *schemaEngine {
	return &schemaEngine{
		registry:        registry,
		limits:          productionSchemaExpansionLimits(),
		otelSchemaCache: newStaticCache[otelSchema](),
	}
}

type matcherVariant struct {
	matchers          []*labels.Matcher
	mapping           *labelMapping
	canonicalMatchers []*labels.Matcher
}

func extractMetricName(matchers []*labels.Matcher) (string, error) {
	hasMetricMatcher := false
	for _, m := range matchers {
		if m.Name != model.MetricNameLabel {
			continue
		}
		hasMetricMatcher = true
		if m.Type == labels.MatchEqual && m.Value != "" {
			return m.Value, nil
		}
	}
	if hasMetricMatcher {
		return "", errMetricNameAnchor
	}
	return "", nil
}

// normalizeMetricMatchers evaluates every metric-name constraint against the
// exact name that anchors schema traversal. Compatible constraints are
// redundant once that equality is translated to each naming era.
func normalizeMetricMatchers(matchers []*labels.Matcher) (string, []*labels.Matcher, bool, error) {
	metricName, err := extractMetricName(matchers)
	if err != nil || metricName == "" {
		return metricName, matchers, true, err
	}

	metricMatchers := 0
	for _, matcher := range matchers {
		if matcher.Name != model.MetricNameLabel {
			continue
		}
		metricMatchers++
		if !matcher.Matches(metricName) {
			return metricName, matchers, false, nil
		}
	}
	if metricMatchers == 1 {
		return metricName, matchers, true, nil
	}

	out := make([]*labels.Matcher, 0, len(matchers)-metricMatchers+1)
	insertedMetric := false
	for _, matcher := range matchers {
		if matcher.Name != model.MetricNameLabel {
			out = append(out, matcher)
			continue
		}
		if !insertedMetric {
			out = append(out, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, metricName))
			insertedMetric = true
		}
	}
	return metricName, out, true, nil
}

func replaceMetricMatchersWithBudget(matchers []*labels.Matcher, currentMetric, targetMetric string, budget *schemaExpansionBudget) ([]*labels.Matcher, error) {
	if err := budget.reserveWork(uint64(len(matchers))); err != nil {
		return nil, err
	}
	out := slices.Clone(matchers)
	for i, matcher := range out {
		if matcher.Name == model.MetricNameLabel && matcher.Type == labels.MatchEqual && matcher.Value == currentMetric {
			out[i] = labels.MustNewMatcher(matcher.Type, matcher.Name, targetMetric)
		}
	}
	return out, nil
}

func matcherKeyWithBudget(matchers []*labels.Matcher, budget *schemaExpansionBudget) (string, error) {
	if err := budget.reserveWork(1); err != nil {
		return "", err
	}
	remaining := budget.remainingKeyBytes()
	size := keyIntSize(len(matchers))
	if size > remaining {
		return "", schemaExpansionLimitError("deduplication key bytes", budget.limits.keyBytes)
	}
	for _, matcher := range matchers {
		if err := budget.reserveWork(1); err != nil {
			return "", err
		}
		parts := 1 + keyPartSize(uint64(len(matcher.Name))) + keyPartSize(uint64(len(matcher.Value)))
		if parts > remaining-size {
			return "", schemaExpansionLimitError("deduplication key bytes", budget.limits.keyBytes)
		}
		size += parts
	}
	if err := budget.reserveKeyBytes(size); err != nil {
		return "", err
	}

	var b strings.Builder
	b.Grow(int(size))
	writeKeyInt(&b, len(matchers))
	for _, matcher := range matchers {
		b.WriteByte(byte(matcher.Type))
		writeKeyPart(&b, matcher.Name)
		writeKeyPart(&b, matcher.Value)
	}
	return b.String(), nil
}

func keyPartSize(valueLen uint64) uint64 {
	return decimalDigits(valueLen) + 1 + valueLen
}

func keyIntSize(value int) uint64 {
	digits := decimalDigits(uint64(value))
	return keyPartSize(digits)
}

func decimalDigits(value uint64) uint64 {
	digits := uint64(1)
	for value >= 10 {
		value /= 10
		digits++
	}
	return digits
}

func writeKeyPart(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
}

func writeKeyInt(b *strings.Builder, value int) {
	writeKeyPart(b, strconv.Itoa(value))
}

type queryContext struct {
	warnings []string
}

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

// getTransformSchemasByTarget returns all transform/2.0 schemas in the registry
// whose target.schema_url matches targetURL (either as the full target URL or as
// registry/<path.Base(targetURL)>).
func (e *schemaEngine) getTransformSchemasByTarget(targetURL string) []*otelSchema {
	if targetURL == "" {
		return nil
	}
	var matched []*otelSchema
	for _, yamlPath := range listRegistryYAMLFiles(e.registry) {
		s, err := e.getOTelSchema(yamlPath)
		if err != nil || s.FileFormat != transformFileFormat || s.transform == nil {
			continue
		}
		tURL := s.transform.targetURL
		if tURL == "" {
			continue
		}
		if tURL == targetURL || registryPrefix+"/"+path.Base(tURL) == targetURL {
			sCopy := s
			matched = append(matched, &sCopy)
		}
	}
	return matched
}

// findMatcherVariants returns all variants to match for a single schematized
// metric selection using the transform/2.0 policy referenced by schemaURL.
func (e *schemaEngine) findMatcherVariants(semconvURL, schemaURL string, originalMatchers []*labels.Matcher) ([]matcherVariant, queryContext, error) {
	if schemaURL == "" {
		return nil, queryContext{}, errSemconvURLRequired
	}
	if semconvURL != "" && !registryURLRe.MatchString(semconvURL) {
		return nil, queryContext{}, fmt.Errorf("invalid semconv URL %q: must match %s", semconvURL, registryURLRe.String())
	}

	matchers := stripReservedLabels(originalMatchers)

	metricName, normalizedMatchers, satisfiable, err := normalizeMetricMatchers(matchers)
	if err != nil {
		return nil, queryContext{}, err
	}
	if metricName == "" {
		return nil, queryContext{}, errMetricNameAnchor
	}
	if !satisfiable {
		return []matcherVariant{{matchers: matchers}}, queryContext{}, nil
	}
	matchers = normalizedMatchers

	schema, err := e.getOTelSchema(schemaURL)
	if err != nil {
		// If schemaURL is a target schema URL or registry/<target-version> rather than
		// a direct .yaml filename, check if one or more transform/2.0 policies target it.
		if targetSchemas := e.getTransformSchemasByTarget(schemaURL); len(targetSchemas) > 0 {
			budget := newSchemaExpansionBudget(e.limits)
			variants, warnings, genErr := generateTransformPolicyVariantsMulti(targetSchemas, matchers, budget)
			if genErr != nil {
				return nil, queryContext{}, genErr
			}
			return variants, queryContext{warnings: warnings}, nil
		}
		return nil, queryContext{}, err
	}

	budget := newSchemaExpansionBudget(e.limits)
	variants, warnings, err := generateTransformPolicyVariants(&schema, matchers, budget)
	if err != nil {
		return nil, queryContext{}, err
	}
	return variants, queryContext{warnings: warnings}, nil
}

type labelMapping struct {
	translatedLabels map[string]string
	translatedMetric string
	policyTransform  *policySeriesTransformer
}

func buildLabelMapping(metricName string, translatedLabels map[string]string) *labelMapping {
	return &labelMapping{translatedMetric: metricName, translatedLabels: translatedLabels}
}

func (m *labelMapping) aliasesOf(name string) []string {
	aliases := []string{name}
	for alias, canonical := range m.translatedLabels {
		if canonical == name {
			aliases = append(aliases, alias)
		}
	}
	slices.Sort(aliases[1:])
	return aliases
}

func transformOTelSchemaLabels(originalLabels labels.Labels, mapping *labelMapping) (labels.Labels, error) {
	if mapping != nil && mapping.policyTransform != nil {
		return mapping.policyTransform.transformLabels(originalLabels)
	}
	return originalLabels, nil
}

func labelMappingChangesLabels(lbls labels.Labels, mapping *labelMapping) bool {
	if mapping != nil && mapping.policyTransform != nil {
		return true
	}
	changed := false
	lbls.Range(func(label labels.Label) {
		if changed {
			return
		}
		if isReservedLabel(label.Name) {
			changed = true
			return
		}
		if mapping == nil {
			return
		}
		if label.Name == model.MetricNameLabel {
			changed = label.Value != mapping.translatedMetric
			return
		}
		_, changed = mapping.translatedLabels[label.Name]
	})
	return changed
}
