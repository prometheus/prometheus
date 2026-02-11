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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/rulefmt"
)

func TestRuleCoverageTracker(t *testing.T) {
	// Create test rule files
	testRulesContent := `
groups:
  - name: test_alerts
    rules:
      - alert: HighCPU
        expr: cpu_usage > 0.8
        labels:
          severity: warning
      - alert: LowMemory
        expr: memory_available < 100
        labels:
          severity: critical
  - name: test_records
    rules:
      - record: cpu:rate5m
        expr: rate(cpu_usage[5m])
      - record: memory:usage
        expr: 1 - (memory_available / memory_total)
`

	// Create temporary test directory
	tmpDir := t.TempDir()
	ruleFile := filepath.Join(tmpDir, "rules.yml")
	err := os.WriteFile(ruleFile, []byte(testRulesContent), 0o644)
	require.NoError(t, err)

	t.Run("load rules from files", func(t *testing.T) {
		tracker := NewRuleCoverageTracker(nil)
		err := tracker.LoadRulesFromFiles([]string{ruleFile})
		require.NoError(t, err)
		require.Len(t, tracker.rules, 4)
	})

	t.Run("mark rule tested", func(t *testing.T) {
		tracker := NewRuleCoverageTracker(nil)
		err := tracker.LoadRulesFromFiles([]string{ruleFile})
		require.NoError(t, err)

		tracker.MarkRuleTested("HighCPU", "test.yml", true)
		require.True(t, tracker.testedRules["test_alerts/HighCPU"])
	})

	t.Run("generate coverage report", func(t *testing.T) {
		tracker := NewRuleCoverageTracker(nil)
		err := tracker.LoadRulesFromFiles([]string{ruleFile})
		require.NoError(t, err)

		// Mark some rules as tested
		tracker.MarkRuleTested("HighCPU", "test.yml", true)
		tracker.MarkRuleTested("cpu:rate5m", "test.yml", true)

		report := tracker.GenerateReport()
		require.Equal(t, 4, report.Summary.TotalRules)
		require.Equal(t, 2, report.Summary.TestedRules)
		require.Equal(t, 50.0, report.Summary.CoveragePercent)
	})

	t.Run("coverage threshold check", func(t *testing.T) {
		config := &CoverageConfig{
			MinCoverage: 75.0,
		}
		tracker := NewRuleCoverageTracker(config)
		err := tracker.LoadRulesFromFiles([]string{ruleFile})
		require.NoError(t, err)

		// Mark only 50% as tested
		tracker.MarkRuleTested("HighCPU", "test.yml", true)
		tracker.MarkRuleTested("cpu:rate5m", "test.yml", true)

		report := tracker.GenerateReport()
		err = tracker.CheckCoverageThreshold(report)
		require.Error(t, err)
		require.Contains(t, err.Error(), "below minimum threshold")
	})

	t.Run("text format output", func(t *testing.T) {
		tracker := NewRuleCoverageTracker(nil)
		err := tracker.LoadRulesFromFiles([]string{ruleFile})
		require.NoError(t, err)

		tracker.MarkRuleTested("HighCPU", "test.yml", true)
		report := tracker.GenerateReport()

		output := tracker.formatText(report)
		require.Contains(t, output, "Prometheus Rules Test Coverage Report")
		require.Contains(t, output, "25.0%")
		require.Contains(t, output, "Untested Rules")
	})

	t.Run("json format output", func(t *testing.T) {
		tracker := NewRuleCoverageTracker(nil)
		err := tracker.LoadRulesFromFiles([]string{ruleFile})
		require.NoError(t, err)

		tracker.MarkRuleTested("HighCPU", "test.yml", true)
		report := tracker.GenerateReport()

		output, err := tracker.formatJSON(report)
		require.NoError(t, err)
		require.Contains(t, output, `"total_rules": 4`)
		require.Contains(t, output, `"tested_rules": 1`)
	})

	t.Run("ignore patterns", func(t *testing.T) {
		config := &CoverageConfig{
			IgnorePatterns: []string{"test_records/*"},
		}
		tracker := NewRuleCoverageTracker(config)
		err := tracker.LoadRulesFromFiles([]string{ruleFile})
		require.NoError(t, err)

		report := tracker.GenerateReport()
		// Should only report on alert rules, not recording rules
		require.Len(t, report.UntestedRules, 2)
		for _, rule := range report.UntestedRules {
			require.NotEqual(t, "test_records", rule.Group)
		}
	})

	t.Run("mark expression tested", func(t *testing.T) {
		tracker := NewRuleCoverageTracker(nil)
		err := tracker.LoadRulesFromFiles([]string{ruleFile})
		require.NoError(t, err)

		// Test an expression that references a recording rule
		tracker.MarkExpressionTested("sum(cpu:rate5m)", "test.yml")

		// The recording rule should be marked as indirectly tested
		require.True(t, tracker.testedRules["test_records/cpu:rate5m"])
	})
}

func TestCoverageConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		tracker := NewRuleCoverageTracker(nil)
		require.NotNil(t, tracker.config)
		require.Equal(t, "text", tracker.config.OutputFormat)
	})

	t.Run("custom config", func(t *testing.T) {
		config := &CoverageConfig{
			MinCoverage:              80.0,
			FailOnUntested:           true,
			OutputFormat:             "json",
			EnableDependencyAnalysis: true,
		}
		tracker := NewRuleCoverageTracker(config)
		require.Equal(t, 80.0, tracker.config.MinCoverage)
		require.True(t, tracker.config.FailOnUntested)
		require.Equal(t, "json", tracker.config.OutputFormat)
		require.True(t, tracker.config.EnableDependencyAnalysis)
	})
}

func TestRuleIDGeneration(t *testing.T) {
	tracker := NewRuleCoverageTracker(nil)

	tests := []struct {
		groupName string
		rule      rulefmt.Rule
		expected  string
	}{
		{
			groupName: "alerts",
			rule:      rulefmt.Rule{Alert: "HighCPU"},
			expected:  "alerts/HighCPU",
		},
		{
			groupName: "records",
			rule:      rulefmt.Rule{Record: "cpu:rate5m"},
			expected:  "records/cpu:rate5m",
		},
		{
			groupName: "test",
			rule:      rulefmt.Rule{},
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tracker.generateRuleID(tt.groupName, tt.rule)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestJUnitXMLFormat(t *testing.T) {
	tracker := NewRuleCoverageTracker(nil)

	// Create a simple test rule
	tracker.rules["test/rule1"] = &RuleInfo{
		Name:  "rule1",
		Group: "test",
		Type:  "alert",
		File:  "test.yml",
	}
	tracker.rules["test/rule2"] = &RuleInfo{
		Name:  "rule2",
		Group: "test",
		Type:  "alert",
		File:  "test.yml",
	}

	// Mark one as tested
	tracker.testedRules["test/rule1"] = true

	report := tracker.GenerateReport()
	output, err := tracker.formatJUnitXML(report)
	require.NoError(t, err)

	require.Contains(t, output, `<?xml version="1.0" encoding="UTF-8"?>`)
	require.Contains(t, output, `<testsuites name="promtool-coverage">`)
	require.Contains(t, output, `<property name="coverage.overall"`)
	require.Contains(t, output, `<testcase name="test/rule1"`)
	require.Contains(t, output, `<testcase name="test/rule2"`)
	require.Contains(t, output, `<failure message="No test coverage found for this rule"/>`)
}

func TestCoverageByGroup(t *testing.T) {
	tracker := NewRuleCoverageTracker(&CoverageConfig{ByGroup: true})

	// Add rules in different groups
	tracker.rules["group1/rule1"] = &RuleInfo{Name: "rule1", Group: "group1", Type: "alert"}
	tracker.rules["group1/rule2"] = &RuleInfo{Name: "rule2", Group: "group1", Type: "alert"}
	tracker.rules["group2/rule3"] = &RuleInfo{Name: "rule3", Group: "group2", Type: "alert"}

	// Mark some as tested
	tracker.testedRules["group1/rule1"] = true
	tracker.testedRules["group2/rule3"] = true

	report := tracker.GenerateReport()

	require.NotNil(t, report.ByGroup)
	require.Len(t, report.ByGroup, 2)
	require.Equal(t, 50.0, report.ByGroup["group1"].Percentage)
	require.Equal(t, 100.0, report.ByGroup["group2"].Percentage)
}

func TestDependencyAnalysis(t *testing.T) {
	config := &CoverageConfig{
		EnableDependencyAnalysis: true,
	}
	tracker := NewRuleCoverageTracker(config)

	// Add rules with dependencies
	tracker.rules["records/cpu:rate5m"] = &RuleInfo{
		Name:       "cpu:rate5m",
		Group:      "records",
		Type:       "record",
		Expression: "rate(cpu_usage[5m])",
	}
	tracker.rules["alerts/HighCPU"] = &RuleInfo{
		Name:       "HighCPU",
		Group:      "alerts",
		Type:       "alert",
		Expression: "cpu:rate5m > 0.8",
	}

	// Analyze dependencies
	tracker.analyzeDependencies()

	// The alert should depend on the recording rule
	require.Contains(t, tracker.dependencies["alerts/HighCPU"], "records/cpu:rate5m")
}
