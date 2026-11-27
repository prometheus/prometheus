---
name: contributor-trust
description: >-
  Computes deterministic contributor trust and queue priority scores for any GitHub repository using Prometheus Governance roles (Maintainer, Member, Contributor, New Contributor) and a 90-day rolling monthly contribution average (1.0x merged PRs + 1.5x reviewed PRs). Use when ranking PR queues by author trust, auditing contributor activity, or identifying candidates for Member or Maintainer nominations.
---

# Contributor trust scorer

This skill scores contributors in any GitHub repository by combining their contributor ladder role with their 90-day rolling monthly average of accepted work. It runs with zero LLM tokens using `gh api graphql` and repository ownership files, and works both as a standalone CLI report and as a Python library imported by `.agents/skills/pr-triage-board`.

## Trust score formula

Each contributor gets a `trust_score` on a `0.0` to `90.0` scale:

```text
weighted_rma  = (1.0 * merged_prs_90d + 1.5 * reviewed_prs_90d) / (window_days / 30)
contrib_bonus = min(40.0, 12.0 * log2(1.0 + weighted_rma))
trust_score   = role_base + contrib_bonus
```

### Role base

Roles follow [Prometheus Governance](https://github.com/prometheus/governance/blob/main/ROLES.md) and map cleanly to non-Prometheus repositories:

- **Maintainer (`50`)**: Listed in `MAINTAINERS.md` (bullet or Markdown table format), listed as an individual handle in `CODEOWNERS` or `.github/CODEOWNERS`, listed under `approvers` in `OWNERS` / `OWNERS_ALIASES`, repository `OWNER`, or inferred from `mergedBy` activity when a repository has no explicit ownership files.
- **Member (`35`)**: GitHub organization `MEMBER` or repository `COLLABORATOR` not listed as a maintainer of this repository.
- **Contributor (`15`)**: Returning contributor with previously merged commits in the repository (`authorAssociation == "CONTRIBUTOR"`) or accepted merges or reviews in the rolling window.
- **New Contributor (`0`)**: First-time contributor (`FIRST_TIME_CONTRIBUTOR`, `FIRST_TIMER`, `NONE`) with no merged or reviewed PRs yet. When ordering PR triage queues in `pr-triage-board`, first-time contributors receive a `30` welcome baseline instead of `0` so new contributors get timely onboarding feedback on par with active contributors.

### Why these weights work

- **Reviews count `1.5x`, merges count `1.0x`.** Code review is the scarce bottleneck in open-source maintenance, and counting only merged PRs and distinct PRs reviewed for others prevents unmerged PR spam from inflating trust. Self-reviews on an author's own PR are excluded.
- **90-day rolling monthly average.** Averaging over 90 days (`window_days / 30` months) prevents a two-week vacation or release freeze from dropping a regular contributor to zero.
- **Logarithmic cap at `+40`.** A prolific `Contributor` averaging ~3 weighted contributions per month (`15 + 24 = 39`) ranks just ahead of an inactive `Member` (`35`), rewarding active work in the repository while preventing extreme volume differences between core maintainers from overriding governance roles.
- **Queue priority with aging (`trust_score + 1.5 * waiting_days`).** When ordering PR triage queues, adding `1.5` points per waiting day ensures small score differences do not trump weeks of waiting and prevents older PRs from starving.

## Usage

Run `scripts/score_contributors.py` (requires authenticated `gh` CLI and `python3`):

```bash
# Score active contributors in prometheus/prometheus over the last 90 days
python3 .agents/skills/contributor-trust/scripts/score_contributors.py \
  --repo prometheus/prometheus \
  --limit 50

# Find top non-member Contributors as potential org Member candidates
python3 .agents/skills/contributor-trust/scripts/score_contributors.py \
  --repo prometheus/prometheus \
  --role Contributor \
  --limit 20

# Inspect specific handles or emit JSON for another tool
python3 .agents/skills/contributor-trust/scripts/score_contributors.py \
  --repo prometheus/prometheus \
  --user bwplotka --user roidelapluie \
  --format json
```

### Common flags

- `--repo <owner/name>` (`-r`): Target repository. Repeatable to aggregate across multiple repositories. Defaults to the current git repository or `prometheus/prometheus`.
- `--window-days <days>` (`-w`): Rolling window in days for the monthly average (default: `90`).
- `--user <handle>` (`-u`): Filter output to one or more GitHub handles.
- `--role <Maintainer|Member|Contributor|New Contributor>`: Filter output to a specific governance role.
- `--limit <N>` (`-l`): Maximum contributors to display in Markdown output (default: `50`, `0` for unlimited).
- `--format <markdown|json>` (`-f`): Output format (default: `markdown`).
- `--output <path>` (`-o`): Write output to a file instead of stdout.
