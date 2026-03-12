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

- Each commit must compile and pass tests independently.
- Keep commits small and focused. Do not bundle unrelated changes in one commit.
- Sign off every commit with `git commit -s` to satisfy the DCO requirement.
- Do not include unrelated local changes in the PR (reviewers will ask you to
  remove them — see PR #18223).

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
- Use only exported APIs in tests where possible — this keeps tests closer to
  real library usage and simplifies review.
- Performance improvements require a benchmark that proves the improvement
  (maintainers will ask for one if missing — see PR #18252).
- When refactoring test utilities for reuse, move them to a dedicated package
  (e.g. `util/testwal`) rather than duplicating them — see PR #18218.
- Test helpers that interact with the filesystem should use atomic writes
  (`os.WriteFile` → temp file + `os.Rename`) to avoid race conditions with
  file watchers — see PR #18259.

---

## Performance Work

Maintainers take performance seriously. For any PERF PR:

- Provide benchmark numbers in the PR body using `go test -bench` output with
  a comparison table (before vs after).
- Reuse allocations where possible (slices, buffers). 
- When reusing buffers passed to interfaces, document that callers must copy
  the contents and must not retain references — reviewers will ask for this
  note if it is missing.
- Link to supporting analysis (Google Doc, issue, etc.) for complex changes.

---

## Code Style

- Follow [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
  and the formatting/style section of
  [Go: Best Practices for Production Environments](https://peter.bourgon.org/go-in-production/#formatting-and-style).
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

or

```
Fixes https://github.com/prometheus/prometheus/issues/18243
```

---

## Scope Discipline

- Do not include unrelated changes in a PR. Reviewers notice and will ask for
  them to be removed (PR #18223: "there seem to be a bunch of unrelated local
  changes that got checked in").
- If a PR is large, split it into preparatory and follow-up PRs and reference
  them with "Part of #NNNN" or "Depends on #NNNN".

---

## Documentation Changes

- Docs PRs are welcome for clarifying ambiguous parameter descriptions,
  fixing Markdown formatting, and keeping the OpenAPI spec consistent with
  the implementation.
- When changing documented behaviour, check whether the OpenAPI spec also
  needs updating (reviewers will ask — see PR #18245).

---

## CI / Workflow Changes

- Workflow files need the correct GitHub token permissions declared explicitly.
  Missing permissions (e.g. `statuses: write`) cause silent 403 failures
  (PR #18246).
- Use the official `prometheus/promci-artifacts` action for artifact uploads
  rather than deprecated alternatives (PR #18277).

---

## What Maintainers Notice

These observations come directly from reviewer comments on merged PRs:

- **Allocations in hot paths** — maintainers profile and benchmark carefully;
  unnecessary allocations in WAL, PromQL evaluation, or chunk encoding will be caught.
- **Correctness of optimization assumptions** — if you skip work because a
  value is "always zero", show why that is true in the PR description
  (PR #18252 documents exactly why `otherC` buckets are always zero).
- **Interface contracts** — when a performance change changes ownership or
  lifetime semantics (e.g. buffer reuse), document it at the interface
  definition, not just in the implementation.
- **Test realism** — tests should mirror production startup/shutdown sequences,
  not reach into internal methods (PR #18220).
- **Benchmark regressions** — unusual regression numbers in benchmark output
  will be questioned; address them or explain why the case is not realistic
  (PR #18247).
- **AI-generated code** — maintainers have noted that AI assistants
  (including Claude) can confidently produce incorrect analyses of unfamiliar
  algorithms (PR #18252 review). Do not blindly apply AI-suggested changes to
  numeric or algorithmic code without understanding the invariants.