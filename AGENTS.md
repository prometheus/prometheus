# Agents Guide for Prometheus

This document captures patterns and preferences observed from maintainer reviews
of recently merged pull requests. Use it to align your contributions with what
maintainers expect.

---

## PR Title Format

Titles must follow `area: short description`, using a prefix that identifies the
subsystem. Examples from merged PRs:

```
tsdb/wlog: optimize WAL watcher reads
fix(PromQL): do not skip histogram buckets when trimming
feat(agent): fix ST append; add compliance RW sender test
chore: fix emptyStringTest issues from gocritic
ci: add statuses write permission to prombench workflow
docs: clarify that `lookback_delta` query parameter takes either a duration or number of seconds
```

Common area prefixes: `tsdb`, `tsdb/wlog`, `promql`, `discovery/<name>`, `agent`,
`alerting`, `textparse`, `ui`, `build`, `ci`, `docs`, `chore`.

For performance work, append `[PERF]` to the area segment or use the `perf(area):`
convention.

---

## Commits

- Each commit must compile and pass tests independently, except when one commit adds a test to expose a bug and then the next commit fixes the bug.
- Keep commits small and focused. Do not bundle unrelated changes in one commit.
- Sign off every commit with `git commit -s` to satisfy the DCO requirement.
- Do not include unrelated local changes in the PR.

---

## Release Notes Block

Every PR must include a `release-notes` fenced code block in the description.
If there is no user-facing change, write `NONE`:

````
```release-notes
NONE
```
````

Otherwise use one of these prefixes, matching the CHANGELOG style:

```
[FEATURE]     new capability
[ENHANCEMENT] improvement to existing behaviour
[PERF]        performance improvement
[BUGFIX]      bug fix
[SECURITY]    security fix
[CHANGE]      breaking or behavioural change
```

Example:
````
```release-notes
[BUGFIX] PromQL: Do not skip histogram buckets in queries where histogram trimming is used.
```
````

---

## Tests

- Bug fixes require a test that reproduces the bug.
- New behaviour or exported API changes require unit or e2e tests.
- Tests should attempt to mirror realistic data and/or behaviour.
- Use only exported APIs in tests where possible — this keeps tests closer to
  real library usage and simplifies review.

---

## Performance Work

Maintainers take performance seriously. For any PERF PR:

- Performance improvements require a benchmark that demonstrates the improvement.
- Run benchmarks before and after the change using `go test -count=6 -benchmem -bench <directory changed in PR>`
- Provide benchmark numbers in the PR body using `benchstat` output.
- If a subset of benchmark results show a regression, address this or explain why the case is not important.
- Reuse allocations in hot paths where possible (slices, buffers). 
- When reusing buffers passed to interfaces, document that callers must copy
  the contents and must not retain references.
- Link to supporting analysis (Google Doc, issue, etc.) for complex changes.

---

## Code Style

- Follow [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
  and the formatting/style section of
  [Go: Best Practices for Production Environments](https://peter.bourgon.org/go-in-production/#formatting-and-style).
- State your assumptions.
- Interface contracts: when ownership or lifetime semantics (e.g. buffer reuse) are important,
  document it at the interface definition, not just in the implementation.
- All exposed objects must have a doc comment.
- All comments must start with a capital letter and end with a full stop.
- Run `make lint` before submitting. The project uses `golangci-lint` including
  `gocritic` rules such as `emptyStringTest` — fix linter findings rather than
  suppressing them with `//nolint` unless there is a clear false-positive.
- Use `//nolint:linter1[,linter2,...]` sparingly; prefer fixing the code.

---

## Linking Issues

Use GitHub closing keywords in the PR body so the linked issue closes
automatically on merge:

```
Fixes #18243
```

---

## Scope Discipline

- Do not include unrelated changes in a PR; make a separate PR instead.
- If a refactor is necessary to make a change, do those in separate commits.
- If a PR is large, split it into preparatory and follow-up PRs and reference
  them with "Part of #NNNN" or "Depends on #NNNN".

---

## Documentation Changes

- Docs PRs are welcome for clarifying ambiguous parameter descriptions,
  fixing Markdown formatting, and keeping the OpenAPI spec consistent with
  the implementation.
- When changing documented behaviour, update any relevant text in the docs/ directory.
  Check whether the OpenAPI spec also needs updating.

---

## CI / Workflow Changes

- Workflow files need the correct GitHub token permissions declared explicitly.
  Missing permissions (e.g. `statuses: write`) cause silent 403 failures.

---
