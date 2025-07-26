// Copyright 2023 The Prometheus Authors
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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/util/junitxml"
)

// TestResult represents the result of an individual test.
type TestResult struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration_ms"`
	Error    string        `json:"error,omitempty"`
}

// CoverageTracker tracks the coverage of rules.
type CoverageTracker struct {
	rules       map[string]map[string]struct{} // groupName -> ruleName -> struct{}
	untested    map[string]map[string]struct{} // groupName -> ruleName -> struct{}
	ruleFiles   map[string][]string            // fileName -> []groupName
	testResults []TestResult
}

// NewCoverageTracker creates a new CoverageTracker.
func NewCoverageTracker() *CoverageTracker {
	return &CoverageTracker{
		rules:     make(map[string]map[string]struct{}),
		untested:  make(map[string]map[string]struct{}),
		ruleFiles: make(map[string][]string),
	}
}

// AddRule adds a rule to the tracker.
func (t *CoverageTracker) AddRule(groupName, ruleName string) {
	if _, ok := t.rules[groupName]; !ok {
		t.rules[groupName] = make(map[string]struct{})
		t.untested[groupName] = make(map[string]struct{})
	}
	t.rules[groupName][ruleName] = struct{}{}
	t.untested[groupName][ruleName] = struct{}{}
}

// AddRuleFile adds a rule file and its groups to the tracker.
func (t *CoverageTracker) AddRuleFile(fileName string, groupNames []string) {
	t.ruleFiles[fileName] = groupNames
}

// MarkRuleAsTested marks a rule as tested.
func (t *CoverageTracker) MarkRuleAsTested(ruleName string) {
	for groupName, rules := range t.untested {
		if _, ok := rules[ruleName]; ok {
			delete(t.untested[groupName], ruleName)
		}
	}
}

// MarkRuleAsTestedBySelector marks rules as tested based on a set of label matchers.
func (t *CoverageTracker) MarkRuleAsTestedBySelector(groups []*rules.Group, matchers []*labels.Matcher) {
	for _, group := range groups {
		for _, rule := range group.Rules() {
			if t.ruleMatchesSelector(rule, matchers) {
				t.MarkRuleAsTested(rule.Name())
			}
		}
	}
}

// ruleMatchesSelector checks if a rule matches a set of label matchers.
func (t *CoverageTracker) ruleMatchesSelector(rule rules.Rule, matchers []*labels.Matcher) bool {
	// Check __name__ label first.
	for _, matcher := range matchers {
		if matcher.Name == labels.MetricName {
			// Use the matcher to test against the rule name
			if matcher.Matches(rule.Name()) {
				return true
			}
		}
		// Also check other labels associated with the rule.
		if ruleLabels := rule.Labels(); ruleLabels.Len() > 0 {
			if labelValue := ruleLabels.Get(matcher.Name); labelValue != "" {
				if matcher.Matches(labelValue) {
					return true
				}
			}
		}
	}
	return false
}

// AddTestResult adds a test result to the tracker.
func (t *CoverageTracker) AddTestResult(result TestResult) {
	t.testResults = append(t.testResults, result)
}

// PrintCoverageReport prints the coverage report to the specified output file.
func (t *CoverageTracker) PrintCoverageReport(w io.Writer, outputFile, outputFormat string) error {
	var (
		totalRules      int
		totalUntested   int
		untestedRules   = make(map[string][]string)
		coveragePercent float64
	)

	for groupName, rules := range t.rules {
		totalRules += len(rules)
		untestedCount := len(t.untested[groupName])
		totalUntested += untestedCount
		if untestedCount > 0 {
			untestedRules[groupName] = make([]string, 0, untestedCount)
			for ruleName := range t.untested[groupName] {
				untestedRules[groupName] = append(untestedRules[groupName], ruleName)
			}
			sort.Strings(untestedRules[groupName])
		}
	}

	if totalRules > 0 {
		coveragePercent = float64(totalRules-totalUntested) / float64(totalRules) * 100
	}

	var (
		report []byte
		err    error
	)

	switch outputFormat {
	case "json":
		if len(t.testResults) > 0 {
			report, err = t.generateJSONReportWithTestResults(coveragePercent, totalRules, totalUntested, untestedRules, t.testResults)
		} else {
			report, err = t.generateJSONReport(coveragePercent, totalRules, totalUntested, untestedRules)
		}
	case "junit-xml":
		// JUnit XML is handled by the junitxml package. We just add the coverage
		// as a property to the test suite.
		return nil
	default: // text
		report = t.generateTextReport(coveragePercent, totalRules, totalUntested, untestedRules)
	}

	if err != nil {
		return err
	}

	if outputFile != "" {
		return os.WriteFile(outputFile, report, 0o644)
	}

	_, err = w.Write(report)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

func (t *CoverageTracker) generateTextReport(coveragePercent float64, totalRules, totalUntested int, untestedRules map[string][]string) []byte {
	var b strings.Builder
	// Print file-level coverage
	if len(t.ruleFiles) > 0 {
		b.WriteString("Rule file coverage:\n")
		for fileName, groupNames := range t.ruleFiles {
			fileRules := 0
			fileUntested := 0
			for _, groupName := range groupNames {
				if rules, ok := t.rules[groupName]; ok {
					fileRules += len(rules)
				}
				if untestedInGroup, ok := t.untested[groupName]; ok {
					fileUntested += len(untestedInGroup)
				}
			}
			fileCoverage := float64(0)
			if fileRules > 0 {
				fileCoverage = float64(fileRules-fileUntested) / float64(fileRules) * 100
			}
			b.WriteString(fmt.Sprintf("  %s: %.0f%% rule coverage (%d/%d rules tested)\n",
				fileName, fileCoverage, fileRules-fileUntested, fileRules))
		}
	}

	b.WriteString(fmt.Sprintf("Overall coverage: %.0f%% (%d/%d rules tested)\n",
		coveragePercent, totalRules-totalUntested, totalRules))

	if totalUntested > 0 {
		b.WriteString("\nUntested rules:\n")
		for groupName, rules := range untestedRules {
			if len(rules) > 0 {
				b.WriteString(fmt.Sprintf("  %s:\n", groupName))
				for _, ruleName := range rules {
					b.WriteString(fmt.Sprintf("    - %s\n", ruleName))
				}
			}
		}
	}
	return []byte(b.String())
}

func (t *CoverageTracker) generateJSONReportWithTestResults(coveragePercent float64, totalRules, totalUntested int, _ map[string][]string, testResults []TestResult) ([]byte, error) {
	// Calculate file-level coverage
	fileCoverage := make(map[string]interface{})
	for fileName, groupNames := range t.ruleFiles {
		fileRules := 0
		fileUntested := 0
		testedRules := []string{}
		untestedRulesList := []string{}

		for _, groupName := range groupNames {
			if rules, ok := t.rules[groupName]; ok {
				for ruleName := range rules {
					fileRules++
					if _, untested := t.untested[groupName][ruleName]; untested {
						fileUntested++
						untestedRulesList = append(untestedRulesList, ruleName)
					} else {
						testedRules = append(testedRules, ruleName)
					}
				}
			}
		}

		fileCoveragePercent := float64(0)
		if fileRules > 0 {
			fileCoveragePercent = float64(fileRules-fileUntested) / float64(fileRules) * 100
		}

		sort.Strings(testedRules)
		sort.Strings(untestedRulesList)

		fileCoverage[fileName] = map[string]interface{}{
			"coverage":       fileCoveragePercent,
			"tested_rules":   testedRules,
			"untested_rules": untestedRulesList,
		}
	}

	report := struct {
		OverallCoverage float64                `json:"overall_coverage"`
		TotalRules      int                    `json:"total_rules"`
		TestedRules     int                    `json:"tested_rules"`
		Files           map[string]interface{} `json:"files"`
		TestResults     []TestResult           `json:"test_results"`
	}{
		OverallCoverage: coveragePercent,
		TotalRules:      totalRules,
		TestedRules:     totalRules - totalUntested,
		Files:           fileCoverage,
		TestResults:     testResults,
	}
	return json.MarshalIndent(report, "", "  ")
}

func (t *CoverageTracker) generateJSONReport(coveragePercent float64, totalRules, totalUntested int, _ map[string][]string) ([]byte, error) {
	// Calculate file-level coverage
	fileCoverage := make(map[string]interface{})
	for fileName, groupNames := range t.ruleFiles {
		fileRules := 0
		fileUntested := 0
		testedRules := []string{}
		untestedRulesList := []string{}

		for _, groupName := range groupNames {
			if rules, ok := t.rules[groupName]; ok {
				for ruleName := range rules {
					fileRules++
					if _, untested := t.untested[groupName][ruleName]; untested {
						fileUntested++
						untestedRulesList = append(untestedRulesList, ruleName)
					} else {
						testedRules = append(testedRules, ruleName)
					}
				}
			}
		}

		fileCoveragePercent := float64(0)
		if fileRules > 0 {
			fileCoveragePercent = float64(fileRules-fileUntested) / float64(fileRules) * 100
		}

		sort.Strings(testedRules)
		sort.Strings(untestedRulesList)

		fileCoverage[fileName] = map[string]interface{}{
			"coverage":       fileCoveragePercent,
			"tested_rules":   testedRules,
			"untested_rules": untestedRulesList,
		}
	}

	report := struct {
		OverallCoverage float64                `json:"overall_coverage"`
		TotalRules      int                    `json:"total_rules"`
		TestedRules     int                    `json:"tested_rules"`
		Files           map[string]interface{} `json:"files"`
	}{
		OverallCoverage: coveragePercent,
		TotalRules:      totalRules,
		TestedRules:     totalRules - totalUntested,
		Files:           fileCoverage,
	}
	return json.MarshalIndent(report, "", "  ")
}

func (t *CoverageTracker) AddCoverageToJUnit(ts *junitxml.TestSuite) {
	var totalRules, totalUntested int
	for _, rules := range t.rules {
		totalRules += len(rules)
	}
	for _, rules := range t.untested {
		totalUntested += len(rules)
	}

	if totalRules > 0 {
		coveragePercent := float64(totalRules-totalUntested) / float64(totalRules) * 100
		ts.AddProperty("coverage", fmt.Sprintf("%.2f%%", coveragePercent))
	}
}

// GetCoveragePercent returns the overall coverage percentage.
func (t *CoverageTracker) GetCoveragePercent() float64 {
	var totalRules, totalUntested int
	for _, rules := range t.rules {
		totalRules += len(rules)
	}
	for _, rules := range t.untested {
		totalUntested += len(rules)
	}

	if totalRules == 0 {
		return 100.0 // No rules means 100% coverage
	}
	return float64(totalRules-totalUntested) / float64(totalRules) * 100
}

// CheckCoverageThreshold returns true if coverage meets the specified threshold.
func (t *CoverageTracker) CheckCoverageThreshold(threshold float64) bool {
	return t.GetCoveragePercent() >= threshold
}
