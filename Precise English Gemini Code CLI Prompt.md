# Precise English Gemini Code CLI Prompt

Based on PR \#16887 and issue \#11848 analysis, here's a structured prompt for Gemini Code CLI to fix the coverage tracking feature in `promtool test rules`:

## CONTEXT

You are a senior Prometheus engineer fixing the coverage-tracking implementation in PR \#16887. The current approach of directly using `testCase.Expr` to mark rules as tested is fundamentally flawed[^1].

## PROBLEM STATEMENT

In `cmd/promtool/unittest.go` (lines 462-464):

```go
if coverageTracker != nil {
    coverageTracker.MarkRuleAsTested(testCase.Expr)
}
```

**Issues:**

1. PromQL expressions may wrap rule names (`sum by(something) (record)`) rather than reference them directly[^2][^3]
2. Tests check expected data through complex expressions, not direct rule references
3. Current implementation ignores labels]

## TECHNICAL REQUIREMENTS

### 1. Update unittest.go

Replace the problematic code block with PromQL expression parsing:

```go
if coverageTracker != nil {
    // Parse expression to extract selectors
    selectors, err := parser.ExtractSelectors(testCase.Expr)
    if err != nil {
        // Log error and continue - don't fail test execution
        continue
    }
    
    // Mark rules as tested based on extracted selectors
    for _, selectorGroup := range selectors {
        coverageTracker.MarkRuleAsTestedBySelector(selectorGroup)
    }
}
```


### 2. Update coverage.go

Implement selector-based rule matching:

```go
// Add new method to CoverageTracker
func (t *CoverageTracker) MarkRuleAsTestedBySelector(matchers []*labels.Matcher) {
    for _, rule := range t.rules {
        if t.ruleMatchesSelector(rule, matchers) {
            t.markRuleAsTested(rule)
        }
    }
}

// Helper method for rule matching logic
func (t *CoverageTracker) ruleMatchesSelector(rule RuleNode, matchers []*labels.Matcher) bool {
    // Check __name__ label first
    for _, matcher := range matchers {
        if matcher.Name == labels.MetricName {
            if matcher.Value == rule.Record() || matcher.Value == rule.Alert() {
                return true
            }
        }
        // Also check other labels associated with the rule
        if ruleLabels := rule.Labels(); ruleLabels != nil {
            if labelValue := ruleLabels.Get(matcher.Name); labelValue != "" {
                if matcher.Matches(labelValue) {
                    return true
                }
            }
        }
    }
    return false
}
```


### 3. Required Imports

Add to both files:

```go
import (
    "github.com/prometheus/prometheus/promql/parser"
    "github.com/prometheus/prometheus/pkg/labels"
)
```


### 4. Error Handling

- Parse errors should not break test execution
- Log parsing failures for debugging but continue processing
- Gracefully handle empty selector results


### 5. Unit Test Updates

Create test cases covering:

- Direct rule name references: `my_rule`
- Wrapped expressions: `sum(my_rule)`
- Complex aggregations: `sum by(label) (my_rule{job="test"})`
- Label-based matching scenarios
- Parse error scenarios


## IMPLEMENTATION NOTES

- Use `promql/parser.ExtractSelectors()` which returns `[][]*labels.Matcher`[^3][^4]
- Consider both record and alert rule matching logic
- This approach provides closer-to-actual coverage tracking than string matching[^5]
- Implementation will have limitations due to rule interdependencies but represents significant improvement[^2]


## EXPECTED OUTPUT

1. Modified `unittest.go` with selector-based coverage tracking
2. Updated `coverage.go` with new matching methods
3. Proper import statements
4. Comprehensive error handling
5. Unit tests demonstrating selector-based coverage functionality

Implement these changes to provide accurate PromQL expression-based rule coverage tracking that properly handles the complexity of PromQL expressions in test scenarios.