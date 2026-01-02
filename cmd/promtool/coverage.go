// Copyright 2024 The Prometheus Authors
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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/junitxml"
)

// RuleCoverageTracker tracks test coverage for Prometheus rules.
type RuleCoverageTracker struct {
	mu           sync.RWMutex
	rules        map[string]*RuleInfo
	testedRules  map[string]bool
	testDetails  map[string]*TestDetails
	dependencies map[string][]string
	config       *CoverageConfig
}

// RuleInfo contains information about a rule.
type RuleInfo struct {
	Name        string            `json:"name"`
	Group       string            `json:"group"`
	Type        string            `json:"type"` // "alert" or "record"
	File        string            `json:"file"`
	Line        int               `json:"line,omitempty"`
	Expression  string            `json:"expression"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// TestDetails contains information about how a rule was tested.
type TestDetails struct {
	RuleName     string    `json:"rule_name"`
	TestFiles    []string  `json:"test_files"`
	TestCount    int       `json:"test_count"`
	DirectTest   bool      `json:"direct_test"`
	IndirectTest bool      `json:"indirect_test"`
	LastTested   time.Time `json:"last_tested"`
}

// CoverageConfig contains configuration for coverage tracking.
type CoverageConfig struct {
	EnableDependencyAnalysis bool     `json:"enable_dependency_analysis"`
	MinCoverage              float64  `json:"min_coverage"`
	FailOnUntested           bool     `json:"fail_on_untested"`
	IgnorePatterns           []string `json:"ignore_patterns,omitempty"`
	OutputFormat             string   `json:"output_format"`
	OutputFile               string   `json:"output_file,omitempty"`
	ByGroup                  bool     `json:"by_group"`
}

// CoverageReport contains the coverage analysis results.
type CoverageReport struct {
	Timestamp        time.Time                 `json:"timestamp"`
	Summary          CoverageSummary           `json:"summary"`
	ByType           map[string]*CoverageStats `json:"by_type,omitempty"`
	ByGroup          map[string]*CoverageStats `json:"by_group,omitempty"`
	UntestedRules    []*RuleInfo               `json:"untested_rules"`
	IndirectlyTested []*RuleInfo               `json:"indirectly_tested,omitempty"`
	Details          map[string]*TestDetails   `json:"test_details,omitempty"`
}

// CoverageSummary contains overall coverage statistics.
type CoverageSummary struct {
	TotalRules      int     `json:"total_rules"`
	TestedRules     int     `json:"tested_rules"`
	UntestedRules   int     `json:"untested_rules"`
	CoveragePercent float64 `json:"coverage_percent"`
	AlertCoverage   float64 `json:"alert_coverage,omitempty"`
	RecordCoverage  float64 `json:"record_coverage,omitempty"`
}

// CoverageStats contains coverage statistics for a category.
type CoverageStats struct {
	Total      int     `json:"total"`
	Tested     int     `json:"tested"`
	Percentage float64 `json:"percentage"`
}

// NewRuleCoverageTracker creates a new coverage tracker.
func NewRuleCoverageTracker(config *CoverageConfig) *RuleCoverageTracker {
	if config == nil {
		config = &CoverageConfig{
			OutputFormat: "text",
		}
	}
	return &RuleCoverageTracker{
		rules:        make(map[string]*RuleInfo),
		testedRules:  make(map[string]bool),
		testDetails:  make(map[string]*TestDetails),
		dependencies: make(map[string][]string),
		config:       config,
	}
}

// LoadRulesFromFiles loads rules from the specified files.
func (rct *RuleCoverageTracker) LoadRulesFromFiles(ruleFiles []string) error {
	rct.mu.Lock()
	defer rct.mu.Unlock()

	for _, file := range ruleFiles {
		if err := rct.loadRuleFile(file); err != nil {
			return fmt.Errorf("error loading rule file %s: %w", file, err)
		}
	}

	if rct.config.EnableDependencyAnalysis {
		rct.analyzeDependencies()
	}

	return nil
}

// loadRuleFile loads rules from a single file.
func (rct *RuleCoverageTracker) loadRuleFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var ruleGroups rulefmt.RuleGroups
	if err := yaml.UnmarshalStrict(content, &ruleGroups); err != nil {
		return err
	}

	for _, group := range ruleGroups.Groups {
		for i, rule := range group.Rules {
			ruleID := rct.generateRuleID(group.Name, rule)

			ruleInfo := &RuleInfo{
				Group:       group.Name,
				File:        filepath.Clean(filename),
				Line:        i + 1, // Approximate line number
				Expression:  rule.Expr,
				Labels:      rule.Labels,
				Annotations: rule.Annotations,
			}

			switch {
			case rule.Alert != "":
				ruleInfo.Name = rule.Alert
				ruleInfo.Type = "alert"
			case rule.Record != "":
				ruleInfo.Name = rule.Record
				ruleInfo.Type = "record"
			default:
				continue // Skip invalid rules
			}

			rct.rules[ruleID] = ruleInfo
		}
	}

	return nil
}

// generateRuleID creates a unique identifier for a rule.
func (*RuleCoverageTracker) generateRuleID(groupName string, rule rulefmt.Rule) string {
	if rule.Alert != "" {
		return fmt.Sprintf("%s/%s", groupName, rule.Alert)
	}
	if rule.Record != "" {
		return fmt.Sprintf("%s/%s", groupName, rule.Record)
	}
	return ""
}

// MarkRuleTested marks a rule as tested.
func (rct *RuleCoverageTracker) MarkRuleTested(ruleName, testFile string, isDirect bool) {
	rct.mu.Lock()
	defer rct.mu.Unlock()

	// Try to find the rule in any group
	var ruleID string
	for id, rule := range rct.rules {
		if rule.Name == ruleName || strings.HasSuffix(id, "/"+ruleName) {
			ruleID = id
			break
		}
	}

	if ruleID == "" {
		// Rule not found, might be testing a non-existent rule
		return
	}

	rct.testedRules[ruleID] = true

	if _, exists := rct.testDetails[ruleID]; !exists {
		rct.testDetails[ruleID] = &TestDetails{
			RuleName:   ruleName,
			TestFiles:  []string{},
			LastTested: time.Now(),
		}
	}

	details := rct.testDetails[ruleID]
	details.TestCount++
	if !contains(details.TestFiles, testFile) {
		details.TestFiles = append(details.TestFiles, testFile)
	}

	if isDirect {
		details.DirectTest = true
	} else {
		details.IndirectTest = true
	}
	details.LastTested = time.Now()
}

// MarkExpressionTested analyzes an expression and marks related rules as tested.
func (rct *RuleCoverageTracker) MarkExpressionTested(expr, testFile string) {
	rct.mu.Lock()
	defer rct.mu.Unlock()

	// Parse the expression to find referenced metrics
	parsedExpr, err := parser.ParseExpr(expr)
	if err != nil {
		return
	}

	// Extract metric names from the expression
	metrics := rct.extractMetricsFromExpr(parsedExpr)

	// Check if any metrics match recording rules
	for _, metric := range metrics {
		for ruleID, rule := range rct.rules {
			if rule.Type == "record" && rule.Name == metric {
				rct.testedRules[ruleID] = true

				if _, exists := rct.testDetails[ruleID]; !exists {
					rct.testDetails[ruleID] = &TestDetails{
						RuleName:     rule.Name,
						TestFiles:    []string{},
						IndirectTest: true,
						LastTested:   time.Now(),
					}
				}

				details := rct.testDetails[ruleID]
				details.TestCount++
				if !contains(details.TestFiles, testFile) {
					details.TestFiles = append(details.TestFiles, testFile)
				}
				details.IndirectTest = true
				details.LastTested = time.Now()
			}
		}
	}
}

// extractMetricsFromExpr extracts metric names from a PromQL expression.
func (*RuleCoverageTracker) extractMetricsFromExpr(expr parser.Expr) []string {
	metrics := []string{}

	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		if n, ok := node.(*parser.VectorSelector); ok {
			metrics = append(metrics, n.Name)
		}
		if n, ok := node.(*parser.MatrixSelector); ok {
			if vs, ok := n.VectorSelector.(*parser.VectorSelector); ok {
				metrics = append(metrics, vs.Name)
			}
		}
		return nil
	})

	return metrics
}

// analyzeDependencies analyzes rule dependencies.
func (rct *RuleCoverageTracker) analyzeDependencies() {
	for ruleID, rule := range rct.rules {
		if rule.Expression == "" {
			continue
		}

		parsedExpr, err := parser.ParseExpr(rule.Expression)
		if err != nil {
			continue
		}

		metrics := rct.extractMetricsFromExpr(parsedExpr)
		var deps []string

		for _, metric := range metrics {
			for depID, depRule := range rct.rules {
				if depRule.Type == "record" && depRule.Name == metric {
					deps = append(deps, depID)
				}
			}
		}

		if len(deps) > 0 {
			rct.dependencies[ruleID] = deps
		}
	}
}

// GenerateReport generates a coverage report.
func (rct *RuleCoverageTracker) GenerateReport() *CoverageReport {
	rct.mu.RLock()
	defer rct.mu.RUnlock()

	report := &CoverageReport{
		Timestamp:     time.Now(),
		UntestedRules: []*RuleInfo{},
		Details:       rct.testDetails,
	}

	// Count totals
	totalRules := len(rct.rules)
	testedRules := len(rct.testedRules)
	untestedRules := totalRules - testedRules

	// Calculate coverage percentage
	coveragePercent := 0.0
	if totalRules > 0 {
		coveragePercent = (float64(testedRules) / float64(totalRules)) * 100
	}

	// Calculate coverage by type
	var alertTotal, alertTested, recordTotal, recordTested int
	byType := make(map[string]*CoverageStats)

	for ruleID, rule := range rct.rules {
		switch rule.Type {
		case "alert":
			alertTotal++
			if rct.testedRules[ruleID] {
				alertTested++
			}
		case "record":
			recordTotal++
			if rct.testedRules[ruleID] {
				recordTested++
			}
		}

		// Collect untested rules
		if !rct.testedRules[ruleID] && !rct.shouldIgnoreRule(rule) {
			report.UntestedRules = append(report.UntestedRules, rule)
		}
	}

	// Calculate percentages by type
	if alertTotal > 0 {
		byType["alert"] = &CoverageStats{
			Total:      alertTotal,
			Tested:     alertTested,
			Percentage: (float64(alertTested) / float64(alertTotal)) * 100,
		}
	}
	if recordTotal > 0 {
		byType["record"] = &CoverageStats{
			Total:      recordTotal,
			Tested:     recordTested,
			Percentage: (float64(recordTested) / float64(recordTotal)) * 100,
		}
	}
	report.ByType = byType

	// Calculate coverage by group if requested
	if rct.config.ByGroup {
		byGroup := make(map[string]*CoverageStats)
		groupTotals := make(map[string]int)
		groupTested := make(map[string]int)

		for ruleID, rule := range rct.rules {
			groupTotals[rule.Group]++
			if rct.testedRules[ruleID] {
				groupTested[rule.Group]++
			}
		}

		for group, total := range groupTotals {
			tested := groupTested[group]
			percentage := 0.0
			if total > 0 {
				percentage = (float64(tested) / float64(total)) * 100
			}
			byGroup[group] = &CoverageStats{
				Total:      total,
				Tested:     tested,
				Percentage: percentage,
			}
		}
		report.ByGroup = byGroup
	}

	// Sort untested rules for consistent output
	sort.Slice(report.UntestedRules, func(i, j int) bool {
		if report.UntestedRules[i].Group != report.UntestedRules[j].Group {
			return report.UntestedRules[i].Group < report.UntestedRules[j].Group
		}
		return report.UntestedRules[i].Name < report.UntestedRules[j].Name
	})

	// Set summary
	report.Summary = CoverageSummary{
		TotalRules:      totalRules,
		TestedRules:     testedRules,
		UntestedRules:   untestedRules,
		CoveragePercent: coveragePercent,
	}

	if byType["alert"] != nil {
		report.Summary.AlertCoverage = byType["alert"].Percentage
	}
	if byType["record"] != nil {
		report.Summary.RecordCoverage = byType["record"].Percentage
	}

	return report
}

// shouldIgnoreRule checks if a rule should be ignored based on patterns.
func (rct *RuleCoverageTracker) shouldIgnoreRule(rule *RuleInfo) bool {
	if len(rct.config.IgnorePatterns) == 0 {
		return false
	}

	fullName := fmt.Sprintf("%s/%s", rule.Group, rule.Name)
	for _, pattern := range rct.config.IgnorePatterns {
		if matched, _ := filepath.Match(pattern, fullName); matched {
			return true
		}
	}
	return false
}

// WriteReport writes the coverage report in the specified format.
func (rct *RuleCoverageTracker) WriteReport(report *CoverageReport) error {
	var output string
	var err error

	switch rct.config.OutputFormat {
	case "json":
		output, err = rct.formatJSON(report)
	case "junit", "junit-xml":
		output, err = rct.formatJUnitXML(report)
	case "text", "":
		output = rct.formatText(report)
	default:
		return fmt.Errorf("unsupported output format: %s", rct.config.OutputFormat)
	}

	if err != nil {
		return err
	}

	// Write to file or stdout
	if rct.config.OutputFile != "" {
		return os.WriteFile(rct.config.OutputFile, []byte(output), 0o644)
	}

	fmt.Print(output)
	return nil
}

// formatText formats the report as human-readable text.
func (rct *RuleCoverageTracker) formatText(report *CoverageReport) string {
	var sb strings.Builder

	// Header
	sb.WriteString("\n📊 Prometheus Rules Test Coverage Report\n")
	sb.WriteString(strings.Repeat("═", 60) + "\n\n")

	// Overall coverage
	coverageColor := rct.getColorForCoverage(report.Summary.CoveragePercent)
	sb.WriteString(fmt.Sprintf("Overall Coverage: %s%.1f%%%s (%d/%d rules)\n",
		coverageColor, report.Summary.CoveragePercent, colorReset,
		report.Summary.TestedRules, report.Summary.TotalRules))

	// Coverage by type
	if len(report.ByType) > 0 {
		sb.WriteString("\n📈 Coverage by Type:\n")
		for ruleType, stats := range report.ByType {
			typeStr := cases.Title(language.Und).String(ruleType) + " Rules"
			sb.WriteString(fmt.Sprintf("  ├─ %-15s: %.1f%% (%d/%d)\n",
				typeStr, stats.Percentage, stats.Tested, stats.Total))
		}
	}

	// Coverage by group
	if len(report.ByGroup) > 0 {
		sb.WriteString("\n📁 Coverage by Group:\n")

		// Sort groups for consistent output
		var groups []string
		for group := range report.ByGroup {
			groups = append(groups, group)
		}
		sort.Strings(groups)

		for _, group := range groups {
			stats := report.ByGroup[group]
			sb.WriteString(fmt.Sprintf("  ├─ %-20s: %.1f%% (%d/%d)\n",
				group, stats.Percentage, stats.Tested, stats.Total))
		}
	}

	// Untested rules
	if len(report.UntestedRules) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️  Untested Rules (%d):\n", len(report.UntestedRules)))
		for _, rule := range report.UntestedRules {
			sb.WriteString(fmt.Sprintf("  • %s/%s [%s] %s:%d\n",
				rule.Group, rule.Name, rule.Type, rule.File, rule.Line))
		}
	} else {
		sb.WriteString("\n✅ All rules have test coverage!\n")
	}

	// Footer
	sb.WriteString(fmt.Sprintf("\nGenerated at: %s\n", report.Timestamp.Format(time.RFC3339)))

	return sb.String()
}

// formatJSON formats the report as JSON.
func (*RuleCoverageTracker) formatJSON(report *CoverageReport) (string, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// formatJUnitXML formats the report as JUnit XML.
func (rct *RuleCoverageTracker) formatJUnitXML(report *CoverageReport) (string, error) {
	junit := &junitxml.JUnitXML{}

	// Create a test suite for coverage
	suite := junit.Suite("coverage-report")
	suite.Settime(report.Timestamp.Format("2006-01-02T15:04:05"))

	// Add properties for coverage metrics
	// Note: Properties should be added to the suite through a proper method
	// This is a simplified version - actual implementation would need to extend junitxml

	// Add test cases for each rule
	for ruleID, rule := range rct.rules {
		tc := &junitxml.TestCase{
			Name: fmt.Sprintf("%s/%s", rule.Group, rule.Name),
		}

		if !rct.testedRules[ruleID] {
			// Untested rules are marked as failures
			tc.Failures = append(tc.Failures, "No test coverage found for this rule")
			suite.FailureCount++
		}

		suite.Cases = append(suite.Cases, tc)
		suite.TestCount++
	}

	// Generate XML
	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<testsuites name="promtool-coverage">` + "\n")

	// Add coverage summary as properties
	buf.WriteString(fmt.Sprintf(`  <testsuite name="coverage-report" tests="%d" failures="%d" time="0" timestamp="%s">`+"\n",
		suite.TestCount, suite.FailureCount, suite.Timestamp))

	buf.WriteString(`    <properties>` + "\n")
	buf.WriteString(fmt.Sprintf(`      <property name="coverage.overall" value="%.2f"/>`+"\n", report.Summary.CoveragePercent))
	buf.WriteString(fmt.Sprintf(`      <property name="coverage.alerts" value="%.2f"/>`+"\n", report.Summary.AlertCoverage))
	buf.WriteString(fmt.Sprintf(`      <property name="coverage.records" value="%.2f"/>`+"\n", report.Summary.RecordCoverage))
	buf.WriteString(`    </properties>` + "\n")

	// Add test cases
	for _, tc := range suite.Cases {
		if len(tc.Failures) > 0 {
			buf.WriteString(fmt.Sprintf(`    <testcase name="%s" classname="rule">`+"\n", tc.Name))
			buf.WriteString(fmt.Sprintf(`      <failure message="%s"/>`+"\n", tc.Failures[0]))
			buf.WriteString(`    </testcase>` + "\n")
		} else {
			buf.WriteString(fmt.Sprintf(`    <testcase name="%s" classname="rule"/>`+"\n", tc.Name))
		}
	}

	buf.WriteString(`  </testsuite>` + "\n")
	buf.WriteString(`</testsuites>` + "\n")

	return buf.String(), nil
}

// getColorForCoverage returns ANSI color code based on coverage percentage.
func (*RuleCoverageTracker) getColorForCoverage(percentage float64) string {
	switch {
	case percentage >= 80:
		return "\033[32m" // Green
	case percentage >= 50:
		return "\033[33m" // Yellow
	default:
		return "\033[31m" // Red
	}
}

const colorReset = "\033[0m"

// CheckCoverageThreshold checks if coverage meets the minimum threshold.
func (rct *RuleCoverageTracker) CheckCoverageThreshold(report *CoverageReport) error {
	if rct.config.MinCoverage > 0 && report.Summary.CoveragePercent < rct.config.MinCoverage {
		return fmt.Errorf("coverage %.1f%% is below minimum threshold %.1f%%",
			report.Summary.CoveragePercent, rct.config.MinCoverage)
	}

	if rct.config.FailOnUntested && len(report.UntestedRules) > 0 {
		return fmt.Errorf("found %d untested rules", len(report.UntestedRules))
	}

	return nil
}

// Helper function to check if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
