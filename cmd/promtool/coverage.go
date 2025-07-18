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
	"os"
	"sort"
	"strings"

	"github.com/prometheus/prometheus/util/junitxml"
)

// CoverageTracker tracks the coverage of rules.
type CoverageTracker struct {
	rules    map[string]map[string]struct{}
	untested map[string]map[string]struct{}
}

// NewCoverageTracker creates a new CoverageTracker.
func NewCoverageTracker() *CoverageTracker {
	return &CoverageTracker{
		rules:    make(map[string]map[string]struct{}),
		untested: make(map[string]map[string]struct{}),
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

// MarkRuleAsTested marks a rule as tested.
func (t *CoverageTracker) MarkRuleAsTested(ruleName string) {
	for groupName, rules := range t.untested {
		if _, ok := rules[ruleName]; ok {
			delete(t.untested[groupName], ruleName)
		}
	}
}

// PrintCoverageReport prints the coverage report to the specified output file.
func (t *CoverageTracker) PrintCoverageReport(outputFile, outputFormat string) error {
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
		report, err = t.generateJSONReport(coveragePercent, totalRules, totalUntested, untestedRules)
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

	fmt.Println(string(report))
	return nil
}

func (t *CoverageTracker) generateTextReport(coveragePercent float64, totalRules, totalUntested int, untestedRules map[string][]string) []byte {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Coverage: %.2f%%\n", coveragePercent))
	b.WriteString(fmt.Sprintf("Total rules: %d\n", totalRules))
	b.WriteString(fmt.Sprintf("Untested rules: %d\n", totalUntested))
	if totalUntested > 0 {
		b.WriteString("Untested rules:\n")
		for groupName, rules := range untestedRules {
			b.WriteString(fmt.Sprintf("  %s:\n", groupName))
			for _, ruleName := range rules {
				b.WriteString(fmt.Sprintf("    - %s\n", ruleName))
			}
		}
	}
	return []byte(b.String())
}

func (t *CoverageTracker) generateJSONReport(coveragePercent float64, totalRules, totalUntested int, untestedRules map[string][]string) ([]byte, error) {
	report := struct {
		CoveragePercent float64             `json:"coveragePercent"`
		TotalRules      int                 `json:"totalRules"`
		UntestedRules   int                 `json:"untestedRules"`
		Untested        map[string][]string `json:"untested,omitempty"`
	}{
		CoveragePercent: coveragePercent,
		TotalRules:      totalRules,
		UntestedRules:   totalUntested,
		Untested:        untestedRules,
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
