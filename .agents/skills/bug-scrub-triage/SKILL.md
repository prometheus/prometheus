---
name: bug-scrub-triage
description: >-
  Triages open GitHub Pull Requests and Issues in Prometheus Bug Scrub style ("Hello from the bug scrub!"). Determines whether to skip, close, route/ping reviewers, nudge authors on stale feedback/CI/conflicts, or leave architectural/scope feedback, and drafts concrete "Hello from the bug scrub!" comments and actions. Use when running or automating a bug scrub for PRs and GitHub issues.
---

# Bug Scrub Triage Skill (PRs & GitHub Issues)

This skill defines how to triage open Pull Requests and GitHub Issues during a maintainer Bug Scrub (modeled on `prometheus/prometheus` bug scrubs) and what concrete action to take for each item (`SKIP`, `CLOSE`, `COMMENT`, `ASSIGN`, `LABEL`, or `APPROVE`).

Higher-level automation or interactive sessions decide whether to apply the resulting decisions automatically, stage them for batch review, or step through items interactively.

---

## Step 1: Gather & Pre-Classify Candidates

Run `scripts/prepare_bug_scrub.py` (requires `gh` CLI authenticated and `python3`) to fetch candidate PRs and Issues, cross-reference linked PRs/issues, inspect prior `"Hello from the bug scrub!"` comments, resolve `CODEOWNERS` / `MAINTAINERS.md` owners, and compute a preliminary triage action:

```bash
# Inspect the 30 longest-inactive open PRs and 30 longest-inactive open Issues
python3 .agents/skills/bug-scrub-triage/scripts/prepare_bug_scrub.py \
  --repo prometheus/prometheus \
  --type all \
  --limit 30 \
  --order oldest-updated \
  --output /tmp/bug_scrub_queue.md
```

### Common Flags

- `--repo <owner/name>` (`-r`): Target repository (repeatable; defaults to current git repo or `prometheus/prometheus`).
- `--type <all|pr|issue>` (`-t`): Scrub both PRs and Issues (`all`), only PRs (`pr`), or only Issues (`issue`).
- `--limit <N>` (`-l`): Max non-draft, non-bot open items per type to inspect (default: `30`).
- `--order <oldest-updated|oldest-created|recently-updated|newest-created>`: Queue order (default: `oldest-updated`, surfacing neglected items first).
- `--start-from <N>`: Start the queue at PR/Issue number `<N>` (inclusive) along the `--order` direction (e.g. `--order oldest-created --start-from 5242` for old-to-new issue scrub bookmarks, or `--order newest-created --start-from 17852` for new-to-old PR scrub bookmarks).
- `--min-inactive-days <D>`: Mark items with activity in the last `<D>` days as `SKIP_ACTIVE` (default: `7`).
- `--include-drafts`: Include draft PRs (skipped by default in bug scrub).
- `--format <markdown|json>` (`-f`): Output Markdown report or machine-readable JSON for automation pipelines.

---

## Step 2: Pull Request Triage Decision Matrix

For each non-skipped PR candidate, inspect its diff (`gh pr diff <num>`) and linked issue (`gh issue view <num>`) when needed, then choose one action in priority order:

### 1. `SKIP` (Leave alone)
- **Draft PR (`SKIP_DRAFT`)**: Skip draft PRs unless the author left a comment indicating it is ready for review (in which case remind them to mark it ready for review, as bug scrub normally skips drafts).
- **Active review (`SKIP_ACTIVE`)**: Commits, review comments, or active maintainer-author discussion within `--min-inactive-days` (default 7 days).
- **Recently scrubbed (`SKIP_RECENTLY_SCRUBBED`)**: Already received a `"Hello from the bug scrub!"` comment within the last 14 days and is waiting on the pinged person to respond.

### 2. `CLOSE` (Close PR with `"Hello from the bug scrub!"` comment)
- **Abandoned after prior bug scrub (`CLOSE_ABANDONED_AFTER_SCRUB`)**: A previous bug scrub comment (>= 21 days ago) asked `@<author> do you think you will come back to this?` (or the PR has unanswered review feedback + `stale` label) with zero author activity since.
  ```markdown
  Hello from the bug scrub!

  Closing this PR due to inactivity since our last check-in. Please feel free to reopen or open a new PR if you'd like to come back to this. Thanks!
  ```
- **Superseded or already fixed (`CLOSE_SUPERSEDED`)**: Another merged PR already solved the problem or implemented the change.
  ```markdown
  Hello from the bug scrub!

  Looks like this was already addressed in #<merged-pr>. Closing, thanks anyway!
  ```
- **Obsolete or blocked upstream (`CLOSE_OBSOLETE`)**: The change targets tooling or dependencies the project moved away from (e.g. Dependabot config after switching to Renovate, or an unmerged upstream Go stdlib proposal, or an RC-only dependency like `yaml/v4` that isn't GA yet).
  ```markdown
  Hello from the bug scrub!

  <Specific reason why this is obsolete or blocked upstream, with link>. Closing for now, we can always reopen when that changes. Thanks!
  ```
- **Unjustified complexity or consensus against (`CLOSE_NO_CONSENSUS`)**: The diff adds disproportionate complexity for something without issue consensus, or contradicts prior bug scrub / Dev Summit feedback on the linked issue.
  ```markdown
  Hello from the bug scrub!

  <Specific technical reason why the change is not justified or conflicts with existing design>. Please continue the discussion on #<issue> first, and we can revisit the implementation if there is consensus.
  ```

### 3. `COMMENT_PING_AUTHOR` (Ball is in the author's court)
- **Unaddressed review feedback (`COMMENT_PING_AUTHOR_FEEDBACK`)**: A maintainer (or reviewer) requested changes or left inline review comments, and the author has been inactive for >= 14 days. Call out the key open point when possible.
  ```markdown
  Hello from the bug scrub!

  @<author> do you think you will come back to this to address the open comments (especially regarding <specific point>)?
  ```
- **Merge conflicts (`COMMENT_PING_AUTHOR_CONFLICTS`)**: `mergeable == CONFLICTING`. If the PR was already reviewed or looks close to ready, ask the author to rebase and ping the reviewer to re-check afterward.
  ```markdown
  Hello from the bug scrub!

  @<author> this looks on the right track, but there are merge conflicts now. Could you please rebase and resolve them?
  ```
- **Failing CI or missing DCO (`COMMENT_PING_AUTHOR_CI`)**: CI is failing (without an author comment explaining an unrelated flake) or commits lack `Signed-off-by:`. Quote the failing check or compiler error if obvious. If the PR looks abandoned but useful, ask the author to sign the DCO so another contributor can take it over.
  ```markdown
  Hello from the bug scrub!

  @<author> please take a look at the CI failures (<short error summary>), otherwise this looks reasonable.
  ```

### 4. `COMMENT_FOLLOWUP_REVIEWER` (Author updated, waiting on existing reviewer)
- Author pushed fixes or replied to all open review threads after the last maintainer review, and CI is green (or explained). Ping the reviewing maintainer(s) directly.
  ```markdown
  Hello from the bug scrub!

  @<reviewer> friendly ping to take another look, @<author>'s latest updates have addressed the previous comments.
  ```

### 5. `COMMENT_ROUTE_REVIEWER_OR_SANITY_CHECK` (Unreviewed PR needing a reviewer or first-pass feedback)
- Before pinging code owners from `CODEOWNERS` / `MAINTAINERS.md`, read the diff (`gh pr diff <num>`) and check:
  1. **Missing tests**: Does a bug fix or error-handling path lack a unit test reproducing the failure?
  2. **Over-complication / duplication**: Can the change reuse an existing helper or be split into smaller PRs?
  3. **Performance**: Does a hot-path or data-structure change lack benchmarks (`benchstat`)?
  4. **Breaking change / Feature flag / Dev Summit**:
     - If breaking behavior: defer to Prometheus 4.0 (`prometheus-4.0` label).
     - If adding experimental PromQL/scrape/TSDB behavior: suggest gating behind `--enable-feature=<flag>`.
     - If changing user-facing architecture without consensus: ask to discuss on the issue or add to the Prometheus Dev Summit agenda.
  5. **Stray files**: Flag accidental markdown notes or unrelated files committed in the PR.
- Draft the comment combining any first-pass technical note with the reviewer routing ping:
  ```markdown
  Hello from the bug scrub!

  Thank you for the contribution. <Optional 1-2 sentence first-pass feedback on tests, benchmarks, simplification, or feature flag>.

  @<owner1> @<owner2> would you be able to take a look?
  ```
- **Quick approve (`APPROVE_SIMPLE`)**: If the PR is a small, self-contained, obviously correct fix or test with green CI and clean hygiene, approve or note readiness to merge.

---

## Step 3: GitHub Issue Triage Decision Matrix

For each non-skipped Issue candidate, inspect its body, comments, and cross-referenced PRs/issues, then choose one action in priority order:

### 1. Check Linked & Cross-Referenced PRs First
- **Fixed by merged PR or release (`CLOSE_FIXED_BY_PR`)**: A linked PR is already `MERGED` or the fix shipped in a release.
  ```markdown
  Hello from the bug scrub!

  We see that #<pr> is already merged, so we're closing this issue. Please feel free to reopen if anything remains unresolved.
  ```
- **Active open PR in progress (`SKIP_OR_NOTE_ACTIVE_PR`)**: Keep the issue open. If the issue thread itself looks neglected, post a brief status note (`Hello from the bug scrub! PR #<pr> is under review, keeping open.`).
- **Stalled open PR (`FOLLOWUP_STALLED_LINKED_PR`)**: An open PR exists for the issue but has stalled without review or author follow-up. Nudge the area maintainer to review `#<pr>` or ask the PR author if they are still working on it.
- **Competing PRs or duplicate assignment requests**: If `#<pr>` is already under active review and another user asks to be assigned, politely reply that `#<pr>` is already in progress to avoid duplicate work. If two contributors opened partial PRs, invite them to collaborate.
- **Prior PR closed for inactivity (`COMMENT_REVIVE_AFTER_CLOSED_PR`)**: Confirm the issue is still relevant, keep it open, add `help wanted` if suitable, and state any constraints from the previous review (e.g. must be gated behind `--enable-feature`).
  ```markdown
  Hello from the bug scrub!

  The linked PR #<pr> was closed due to inactivity. We're still open to this (<e.g. behind a feature flag>) if someone is willing to pick it up.
  ```

### 2. `CLOSE` (Close Issue with `"Hello from the bug scrub!"` comment)
- **Out of scope / Wrong repository (`CLOSE_OUT_OF_SCOPE`)**: Issue belongs to `prometheus-community/helm-charts` (`kube-prometheus-stack`), `prometheus/exporter-toolkit`, `prometheus/client_golang`, `prometheus/alertmanager`, or Thanos.
  ```markdown
  Hello from the bug scrub!

  This is out of scope for the core Prometheus repository, please report or implement this at <target-repo-url> instead.
  ```
- **Duplicate of existing issue (`CLOSE_DUPLICATE`)**: Point to the canonical open issue `#<canonical>` and close as duplicate.
- **Already supported / Works as designed (`CLOSE_WORKS_AS_DESIGNED`)**: The requested behavior already exists via a flag, config option (e.g. `scrape_failure_log_file`, `--enable-feature=created-timestamp-zero-ingestion`), or metric/log. Link directly to the documentation URL or code permalink (`blob/<commit>/path#L...`) and close.
- **No response after `kind/more-info-needed` or prior scrub (`CLOSE_STALE_NO_RESPONSE`)**: Reporter never answered a prior bug scrub question (>= 30 days) or the report lacks actionable information / English description.

### 3. Triage, Label, Assign, or Escalate Open Issues
- **Valid, well-scoped bug or enhancement (`COMMENT_ACCEPT_HELP_WANTED`)**:
  - Add labels: `kind/bug` or `kind/enhancement`, `component/<area>`, `help wanted`, `priority/P3` (or `P2`/`Pmaybe`).
  - Leave a `"Hello from the bug scrub!"` comment confirming the bug/use case and outlining what a PR should include (e.g. unit test, docs update, or `--enable-feature` gate).
- **Contributor volunteered (`ASSIGN_VOLUNTEER_OR_CLARIFY`)**:
  - If the task is well-scoped and unassigned, assign the contributor (`gh issue edit <num> --add-assignee <login>`) and welcome their PR.
  - If the task is deceptively complex or mechanical maintainer work, add `not-as-easy-as-it-looks` and caution the contributor before they sink weeks into it (`This is a very involved task that warrants a design doc in prometheus/proposals first`).
- **Breaking change (`COMMENT_DEFER_V4`)**:
  - Add label `prometheus-4.0` and comment:
    ```markdown
    Hello from the bug scrub!

    Technically this changes existing behavior, so let's keep it open for Prometheus 4.0.
    ```
- **Needs architectural consensus (`COMMENT_ESCALATE_DEV_SUMMIT`)**:
  - When a feature adds new PromQL functions, changes core data models, or has unclear ROI, recommend bringing it to the Prometheus Dev Summit agenda or writing a design doc in `prometheus/proposals`.
- **Needs area maintainer opinion (`COMMENT_PING_OWNER`)**:
  - Ping the specific code owners inferred from `CODEOWNERS` / `MAINTAINERS.md` (`Hello from the bug scrub! @<owner1> @<owner2> WDYT about this?`).
- **Needs reporter clarification (`COMMENT_ASK_CLARIFICATION`)**:
  - Add `kind/more-info-needed` and ask a concrete question about reproduction steps, real-world vs. synthetic workload, or expected behavior.

---

## Step 4: Comment Style Rules

Every comment drafted or posted by this skill must follow these rules:
1. **Opening line**: Always start with `Hello from the bug scrub!` (followed by a blank line, or a single sentence on the same line for short approvals/pings).
2. **Human maintainer voice**: Write direct, concise sentences like an experienced Prometheus maintainer. No AI filler, no bolded bullet lead-ins, no decorative emojis, no em dashes.
3. **Concrete references**: Name specific `@handles`, `#issue`/`#pr` numbers, flags (`--enable-feature=...`), config keys, or full GitHub code/doc URLs.
4. **Action output format**: For each triaged item, output a structured record (in Markdown or JSON depending on caller needs) containing:
   - `item`: `PR #<num>` or `Issue #<num>` with title and URL
   - `action`: One of `SKIP_*`, `CLOSE_*`, `COMMENT_*`, `ASSIGN_*`, `APPROVE_*`
   - `labels_to_add` / `assignees_to_add` / `reviewers_to_request` (when applicable)
   - `comment_body`: Exact `"Hello from the bug scrub!"` Markdown text
   - `gh_commands`: Ready-to-run `gh pr` / `gh issue` CLI commands for the caller or automation to execute
