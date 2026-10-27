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
	"maps"
	"slices"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

// RewritePromQLQuery rewrites a PromQL query expression at authoring time using the
// embedded OTel semantic-conventions and transform/2.0 policy registry.
//
// Selectors that carry __schema_url__ (or use defaultSchemaURL when non-empty) are
// expanded into standard PromQL union ("or") expressions with pushed-down source
// matchers, label_replace / label_join wrappers, and sample value filters so the
// resulting query can be executed against an unmodified Prometheus engine.
func RewritePromQLQuery(query, defaultSchemaURL string) (string, annotations.Annotations, error) {
	return rewritePromQLWithEngine(newSchemaEngine(embeddedRegistry), query, defaultSchemaURL)
}

// RewritePromQLQueryWithRegistry is RewritePromQLQuery backed by an operator-provided
// registry file map (keyed by base filename, e.g. "http_1.20.0_to_1.23.1.yaml").
func RewritePromQLQueryWithRegistry(query, defaultSchemaURL string, registryFiles map[string][]byte) (string, annotations.Annotations, error) {
	if err := validateRegistryFiles(registryFiles); err != nil {
		return "", nil, err
	}
	return rewritePromQLWithEngine(newSchemaEngine(newRegistrySource(cloneRegistryFiles(registryFiles))), query, defaultSchemaURL)
}

func rewritePromQLWithEngine(engine *schemaEngine, query, defaultSchemaURL string) (string, annotations.Annotations, error) {
	p := parser.NewParser(parser.Options{})
	expr, err := p.ParseExpr(query)
	if err != nil {
		return "", nil, err
	}
	annos := annotations.New()
	rewritten, err := rewritePromQLExpr(engine, expr, defaultSchemaURL, nil, annos)
	if err != nil {
		return "", *annos, err
	}
	return rewritten.String(), *annos, nil
}

func rewritePromQLExpr(
	engine *schemaEngine,
	expr parser.Expr,
	defaultSchemaURL string,
	projectedLabels map[string]struct{},
	annos *annotations.Annotations,
) (parser.Expr, error) {
	if expr == nil {
		return nil, nil
	}

	switch n := expr.(type) {
	case *parser.AggregateExpr:
		var childProj map[string]struct{}
		if !n.Without {
			childProj = make(map[string]struct{}, len(n.Grouping))
			for _, g := range n.Grouping {
				childProj[g] = struct{}{}
			}
		}
		rewrittenChild, err := rewritePromQLExpr(engine, n.Expr, defaultSchemaURL, childProj, annos)
		if err != nil {
			return nil, err
		}
		out := *n
		out.Expr = rewrittenChild
		return &out, nil

	case *parser.BinaryExpr:
		lhs, err := rewritePromQLExpr(engine, n.LHS, defaultSchemaURL, nil, annos)
		if err != nil {
			return nil, err
		}
		rhs, err := rewritePromQLExpr(engine, n.RHS, defaultSchemaURL, nil, annos)
		if err != nil {
			return nil, err
		}
		out := *n
		out.LHS = lhs
		out.RHS = rhs
		return &out, nil

	case *parser.ParenExpr:
		inner, err := rewritePromQLExpr(engine, n.Expr, defaultSchemaURL, projectedLabels, annos)
		if err != nil {
			return nil, err
		}
		out := *n
		out.Expr = inner
		return &out, nil

	case *parser.UnaryExpr:
		inner, err := rewritePromQLExpr(engine, n.Expr, defaultSchemaURL, projectedLabels, annos)
		if err != nil {
			return nil, err
		}
		out := *n
		out.Expr = inner
		return &out, nil

	case *parser.SubqueryExpr:
		inner, err := rewritePromQLExpr(engine, n.Expr, defaultSchemaURL, projectedLabels, annos)
		if err != nil {
			return nil, err
		}
		out := *n
		out.Expr = inner
		return &out, nil

	case *parser.Call:
		// If the call takes a MatrixSelector (e.g. rate(m[5m]), increase(m[5m])),
		// expand variants around the Call because label_replace / label_join operate on
		// instant vectors rather than range vectors.
		for argIdx, arg := range n.Args {
			ms, ok := arg.(*parser.MatrixSelector)
			if !ok {
				continue
			}
			vs, ok := ms.VectorSelector.(*parser.VectorSelector)
			if !ok {
				continue
			}
			variants, hasSchema, err := resolveSelectorVariants(engine, vs, defaultSchemaURL, annos)
			if err != nil {
				return nil, err
			}
			if !hasSchema {
				return n, nil
			}
			// Drop __name__ projection inside range functions that strip __name__ automatically.
			callProj := projectedLabels
			if callProj == nil {
				callProj = nil
			}
			var branchExprs []parser.Expr
			for _, v := range variants {
				vsClone := buildVariantVectorSelector(vs, v.matchers)
				msClone := *ms
				msClone.VectorSelector = vsClone
				callClone := *n
				callClone.Args = slices.Clone(n.Args)
				callClone.Args[argIdx] = &msClone
				wrapped := wrapExprWithVariantTransforms(&callClone, v, callProj, true)
				branchExprs = append(branchExprs, wrapped)
			}
			return unionVariantExprs(branchExprs), nil
		}

		out := *n
		out.Args = make(parser.Expressions, len(n.Args))
		for i, arg := range n.Args {
			rewrittenArg, err := rewritePromQLExpr(engine, arg, defaultSchemaURL, nil, annos)
			if err != nil {
				return nil, err
			}
			out.Args[i] = rewrittenArg
		}
		return &out, nil

	case *parser.VectorSelector:
		variants, hasSchema, err := resolveSelectorVariants(engine, n, defaultSchemaURL, annos)
		if err != nil {
			return nil, err
		}
		if !hasSchema {
			return n, nil
		}
		var branchExprs []parser.Expr
		for _, v := range variants {
			vsClone := buildVariantVectorSelector(n, v.matchers)
			wrapped := wrapExprWithVariantTransforms(vsClone, v, projectedLabels, false)
			branchExprs = append(branchExprs, wrapped)
		}
		return unionVariantExprs(branchExprs), nil

	default:
		return expr, nil
	}
}

func resolveSelectorVariants(
	engine *schemaEngine,
	vs *parser.VectorSelector,
	defaultSchemaURL string,
	annos *annotations.Annotations,
) ([]matcherVariant, bool, error) {
	semconvURL, schemaURL, warn, fanout := classifyMatchers(vs.LabelMatchers)
	if warn != "" {
		annos.Add(schemaWarning(warn))
		return []matcherVariant{{matchers: stripReservedLabels(vs.LabelMatchers)}}, true, nil
	}
	if !fanout {
		if defaultSchemaURL == "" {
			return nil, false, nil
		}
		schemaURL = defaultSchemaURL
	}

	variants, qctx, err := engine.findMatcherVariants(semconvURL, schemaURL, vs.LabelMatchers)
	if err != nil {
		return nil, false, err
	}
	for _, w := range qctx.warnings {
		annos.Add(schemaWarning(w))
	}
	return variants, true, nil
}

func buildVariantVectorSelector(orig *parser.VectorSelector, matchers []*labels.Matcher) *parser.VectorSelector {
	clone := *orig
	clone.Name = ""
	clonedMatchers := make([]*labels.Matcher, 0, len(matchers))
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel && m.Type == labels.MatchEqual && model.LegacyValidation.IsValidMetricName(m.Value) {
			clone.Name = m.Value
		}
		clonedMatchers = append(clonedMatchers, m)
	}
	clone.LabelMatchers = clonedMatchers
	return &clone
}

func unionVariantExprs(exprs []parser.Expr) parser.Expr {
	if len(exprs) == 0 {
		return &parser.VectorSelector{}
	}
	acc := exprs[0]
	for _, rhs := range exprs[1:] {
		acc = &parser.BinaryExpr{
			Op:  parser.LOR,
			LHS: acc,
			RHS: rhs,
			VectorMatching: &parser.VectorMatching{
				Card: parser.CardManyToMany,
			},
		}
	}
	return acc
}

func wrapExprWithVariantTransforms(
	baseExpr parser.Expr,
	variant matcherVariant,
	projectedLabels map[string]struct{},
	metricDroppedByFunc bool,
) parser.Expr {
	needsLabel := func(name string) bool {
		if name == model.MetricNameLabel && metricDroppedByFunc {
			return false
		}
		if projectedLabels == nil {
			return true
		}
		_, ok := projectedLabels[name]
		return ok
	}

	expr := baseExpr
	srcMetric, _ := extractMetricName(variant.matchers)

	if variant.mapping == nil {
		return appendresidualCanonicalMatchers(expr, variant)
	}

	if pt := variant.mapping.policyTransform; pt != nil {
		allPolicies := make([]transformPolicy, 0, len(pt.entityPolicies)+len(pt.metricPolicies))
		allPolicies = append(allPolicies, pt.entityPolicies...)
		allPolicies = append(allPolicies, pt.metricPolicies...)

		// Apply sample value filtering/replacement/scaling if configured.
		rule := pt.presetRule
		if rule == nil {
			for _, p := range pt.metricPolicies {
				if p.ops.scale != 0 && p.ops.scale != 1 && pt.histSuffix != "_count" {
					rule = &sampleValueRule{scale: p.ops.scale}
					break
				}
			}
		}
		if rule != nil {
			if rule.hasMatchValue {
				expr = &parser.ParenExpr{
					Expr: &parser.BinaryExpr{
						Op:  parser.EQLC,
						LHS: expr,
						RHS: &parser.NumberLiteral{Val: rule.matchValue},
					},
				}
			}
			for _, dv := range rule.dropValues {
				expr = &parser.ParenExpr{
					Expr: &parser.BinaryExpr{
						Op:  parser.NEQ,
						LHS: expr,
						RHS: &parser.NumberLiteral{Val: dv},
					},
				}
			}
			if rule.hasReplaceValue {
				expr = &parser.ParenExpr{
					Expr: &parser.BinaryExpr{
						Op: parser.ADD,
						LHS: &parser.BinaryExpr{
							Op:  parser.MUL,
							LHS: expr,
							RHS: &parser.NumberLiteral{Val: 0},
						},
						RHS: &parser.NumberLiteral{Val: rule.replaceValue},
					},
				}
			}
			if rule.scale != 0 && rule.scale != 1 {
				expr = &parser.ParenExpr{
					Expr: &parser.BinaryExpr{
						Op:  parser.MUL,
						LHS: expr,
						RHS: &parser.NumberLiteral{Val: rule.scale},
					},
				}
			}
		}

		// Stage 2: rename.
		renamedToFrom := map[string]string{}
		for _, p := range allPolicies {
			for _, r := range p.ops.rename {
				if r.from.isAttr() && r.to.isAttr() && r.from.key != r.to.key {
					renamedToFrom[r.to.key] = r.from.key
					if needsLabel(r.to.key) {
						expr = buildLabelReplaceCall(expr, r.to.key, "$1", r.from.key, "(.+)")
					}
				}
			}
		}

		// Stage 3: split.
		for _, p := range allPolicies {
			for _, s := range p.ops.split {
				if !s.from.isAttr() || s.separator == "" || len(s.to) != 2 {
					continue
				}
				sepRe := regexp.QuoteMeta(s.separator)
				if s.to[0].isAttr() && s.to[0].key != "" && needsLabel(s.to[0].key) {
					expr = buildLabelReplaceCall(expr, s.to[0].key, "$1", s.from.key, "^([^"+sepRe+"]+)"+sepRe+".*$")
				}
				if s.to[1].isAttr() && s.to[1].key != "" && needsLabel(s.to[1].key) {
					expr = buildLabelReplaceCall(expr, s.to[1].key, "$1", s.from.key, "^.*"+sepRe+"(.+)$")
				}
			}
		}

		// Stage 4: add.
		for _, p := range allPolicies {
			if hasNegativeAttrMatcher(&p) {
				continue
			}
			for _, a := range p.ops.add {
				if !a.target.isAttr() || !needsLabel(a.target.key) {
					continue
				}
				if a.value != nil && len(a.from) == 0 {
					expr = buildLabelReplaceCall(expr, a.target.key, *a.value, model.MetricNameLabel, ".*")
				} else if len(a.from) == 1 && a.from[0].isAttr() {
					srcAttr := a.from[0].key
					if !needsLabel(srcAttr) {
						if origSrc, ok := renamedToFrom[srcAttr]; ok {
							srcAttr = origSrc
						}
					}
					expr = buildLabelReplaceCall(expr, a.target.key, "$1", srcAttr, "(.+)")
				} else if len(a.from) > 1 {
					srcAttrs := make([]string, 0, len(a.from))
					for _, ff := range a.from {
						if ff.isAttr() {
							srcAttr := ff.key
							if !needsLabel(srcAttr) {
								if origSrc, ok := renamedToFrom[srcAttr]; ok {
									srcAttr = origSrc
								}
							}
							srcAttrs = append(srcAttrs, srcAttr)
						}
					}
					if len(srcAttrs) > 1 {
						expr = buildLabelJoinCall(expr, a.target.key, a.separator, srcAttrs...)
					}
				}
			}
		}

		// Stage 5: map.
		for _, p := range allPolicies {
			for _, m := range p.ops.mapValues {
				if !m.target.isAttr() || !needsLabel(m.target.key) {
					continue
				}
				for _, oldVal := range slices.Sorted(maps.Keys(m.values)) {
					newVal := m.values[oldVal]
					if oldVal == newVal || newVal == "" {
						continue
					}
					expr = buildLabelReplaceCall(expr, m.target.key, newVal, m.target.key, "^"+regexp.QuoteMeta(oldVal)+"$")
				}
			}
		}
	} else {
		for _, alias := range slices.Sorted(maps.Keys(variant.mapping.translatedLabels)) {
			canonical := variant.mapping.translatedLabels[alias]
			if alias != canonical && needsLabel(canonical) {
				expr = buildLabelReplaceCall(expr, canonical, "$1", alias, "(.+)")
			}
		}
	}

	if srcMetric != "" && variant.mapping.translatedMetric != "" && srcMetric != variant.mapping.translatedMetric && needsLabel(model.MetricNameLabel) {
		expr = buildLabelReplaceCall(expr, model.MetricNameLabel, variant.mapping.translatedMetric, model.MetricNameLabel, ".*")
	}

	return expr
}

func appendresidualCanonicalMatchers(expr parser.Expr, _ matcherVariant) parser.Expr {
	return expr
}

func hasNegativeAttrMatcher(p *transformPolicy) bool {
	for _, m := range p.matches {
		if m.target.isAttr() && m.negate && m.predicate.kind == predicateRegex {
			return true
		}
	}
	return false
}

func buildLabelReplaceCall(vec parser.Expr, dstLabel, replacement, srcLabel, regexStr string) parser.Expr {
	fn := parser.Functions["label_replace"]
	return &parser.Call{
		Func: fn,
		Args: parser.Expressions{
			vec,
			&parser.StringLiteral{Val: dstLabel},
			&parser.StringLiteral{Val: replacement},
			&parser.StringLiteral{Val: srcLabel},
			&parser.StringLiteral{Val: regexStr},
		},
	}
}

func buildLabelJoinCall(vec parser.Expr, dstLabel, separator string, srcLabels ...string) parser.Expr {
	fn := parser.Functions["label_join"]
	args := make(parser.Expressions, 0, 3+len(srcLabels))
	args = append(args,
		vec,
		&parser.StringLiteral{Val: dstLabel},
		&parser.StringLiteral{Val: separator},
	)
	for _, s := range srcLabels {
		args = append(args, &parser.StringLiteral{Val: s})
	}
	return &parser.Call{
		Func: fn,
		Args: args,
	}
}
