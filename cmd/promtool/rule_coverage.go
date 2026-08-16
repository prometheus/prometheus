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

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/regexp"
	"github.com/grafana/regexp/syntax"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
)

// ruleKind distinguishes alerting from recording rules in coverage reports.
type ruleKind uint8

const (
	alertingRule ruleKind = iota
	recordingRule
)

// String returns the rule kind as it appears in rule files, "alert" or "record".
func (k ruleKind) String() string {
	if k == alertingRule {
		return "alert"
	}
	return "record"
}

// Meta-metrics an alerting rule emits, and the label carrying the alert state.
const (
	alertsMetricName         = "ALERTS"
	alertsForStateMetricName = "ALERTS_FOR_STATE"
	alertStateLabel          = "alertstate"
)

// alertMetric is a meta-metric an alerting rule emits, with the labels the engine
// generates on it. Generated labels override the rule's own, so they are matched
// against what the engine produces instead of the rule's static labels.
type alertMetric struct {
	name      string
	generated []string
	// hasState reports whether the engine sets alertstate on this series.
	hasState bool
}

// alertMetrics describes both meta-metrics. ALERTS_FOR_STATE gets no generated
// alertstate, so there alertstate is an ordinary label.
var alertMetrics = []alertMetric{
	{
		name:      alertsMetricName,
		generated: []string{labels.MetricName, labels.AlertName, alertStateLabel},
		hasState:  true,
	},
	{
		name:      alertsForStateMetricName,
		generated: []string{labels.MetricName, labels.AlertName},
	},
}

// reachableAlertStates returns the alertstate values an ALERTS series can carry.
// The state settles before the sample is emitted, so a rule with no hold duration
// is already firing on its first evaluation and is never observable as pending.
func reachableAlertStates(holdDuration time.Duration) []string {
	if holdDuration <= 0 {
		return []string{"firing"}
	}
	return []string{"pending", "firing"}
}

// recordingRuleGenerated are the labels the engine sets on a recording rule's
// output. alertname is an ordinary label there.
var recordingRuleGenerated = []string{labels.MetricName}

// ruleCoverageKey identifies a rule by where it is declared, so the same rule
// reached from several test files counts once while lookalike declarations stay
// distinct.
type ruleCoverageKey struct {
	file  string // Canonical, symlink-resolved absolute path.
	group string
	// ordinal is the rule's index in its group; the loader preserves file order.
	ordinal int
}

// ruleCoverageEntry is the reporting metadata and covered state of a rule.
type ruleCoverageEntry struct {
	kind    ruleKind
	name    string
	labels  labels.Labels
	covered bool
}

// ruleCoverage tracks which rules a test suite exercises. It is populated from
// rules.Manager.LoadGroups so it stays consistent with how Prometheus loads rules.
type ruleCoverage struct {
	parser              parser.Parser
	ignoreUnknownFields bool
	manager             *rules.Manager // Loads rule groups; never evaluates them.

	order   []ruleCoverageKey
	entries map[ruleCoverageKey]*ruleCoverageEntry

	// groups caches groups per canonical path; a nil entry marks a load failure.
	groups map[string][]*rules.Group
	// loadFailures holds, per rule file, why it was left out of the totals,
	// which then understate the suite. failureOrder keeps reporting stable.
	loadFailures map[string][]error
	failureOrder []string
}

// addLoadFailure records that a rule file could not contribute to the totals,
// either because it failed to load or because nothing matched its pattern.
func (c *ruleCoverage) addLoadFailure(file string, errs ...error) {
	if _, ok := c.loadFailures[file]; !ok {
		c.failureOrder = append(c.failureOrder, file)
	}
	c.loadFailures[file] = append(c.loadFailures[file], errs...)
}

func newRuleCoverage(p parser.Parser, ignoreUnknownFields bool) *ruleCoverage {
	return &ruleCoverage{
		parser:              p,
		ignoreUnknownFields: ignoreUnknownFields,
		// Pass a logger explicitly, as the unit test path does, so rule file
		// warnings are discarded instead of interleaved with the report.
		manager: rules.NewManager(&rules.ManagerOptions{
			Parser: p,
			Logger: promslog.NewNopLogger(),
		}),
		entries:      map[ruleCoverageKey]*ruleCoverageEntry{},
		groups:       map[string][]*rules.Group{},
		loadFailures: map[string][]error{},
	}
}

// register counts a rule towards the coverage total. It is idempotent: a rule
// registered again keeps its covered state.
func (c *ruleCoverage) register(file, group string, ordinal int, kind ruleKind, name string, ruleLabels labels.Labels) ruleCoverageKey {
	k := ruleCoverageKey{file: file, group: group, ordinal: ordinal}
	if _, ok := c.entries[k]; !ok {
		c.order = append(c.order, k)
		c.entries[k] = &ruleCoverageEntry{kind: kind, name: name, labels: ruleLabels}
	}
	return k
}

// mark flags the rule identified by k as covered by a test assertion.
func (c *ruleCoverage) mark(k ruleCoverageKey) {
	c.entries[k].covered = true
}

// canonicalRuleFilePath resolves file to an absolute, symlink-free path so the
// same physical file reached by different routes counts once. An unresolvable
// path falls back to the cleaned input.
func canonicalRuleFilePath(file string) string {
	if resolved, err := filepath.EvalSymlinks(file); err == nil {
		file = resolved
	}
	if abs, err := filepath.Abs(file); err == nil {
		file = abs
	}
	return filepath.Clean(file)
}

// load returns a rule file's canonical path and groups, parsing it on first use.
// Files are loaded one at a time so a broken file does not drop its valid
// siblings, and failures go to loadErrs rather than being dropped. interval only
// sets Group.Interval, which coverage ignores, so the cache is shared across
// test files that disagree on it.
func (c *ruleCoverage) load(file string, interval time.Duration) (string, []*rules.Group) {
	path := canonicalRuleFilePath(file)
	if groups, ok := c.groups[path]; ok {
		return path, groups
	}
	groupsMap, errs := c.manager.LoadGroups(interval, labels.EmptyLabels(), "", nil, c.ignoreUnknownFields, file)
	if len(errs) > 0 {
		// Cache the failure so a shared broken file is reported once.
		c.groups[path] = nil
		c.addLoadFailure(path, errs...)
		return path, nil
	}
	// Sort, so registration order does not depend on map iteration order.
	groups := make([]*rules.Group, 0, len(groupsMap))
	for _, g := range groupsMap {
		groups = append(groups, g)
	}
	slices.SortFunc(groups, func(a, b *rules.Group) int {
		return strings.Compare(a.Name(), b.Name())
	})
	c.groups[path] = groups
	return path, groups
}

// record registers the rules of a unit test file's rule files and marks those its
// assertions exercise. Rules are attributed through their loaded group rather than
// by bare name. Only test groups selected by run contribute coverage, so the
// report reflects what this invocation actually ran.
func (c *ruleCoverage) record(run *regexp.Regexp, utf *unitTestFile) {
	// Collect what the test groups that will actually run assert on.
	testedAlertnames := map[string]struct{}{}
	var exprSelectors [][]*labels.Matcher
	for i := range utf.Tests {
		tg := &utf.Tests[i]
		if !matchesRun(tg.TestGroupName, run) {
			continue
		}
		for _, a := range tg.AlertRuleTests {
			testedAlertnames[a.Alertname] = struct{}{}
		}
		for _, tc := range tg.PromqlExprTests {
			expr, err := c.parser.ParseExpr(tc.Expr)
			if err != nil {
				continue
			}
			exprSelectors = append(exprSelectors, parser.ExtractSelectors(expr)...)
		}
	}

	// A rule_files entry that matched nothing never reaches the loader, so the
	// rules it would have contributed are missing from the totals.
	for _, pattern := range utf.unmatchedRuleFiles {
		c.addLoadFailure(canonicalRuleFilePath(pattern),
			fmt.Errorf("no file matched pattern %q", pattern))
	}

	for _, rf := range utf.RuleFiles {
		path, groups := c.load(rf, time.Duration(utf.EvaluationInterval))
		for _, g := range groups {
			for i, r := range g.Rules() {
				switch rule := r.(type) {
				case *rules.AlertingRule:
					k := c.register(path, g.Name(), i, alertingRule, rule.Name(), rule.Labels())
					// Covered by an alert_rule_test naming it, or by a
					// promql_expr_test selecting its meta-metrics.
					if _, ok := testedAlertnames[rule.Name()]; ok {
						c.mark(k)
						continue
					}
					for _, matchers := range exprSelectors {
						if selectorCoversAlertingRule(rule.Name(), rule.HoldDuration(), rule.Labels(), matchers) {
							c.mark(k)
							break
						}
					}
				case *rules.RecordingRule:
					k := c.register(path, g.Name(), i, recordingRule, rule.Name(), rule.Labels())
					for _, matchers := range exprSelectors {
						if selectorCoversRecordingRule(rule.Name(), rule.Labels(), matchers) {
							c.mark(k)
							break
						}
					}
				}
			}
		}
	}
}

// matchersFor returns the matchers constraining name. PromQL allows a label to be
// constrained repeatedly and requires all such matchers to hold, so callers must
// satisfy every matcher returned, not just the first.
func matchersFor(matchers []*labels.Matcher, name string) []*labels.Matcher {
	var out []*labels.Matcher
	for _, m := range matchers {
		if m.Name == name {
			out = append(out, m)
		}
	}
	return out
}

// allMatch reports whether value satisfies every matcher in ms. An empty ms
// constrains nothing and matches.
func allMatch(ms []*labels.Matcher, value string) bool {
	for _, m := range ms {
		if !m.Matches(value) {
			return false
		}
	}
	return true
}

// selectorCoversRecordingRule reports whether a promql_expr_test selector reads
// the named recording rule's output. It is stricter than buildDependencyMap in
// rules/group.go, which matches on name alone.
func selectorCoversRecordingRule(name string, ruleLabels labels.Labels, matchers []*labels.Matcher) bool {
	nameMatchers := matchersFor(matchers, labels.MetricName)
	if len(nameMatchers) == 0 {
		// A wildcard selector cannot be attributed to a specific rule.
		return false
	}
	if !allMatch(nameMatchers, name) {
		return false
	}
	// Recording rule labels are used verbatim, so their values are known exactly.
	return labelsCompatible(ruleLabels, matchers, recordingRuleGenerated, false)
}

// selectorCoversAlertingRule reports whether a promql_expr_test selector asserts
// on the named alerting rule through one of its meta-metrics. One meta-metric must
// satisfy every matcher at once. A selector without an alertname matcher is
// indeterminate and does not count.
func selectorCoversAlertingRule(name string, holdDuration time.Duration, ruleLabels labels.Labels, matchers []*labels.Matcher) bool {
	nameMatchers := matchersFor(matchers, labels.MetricName)
	alertnameMatchers := matchersFor(matchers, labels.AlertName)
	if len(nameMatchers) == 0 || len(alertnameMatchers) == 0 {
		return false
	}
	// Both meta-metrics carry alertname set to the rule name.
	if !allMatch(alertnameMatchers, name) {
		return false
	}
	for _, am := range alertMetrics {
		if !allMatch(nameMatchers, am.name) {
			continue
		}
		// The selector must accept at least one reachable alert state.
		if am.hasState {
			stateMatchers := matchersFor(matchers, alertStateLabel)
			states := reachableAlertStates(holdDuration)
			if !slices.ContainsFunc(states, func(s string) bool { return allMatch(stateMatchers, s) }) {
				continue
			}
		}
		if labelsCompatible(ruleLabels, matchers, am.generated, true) {
			return true
		}
	}
	return false
}

// labelsCompatible reports whether a selector can select a rule's output. Labels
// in generated are checked by the callers and skipped here. A statically set label
// must satisfy every matcher on it; any other value is produced at evaluation time
// and treated as unknown, so it is compatible unless the selector contradicts
// itself. Set expandsTemplates for rules whose labels are templated.
func labelsCompatible(ruleLabels labels.Labels, matchers []*labels.Matcher, generated []string, expandsTemplates bool) bool {
	seen := make(map[string]struct{}, len(matchers))
	for _, m := range matchers {
		if slices.Contains(generated, m.Name) {
			continue
		}
		if _, ok := seen[m.Name]; ok {
			continue
		}
		seen[m.Name] = struct{}{}

		ms := matchersFor(matchers, m.Name)
		if value, known := staticLabelValue(ruleLabels, m.Name, expandsTemplates); known {
			if !allMatch(ms, value) {
				return false
			}
			continue
		}
		if !matchersSatisfiable(ms) {
			return false
		}
	}
	return true
}

// staticLabelValue returns the value a rule sets for a label and whether that
// value is known before evaluation. Alerting rules expand their labels per alert
// instance, so a value carrying template syntax is not known here, which is the
// same reason AlertingRule.QueryForStateSeries skips those labels. A label the
// rule sets to the empty string is still known, and constrains the selector.
func staticLabelValue(ruleLabels labels.Labels, name string, expandsTemplates bool) (string, bool) {
	if !ruleLabels.Has(name) {
		return "", false
	}
	value := ruleLabels.Get(name)
	if expandsTemplates && strings.Contains(value, "{{") {
		return "", false
	}
	return value, true
}

// satisfiabilityProbes are cheap candidate label values, tried before anything is
// synthesized. The empty string doubles as the value of an absent label; the rest
// cover distinct character classes.
var satisfiabilityProbes = []string{"", "a", "A", "0", "-"}

// matchersSatisfiable reports whether some label value satisfies every matcher in
// ms. Values of a dynamic label are not known statically, so the question is
// answered by looking for a value that works: equality matchers and finite
// regexps offer one directly, and a required regexp can have one built from its
// syntax tree. A candidate is only accepted once allMatch confirms it, so a badly
// chosen one costs a missed attribution rather than a wrong one.
//
// Failing to find a value does not prove there is none, so this under-reports on
// matcher sets whose solutions all fall outside the bounded candidate set.
func matchersSatisfiable(ms []*labels.Matcher) bool {
	if slices.ContainsFunc(cheapWitnesses(ms), func(v string) bool { return allMatch(ms, v) }) {
		return true
	}
	// Synthesizing from the required expressions costs a parse, so it is only
	// reached once the cheap candidates are exhausted.
	for _, m := range ms {
		if m.Type != labels.MatchRegexp {
			continue
		}
		if slices.ContainsFunc(regexpCandidates(m.GetRegexString()), func(v string) bool { return allMatch(ms, v) }) {
			return true
		}
	}
	return false
}

// cheapWitnesses returns the candidate values that need no regexp parsing.
func cheapWitnesses(ms []*labels.Matcher) []string {
	out := make([]string, 0, len(ms)+len(satisfiabilityProbes)+1)
	for _, m := range ms {
		switch m.Type {
		case labels.MatchEqual:
			out = append(out, m.Value)
		case labels.MatchRegexp:
			out = append(out, m.SetMatches()...)
		}
	}
	return append(out, exclusionWitnesses(ms)...)
}

// Limits on the candidate search. regexpCandidateLimit caps how many values are
// built from one expression, so a nested alternation cannot blow it up;
// charClassCandidateLimit caps how many are taken from one character class.
const (
	regexpCandidateLimit    = 64
	charClassCandidateLimit = 4
)

// regexpCandidates returns values derived from the expression's syntax tree, for
// a caller to try against the matchers it came from. They are candidates rather
// than witnesses: a zero-width assertion can make one fail the expression it was
// built from, as the walk builds "foobar" for foo\bbar, which does not match it.
// Callers must confirm each value; matchersSatisfiable does.
//
// The flags are the ones labels.NewFastRegexMatcher parses with, so a candidate
// means the same here as it does when the matcher runs.
func regexpCandidates(expr string) []string {
	parsed, err := syntax.Parse(expr, syntax.Perl|syntax.DotNL)
	if err != nil {
		return nil
	}
	return regexpCandidatesFor(parsed.Simplify())
}

// regexpCandidatesFor returns values derived from re, taking every alternation
// branch and several members of each character class, since a later matcher may
// exclude the first one that comes to hand.
func regexpCandidatesFor(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		// Zero-width assertions add nothing to the value. Whether the assertion
		// holds where the surrounding parts put it is left to the caller's check.
		return []string{""}
	case syntax.OpNoMatch:
		return nil
	case syntax.OpLiteral:
		return []string{string(re.Rune)}
	case syntax.OpCharClass:
		return charClassCandidates(re.Rune)
	case syntax.OpAnyChar:
		return []string{"a", "b", "\n"}
	case syntax.OpAnyCharNotNL:
		return []string{"a", "b", "0"}
	case syntax.OpCapture:
		return regexpCandidatesFor(re.Sub[0])
	case syntax.OpQuest:
		return dedupeCandidates(append([]string{""}, regexpCandidatesFor(re.Sub[0])...))
	case syntax.OpStar:
		return repeatedCandidates(re.Sub[0], 0, 3)
	case syntax.OpPlus:
		return repeatedCandidates(re.Sub[0], 1, 3)
	case syntax.OpRepeat:
		maxRepeat := re.Min + 1
		if re.Max >= 0 && re.Max < maxRepeat {
			maxRepeat = re.Max
		}
		return repeatedCandidates(re.Sub[0], re.Min, maxRepeat)
	case syntax.OpConcat:
		lists := make([][]string, 0, len(re.Sub))
		for _, sub := range re.Sub {
			lists = append(lists, regexpCandidatesFor(sub))
		}
		return dedupeCandidates(productCandidates(lists))
	case syntax.OpAlternate:
		var out []string
		for _, sub := range re.Sub {
			out = append(out, regexpCandidatesFor(sub)...)
			if len(out) >= regexpCandidateLimit {
				break
			}
		}
		return dedupeCandidates(out)
	}
	return nil
}

// charClassCandidates returns values from a character class given as range pairs,
// taking both ends of each range so that a class spanning several ranges is not
// represented by its first character alone.
func charClassCandidates(runes []rune) []string {
	out := make([]string, 0, charClassCandidateLimit)
	for i := 0; i+1 < len(runes) && len(out) < charClassCandidateLimit; i += 2 {
		out = append(out, string(runes[i]))
		if runes[i+1] != runes[i] && len(out) < charClassCandidateLimit {
			out = append(out, string(runes[i+1]))
		}
	}
	return out
}

// repeatedCandidates returns the values re produces when repeated between
// minRepeat and maxRepeat times.
func repeatedCandidates(re *syntax.Regexp, minRepeat, maxRepeat int) []string {
	parts := regexpCandidatesFor(re)
	var out []string
	for n := minRepeat; n <= maxRepeat; n++ {
		lists := make([][]string, n)
		for i := range lists {
			lists[i] = parts
		}
		out = append(out, productCandidates(lists)...)
		if len(out) >= regexpCandidateLimit {
			break
		}
	}
	return dedupeCandidates(out)
}

// productCandidates returns the concatenations of one value from each list. A
// list with no values makes the whole product empty.
//
// The limit caps how wide the product gets at each step, never how many lists are
// applied: stopping part way would return values built from only the first few
// sub-expressions, which cannot match the whole thing.
func productCandidates(lists [][]string) []string {
	out := []string{""}
	for _, list := range lists {
		if len(list) == 0 {
			return nil
		}
		next := make([]string, 0, min(len(out)*len(list), regexpCandidateLimit))
		for _, prefix := range out {
			for _, part := range list {
				next = append(next, prefix+part)
				if len(next) >= regexpCandidateLimit {
					break
				}
			}
			if len(next) >= regexpCandidateLimit {
				break
			}
		}
		out = next
	}
	return out
}

// dedupeCandidates drops repeats and trims the list to the candidate limit.
func dedupeCandidates(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, min(len(values), regexpCandidateLimit))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
		if len(out) == regexpCandidateLimit {
			break
		}
	}
	return out
}

// exclusionWitnesses returns candidate values for a matcher set that only rules
// values out. Alongside the probes it offers a value stepped outside whatever the
// equality matchers exclude, so that a set excluding every probe is not mistaken
// for one that excludes everything.
func exclusionWitnesses(ms []*labels.Matcher) []string {
	excluded := make(map[string]struct{}, len(ms))
	for _, m := range ms {
		switch m.Type {
		case labels.MatchNotEqual:
			excluded[m.Value] = struct{}{}
		case labels.MatchNotRegexp:
			// A negated alternation of literals rules out exactly those values.
			for _, v := range m.SetMatches() {
				excluded[v] = struct{}{}
			}
		}
	}

	out := slices.Clone(satisfiabilityProbes)
	// Step outside the excluded set from several starting points, so that one
	// negative matcher cannot rule out every value this is willing to try.
	for _, seed := range freshWitnessSeeds {
		for i := 0; ; i++ {
			fresh := seed + strconv.Itoa(i)
			if _, ok := excluded[fresh]; !ok {
				out = append(out, fresh)
				break
			}
		}
	}
	return out
}

// freshWitnessSeeds are prefixes for values built to sit outside a matcher set's
// exclusions. They differ in shape rather than in spelling, since a negative
// regexp that rules out one shape often accepts another.
var freshWitnessSeeds = []string{"b", "Z", "9", "_", "-", ".", "-b", "b-", "coverage-witness-"}

// coverageStats are the exact rule counts a report is derived from. Thresholds
// are gated on these, never on the rounded percentage that is displayed.
type coverageStats struct {
	Covered int
	Total   int
}

// percentage returns covered/total as a percentage, treating an empty set as
// fully covered. For display only; use belowThreshold to gate.
func (s coverageStats) percentage() float64 {
	if s.Total == 0 {
		return 100
	}
	return float64(s.Covered) / float64(s.Total) * 100
}

// belowThreshold reports whether coverage is below threshold. It cross-multiplies
// rather than dividing, so one uncovered rule always fails a threshold of 100
// however large the total. Callers gate an empty set separately.
func (s coverageStats) belowThreshold(threshold float64) bool {
	if threshold <= 0 || s.Total == 0 {
		return false
	}
	return float64(s.Covered)*100 < threshold*float64(s.Total)
}

// reportAndGate writes the coverage summary to w and returns why the coverage
// gate failed, or the empty string when it passed. A threshold of zero or less
// only reports. Incomplete data never passes: rule files that never reached the
// totals, or an empty suite, mean coverage is unknown rather than complete.
func (c *ruleCoverage) reportAndGate(w io.Writer, threshold float64) string {
	stats := c.report(w)

	base := reportBaseDir()
	for _, file := range c.failureOrder {
		for _, err := range c.loadFailures[file] {
			fmt.Fprintf(w, "  WARNING: %s excluded from coverage: %s\n", displayRuleFilePath(base, file), err)
		}
	}

	if threshold <= 0 {
		return ""
	}
	switch {
	case len(c.loadFailures) > 0:
		return fmt.Sprintf("Cannot evaluate the rule coverage threshold: %d rule file(s) were not analysed.", len(c.loadFailures))
	case stats.Total == 0:
		return "Cannot evaluate the rule coverage threshold: no rules were loaded."
	case stats.belowThreshold(threshold):
		return fmt.Sprintf("Rule test coverage %d/%d (%s%%) is below the threshold of %s%%.",
			stats.Covered, stats.Total, formatCoverageAgainstThreshold(stats, threshold), formatThreshold(threshold))
	}
	return ""
}

// report writes the coverage summary to w and returns the overall statistics.
// Alerting and recording rules are reported separately.
func (c *ruleCoverage) report(w io.Writer) coverageStats {
	var alerting, recording coverageStats
	for _, k := range c.order {
		e := c.entries[k]
		s := &recording
		if e.kind == alertingRule {
			s = &alerting
		}
		s.Total++
		if e.covered {
			s.Covered++
		}
	}
	total := coverageStats{
		Covered: alerting.Covered + recording.Covered,
		Total:   alerting.Total + recording.Total,
	}

	fmt.Fprintln(w, "Rule test coverage:")
	fmt.Fprintf(w, "  Alerting rules:  %s\n", coverageFraction(alerting))
	fmt.Fprintf(w, "  Recording rules: %s\n", coverageFraction(recording))
	fmt.Fprintf(w, "  Total:           %s\n", coverageFraction(total))
	c.reportUncovered(w, total)

	return total
}

// reportUncovered lists to w the rules no assertion could be attributed to,
// grouped by rule file and rule group. A rule can appear here despite having a
// test, when the selector could not be attributed; see the limits in the docs.
func (c *ruleCoverage) reportUncovered(w io.Writer, total coverageStats) {
	if total.Covered == total.Total {
		return
	}
	uncovered := make([]ruleCoverageKey, 0, total.Total-total.Covered)
	for _, k := range c.order {
		if !c.entries[k].covered {
			uncovered = append(uncovered, k)
		}
	}
	slices.SortFunc(uncovered, func(a, b ruleCoverageKey) int {
		if n := strings.Compare(a.file, b.file); n != 0 {
			return n
		}
		if n := strings.Compare(a.group, b.group); n != 0 {
			return n
		}
		return a.ordinal - b.ordinal
	})

	base := reportBaseDir()
	fmt.Fprintln(w, "  Uncovered rules:")
	var lastFile, lastGroup string
	headerPrinted := false
	for _, k := range uncovered {
		if !headerPrinted || k.file != lastFile || k.group != lastGroup {
			fmt.Fprintf(w, "    group %q in %s:\n", k.group, displayRuleFilePath(base, k.file))
			lastFile, lastGroup = k.file, k.group
			headerPrinted = true
		}
		e := c.entries[k]
		// Labels do not always tell two declarations apart, so give the position
		// in the group as well; that is what identifies the rule.
		fmt.Fprintf(w, "      - %s: %s%s (rule #%d)\n", e.kind, e.name, labelSuffix(e.labels), k.ordinal+1)
	}
}

// labelSuffix renders static labels as a PromQL-like suffix, empty if there are none.
func labelSuffix(ls labels.Labels) string {
	if ls.IsEmpty() {
		return ""
	}
	return ls.String()
}

// reportBaseDir returns the canonicalized directory rule file paths are reported
// relative to, or "" if it cannot be determined.
func reportBaseDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(wd); err == nil {
		return resolved
	}
	return wd
}

// displayRuleFilePath renders file relative to base, or absolute if it lies
// outside. Enough path is kept for files sharing a base name to stay distinct.
func displayRuleFilePath(base, file string) string {
	if base == "" {
		return file
	}
	rel, err := filepath.Rel(base, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return file
	}
	return rel
}

// coverageFraction formats a covered/total count together with its percentage.
// A count with no rules is reported without a percentage.
func coverageFraction(s coverageStats) string {
	if s.Total == 0 {
		return "0/0 covered"
	}
	return fmt.Sprintf("%d/%d covered (%s%%)", s.Covered, s.Total, formatCoveragePercentage(s))
}

// formatCoveragePercentage renders a percentage at one decimal place, adding
// decimals when that would round an incomplete result up to 100% or a partly
// covered one down to 0%, contradicting the counts beside it.
func formatCoveragePercentage(s coverageStats) string {
	pct := s.percentage()
	const maxPrec = 6
	for prec := 1; prec < maxPrec; prec++ {
		out := strconv.FormatFloat(pct, 'f', prec, 64)
		rounded, err := strconv.ParseFloat(out, 64)
		if err != nil {
			break
		}
		if (rounded < 100 || s.Covered == s.Total) && (rounded > 0 || s.Covered == 0) {
			return out
		}
	}
	return strconv.FormatFloat(pct, 'f', maxPrec, 64)
}

// formatCoverageAgainstThreshold renders a coverage percentage with enough
// precision to tell it apart from the threshold it failed, so that a message
// about 2 rules of 3 does not read as 66.7% being below 66.7%.
func formatCoverageAgainstThreshold(s coverageStats, threshold float64) string {
	out := formatCoveragePercentage(s)
	if out != formatThreshold(threshold) {
		return out
	}
	for prec := 2; prec <= 8; prec++ {
		if longer := strconv.FormatFloat(s.percentage(), 'f', prec, 64); longer != formatThreshold(threshold) {
			return longer
		}
	}
	return out
}

// formatThreshold renders a threshold percentage without trailing zeros, so that
// a message about a threshold of 60.04 does not report it as 60.0.
func formatThreshold(threshold float64) string {
	return strconv.FormatFloat(threshold, 'g', -1, 64)
}
