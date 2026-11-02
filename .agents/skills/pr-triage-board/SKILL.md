---
name: pr-triage-board
description: >-
  Generates a GitHub-flavored Markdown PR Triage Board for any GitHub repository (such as prometheus/prometheus), categorizing open PRs by turn-based state (Quick Wins, Updated After Review, Blocked on /workflow-approve, Waiting on Maintainer, Waiting on Author, Failing CI, Needs Rebase), ranking queues by contributor trust and waiting age, checking PR hygiene (DCO, title prefix, release-notes, merge conflicts), and proposing missing owners from CODEOWNERS and MAINTAINERS.md. Use when a maintainer wants to generate or update a PR triage dashboard or pinned issue description for a repository.
---

# PR triage board generator

This skill generates a GitHub-flavored Markdown PR Triage Board for any GitHub repository (`owner/repo`). It can be rendered as a local report or synced into a pinned GitHub issue description (`gh issue edit`).

## What it computes

1. **Turn-based buckets**:
   A non-draft, mergeable PR is considered **waiting on maintainer** when:
   - All review threads and comments are either resolved or responded to by the author.
   - CI is green, or the author commented after the latest commit explaining why CI is not green.

   PRs are grouped into the following buckets:
   - **Quick Wins**: `<= 60` LOC, green CI, clean hygiene, waiting on maintainer.
   - **Updated After Review**: Waiting on maintainer after a prior maintainer review, with new commits or author replies since that review.
   - **Blocked on `/workflow-approve`**: First-time contributors waiting for CI approval, with a diff-safety check (`Makefile`/`.github` modifications flagged) and a copy-pasteable `gh pr comment --body "/workflow-approve"` block.
   - **Waiting on Maintainer**: Waiting on maintainer for initial review or merge.
   - **Waiting on Author / Failing CI / Needs Rebase**: Collapsed inside `<details>` blocks so they do not clutter the actionable queue.
2. **Author trust and aging priority**:
   - Works standalone with built-in author trust scoring, and reuses `.agents/skills/contributor-trust` automatically when both skills are installed side by side.
   - Classifies each author by repository role (`Maintainer=50`, `Member=35`, `Contributor=15`, `New Contributor=0`, aligned with [Prometheus Governance](https://github.com/prometheus/governance/blob/main/ROLES.md)) plus a logarithmic bonus up to `+40` from their 90-day rolling monthly contribution average (`1.0x` merged PRs + `1.5x` reviewed PRs). First-time contributors (`New Contributor`) receive a `30` welcome baseline instead of `0` in queue ordering so newcomers get timely reviews alongside active contributors rather than sinking to the bottom.
   - Within each maintainer-actionable bucket, PRs with maintainer approval and no active `CHANGES_REQUESTED` review sort first (`*(approved)*`), followed by descending `priority_score = trust_score + 1.5 * waiting_days` so trusted contributors surface quickly without starving older PRs.
   - Annotates the `Author` cell with the role and monthly contribution rate, such as `@handle` `*(Contributor, 2.7/mo)*`.
3. **Current vs. proposed owners (`↳ Proposed:`)**:
   - Fetches `CODEOWNERS`, `.github/CODEOWNERS`, `MAINTAINERS.md` (bullet or Markdown table format), and Kubernetes/Prow `OWNERS` + `OWNERS_ALIASES` from the target repository, falling back to active mergers when no ownership files exist.
   - Compares currently assigned or reviewing human maintainers against the owners for the PR's changed file paths, adding `↳ **Proposed:** @handle` only when no matching owner is currently on the PR.
   - Wraps `@handles` in backticks so updating a pinned issue description does not spam maintainers with notifications.
4. **Deterministic hygiene audit**:
   - Checks commit `Signed-off-by:` (DCO), `area: description` title format, `release-notes` code block (if required by the repo's PR template), and merge conflicts.

## Usage

Run `scripts/generate_triage_board.py` (requires `gh` CLI authenticated and `python3`):

```bash
# Generate triage board for prometheus/prometheus (default 100 most recently active open PRs)
python3 .agents/skills/pr-triage-board/scripts/generate_triage_board.py \
  --repo prometheus/prometheus \
  --limit 100 \
  --output /tmp/triage_board.md
```

### Common flags

- `--repo <owner/name>` (`-r`): Target repository. Can be passed multiple times to aggregate across repos. If omitted, detects the current git directory's GitHub repo or defaults to `prometheus/prometheus`.
- `--limit <N>` (`-l`): Maximum number of open PRs to inspect per repo (default: `100`).
- `--maintainer <handle>` (`-m`): Filter the board to PRs where `<handle>` is a current or proposed owner.
- `--output <path>` (`-o`): Save the generated Markdown to a file.
- `--update-issue <number>`: Automatically update the description of a pinned GitHub issue in the target repository via `gh issue edit`.
