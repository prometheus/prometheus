# Title: promql/parser: Incorrect error position for duration division by zero

```release-notes
[BUGFIX] PromQL: Fix incorrect error position when duration expression results in division or modulo by zero.
```

## Description
When evaluating a PromQL duration expression that results in a division or modulo by zero (e.g., `foo[5s/0d]`), the parser correctly returns a `division by zero` or `modulo by zero` error. However, it additionally emits a `duration must be greater than 0` error at the wrong position (`0:0`), rather than at the actual position of the duration expression. 

This incorrect position is noted internally in `promql/parser/parse_test.go` with a `// FIXME: this position looks wrong.` comment for multiple test cases.

## Steps to Reproduce
1. Attempt to parse a PromQL query containing a duration division by zero, for example: `foo[5s/0d]` or `foo[step()/0d]`.
2. Observe the parse errors returned by the parser.

## Expected Result
The parser should report a single `division by zero` (or `modulo by zero`) error at the appropriate token location (e.g., `6:7`). If the parser also triggers the `duration must be greater than 0` validation error as a fallback, it should report it at the position of the evaluated duration expression.

## Actual Result
The parser reports the following:
1. `division by zero` at position `6:7` (Correct)
2. `duration must be greater than 0` at position `0:0` (Incorrect)

## Version
main branch (commit `c524bc30e`)

## Environment Information
- OS: N/A (Parser behavior, OS agnostic)
- Compiler (if applicable): N/A

## Repo Configuration
Standard Prometheus configuration. No specific configuration is required, as this is a PromQL parsing issue.

## Log Output
```
errors: ParseErrors{
	ParseErr{
		PositionRange: posrange.PositionRange{Start: 6, End: 7},
		Err:           errors.New(`division by zero`),
		Query:         `foo[5s/0d]`,
	},
	ParseErr{
		PositionRange: posrange.PositionRange{Start: 0, End: 0}, // FIXME: this position looks wrong.
		Err:           errors.New(`duration must be greater than 0`),
		Query:         `foo[5s/0d]`,
	},
},
```

## Additional Context
- **Root cause analysis:** In `promql/parser/generated_parser.y`, when division by zero occurs in `duration_expr DIV duration_expr`, the parser logs a `division by zero` error and assigns `$$ = &NumberLiteral{Val: 0}` as a fallback to allow parsing to continue. However, the `PosRange` of this fallback `NumberLiteral` is not initialized, so it defaults to `{Start: 0, End: 0}`. Later, when this `NumberLiteral` propagates up to `positive_duration_expr`, the check `if numLit.Val <= 0` triggers, and it reports `duration must be greater than 0` using `numLit.PositionRange()`. Since the range was not initialized, it prints `0:0`.
- **Evidence:** Seen in `promql/parser/parse_test.go` lines 5104, 5120, etc., where `// FIXME: this position looks wrong.` is explicitly written.
- **Impact:** Misleading error location reporting for users who have invalid duration expressions resulting in division or modulo by zero.
- **Affected files/packages:** `promql/parser/generated_parser.y`, `promql/parser/parse_test.go`
- **Duplicate search summary:** Searched the GitHub issues for "duration must be greater than 0" position and "FIXME: this position looks wrong". No existing tickets tracking this parser location bug were found.
- **Possible fix direction:** In `promql/parser/generated_parser.y`, initialize the fallback `NumberLiteral` with the merged position ranges of the left and right operands: `$$ = &NumberLiteral{Val: 0, PosRange: mergeRanges(&$1, &$3)}`. This should be applied to both the `DIV` and `MOD` duration expression handlers.
