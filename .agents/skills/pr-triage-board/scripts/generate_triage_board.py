#!/usr/bin/env python3
# Copyright The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""
Generates a GitHub-flavored Markdown PR Triage Board for any GitHub repository
(or list of repositories) using deterministic GraphQL queries, CODEOWNERS /
MAINTAINERS.md routing, and hygiene checks.
"""
import argparse
import base64
import datetime
import fnmatch
import json
import os
import re
import subprocess
import sys
import time


GRAPHQL_QUERY = """
query($owner: String!, $name: String!, $limit: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
    pullRequests(states: OPEN, first: $limit, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        number
        title
        url
        isDraft
        createdAt
        updatedAt
        mergeable
        additions
        deletions
        changedFiles
        author { login }
        authorAssociation
        body
        labels(first: 10) { nodes { name } }
        reviewDecision
        reviewRequests(first: 10) {
          nodes {
            requestedReviewer {
              ... on User { login }
              ... on Team { name slug }
            }
          }
        }
        latestReviews(first: 10) {
          nodes {
            author { login }
            state
            submittedAt
            body
          }
        }
        latestOpinionatedReviews(first: 10) {
          nodes {
            author { login }
            state
            submittedAt
            body
          }
        }
        comments(last: 10) {
          totalCount
          nodes {
            author { login }
            createdAt
            body
          }
        }
        reviewThreads(last: 30) {
          totalCount
          nodes {
            isResolved
            comments(last: 10) {
              nodes {
                author { login }
                state
                createdAt
                body
              }
            }
          }
        }
        files(first: 25) {
          nodes { path }
        }
        commits(last: 1) {
          totalCount
          nodes {
            commit {
              oid
              committedDate
              message
              statusCheckRollup {
                state
              }
            }
          }
        }
      }
    }
  }
}
"""


def run_cmd(cmd):
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode != 0:
        return None
    return res.stdout


def fetch_file_from_repo(owner, repo, path):
    out = run_cmd(["gh", "api", f"repos/{owner}/{repo}/contents/{path}"])
    if not out:
        return None
    try:
        data = json.loads(out)
        if "content" in data:
            return base64.b64decode(data["content"]).decode("utf-8", errors="replace")
    except Exception:
        pass
    return None


def parse_maintainers_md(content):
    """Extracts general maintainers and path-specific maintainers from MAINTAINERS.md."""
    general = []
    specific = []
    if not content:
        return general, specific

    mode = None
    current_prefix = None
    for line in content.splitlines():
        low = line.strip().lower()
        if low.startswith("general maintainers") or "core maintainers" in low:
            mode = "general"
            continue
        elif low.startswith("maintainers for specific"):
            mode = "specific"
            continue
        elif (
            low.startswith("v3 release")
            or low.startswith("release coordinators")
            or low.startswith("for the sake of")
            or low.startswith("## triage")
            or low.startswith("## alumni")
            or low.startswith("## emeritus")
        ):
            mode = None
            continue

        # Match GitHub @handles preceded by whitespace, '/', '(', or '[' (not email domains)
        handles = [h for h in re.findall(r"(?:^|[\s/(\[])@([a-zA-Z0-9-]+)", line)]
        if mode == "general" and handles:
            for h in handles:
                if h not in general:
                    general.append(h)
        elif mode == "specific":
            indent = len(line) - len(line.lstrip(" "))
            m_code = re.search(r"`([^`]+)`", line)
            if m_code:
                token = m_code.group(1).strip("/")
                if indent <= 2:
                    current_prefix = token
                    if handles:
                        specific.append((token, handles))
                elif current_prefix:
                    sub = f"{current_prefix}/{token}"
                    if handles:
                        specific.append((sub, handles))
    return general, specific


def parse_codeowners(content):
    """Parses CODEOWNERS into (pattern, [individual_handles], [team_handles])."""
    rules = []
    default_individuals = []
    if not content:
        return default_individuals, rules

    for line in content.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) < 2:
            continue
        pattern = parts[0]
        raw_owners = [o.lstrip("@") for o in parts[1:] if o.startswith("@")]
        indiv = [o for o in raw_owners if "/" not in o]
        teams = [o for o in raw_owners if "/" in o]
        if pattern == "*":
            for i in indiv:
                if i not in default_individuals:
                    default_individuals.append(i)
        else:
            rules.append((pattern.lstrip("/"), indiv, teams))
    return default_individuals, rules


def match_path(file_path, pattern):
    p = pattern.strip("/")
    if file_path == p or file_path.startswith(p + "/"):
        return True
    return fnmatch.fnmatch(file_path, pattern)


def build_repo_ownership_model(owner, repo):
    codeowners_txt = (
        fetch_file_from_repo(owner, repo, ".github/CODEOWNERS")
        or fetch_file_from_repo(owner, repo, "CODEOWNERS")
        or ""
    )
    maintainers_txt = (
        fetch_file_from_repo(owner, repo, "MAINTAINERS.md")
        or fetch_file_from_repo(owner, repo, ".github/MAINTAINERS.md")
        or ""
    )
    pr_template_txt = (
        fetch_file_from_repo(owner, repo, ".github/PULL_REQUEST_TEMPLATE.md")
        or fetch_file_from_repo(owner, repo, "PULL_REQUEST_TEMPLATE.md")
        or ""
    )

    default_co, co_rules = parse_codeowners(codeowners_txt)
    gen_maint, spec_maint = parse_maintainers_md(maintainers_txt)

    # Merge path rules (more specific paths first)
    path_rules = []
    for pat, indiv, _teams in co_rules:
        if indiv:
            path_rules.append((pat, indiv))
    for pat, indiv in spec_maint:
        if indiv and not any(p == pat for p, _ in path_rules):
            path_rules.append((pat, indiv))

    path_rules.sort(key=lambda x: len(x[0]), reverse=True)
    fallback_owners = default_co or gen_maint

    requires_release_notes = "```release-notes" in pr_template_txt
    requires_area_title = "area: short description" in pr_template_txt

    return {
        "path_rules": path_rules,
        "fallback_owners": fallback_owners,
        "requires_release_notes": requires_release_notes,
        "requires_area_title": requires_area_title,
    }


def get_proposed_owners(model, files, author):
    matched = []
    for pat, owners in model["path_rules"]:
        if any(match_path(f, pat) for f in files):
            for o in owners:
                if o.lower() != author.lower() and o not in matched:
                    matched.append(o)
    if not matched:
        matched = [o for o in model["fallback_owners"] if o.lower() != author.lower()]
    return matched


def get_current_owners(pr, author):
    curr = []
    review_nodes = pr.get("latestReviews", {}).get("nodes", []) + pr.get(
        "latestOpinionatedReviews", {}
    ).get("nodes", [])
    for r in review_nodes:
        if r.get("state") == "PENDING":
            continue
        login = (r.get("author") or {}).get("login")
        if (
            login
            and login.lower() != author.lower()
            and "copilot" not in login.lower()
            and "[bot]" not in login.lower()
        ):
            if login not in curr:
                curr.append(login)
    for n in pr.get("reviewRequests", {}).get("nodes", []):
        rr = n.get("requestedReviewer") or {}
        login = rr.get("login")
        if login and login.lower() != author.lower() and "copilot" not in login.lower():
            if login not in curr:
                curr.append(login)
    return curr


def format_owners_cell(current, proposed):
    curr_str = ", ".join(f"`@{o}`" for o in current[:3]) if current else "_none_"
    curr_set = {x.lower() for x in current}
    prop_set = {x.lower() for x in proposed}
    if curr_set & prop_set:
        return curr_str
    missing = [o for o in proposed if o.lower() not in curr_set]
    if not missing:
        return curr_str
    prop_str = ", ".join(f"`@{o}`" for o in missing[:3])
    return f"{curr_str}<br>↳ **Proposed:** {prop_str}"


def check_hygiene(model, pr):
    issues = []
    title = pr["title"]
    body = pr.get("body") or ""
    commits = pr.get("commits", {}).get("nodes", [])
    last_msg = commits[-1]["commit"]["message"] if commits else ""
    if "Signed-off-by:" not in last_msg:
        issues.append("missing-dco")
    if model["requires_area_title"]:
        if not re.match(r"^[a-zA-Z0-9_/()-]+(\s*\[PERF\])?:\s+.+", title):
            issues.append("title-format")
    if model["requires_release_notes"]:
        if "```release-notes" not in body:
            issues.append("missing-release-notes")
    if pr.get("mergeable") == "CONFLICTING":
        issues.append("merge-conflict")
    return issues


def fetch_repo_prs(owner, name, max_prs):
    nodes = []
    total_count = 0
    cursor = None
    while len(nodes) < max_prs:
        batch_size = min(35, max_prs - len(nodes))
        cmd = [
            "gh",
            "api",
            "graphql",
            "-f",
            f"query={GRAPHQL_QUERY}",
            "-F",
            f"owner={owner}",
            "-F",
            f"name={name}",
            "-F",
            f"limit={batch_size}",
        ]
        if cursor:
            cmd.extend(["-F", f"after={cursor}"])
        out = None
        for attempt in range(3):
            out = run_cmd(cmd)
            if out:
                break
            time.sleep(2 * (attempt + 1))
        if not out:
            raise RuntimeError(f"Failed to fetch PR page from GitHub GraphQL for {owner}/{name} (fetched {len(nodes)}/{max_prs})")
        payload = json.loads(out).get("data", {}).get("repository")
        if not payload:
            raise RuntimeError(f"Empty repository payload from GitHub GraphQL for {owner}/{name}")
        pr_conn = payload["pullRequests"]
        total_count = pr_conn["totalCount"]
        nodes.extend(pr_conn["nodes"])
        if not pr_conn["pageInfo"]["hasNextPage"]:
            break
        cursor = pr_conn["pageInfo"]["endCursor"]
    return total_count, nodes


CI_TOPIC_RE = re.compile(
    r"\b(ci|checks?|tests?|jobs?|workflows?|actions?|build|lint\w*|windows|fuzz\w*)\b",
    re.I,
)
CI_ISSUE_RE = re.compile(
    r"\b(fail\w*|flak\w*|timeout|timed out|unrelated|re-?run|broken|stall\w*|hang\w*|red|don't run|didn't run|not running)\b",
    re.I,
)


def is_bot(login):
    if not login:
        return True
    low = login.lower()
    return (
        "[bot]" in low
        or "dependabot" in low
        or "renovate" in low
        or "copilot" in low
        or low in ("prombot", "cla-bot", "netlify")
    )


def has_author_ci_explanation(author_comments_after_commit):
    for _, body in author_comments_after_commit:
        if CI_TOPIC_RE.search(body) and CI_ISSUE_RE.search(body):
            return True
    return False


def analyze_prs(repos, max_prs, maintainer_filter=None):
    now = datetime.datetime.now(datetime.timezone.utc)
    all_prs = []
    repo_totals = {}

    for repo_full in repos:
        owner, name = repo_full.split("/", 1)
        model = build_repo_ownership_model(owner, name)
        total_open, raw_prs = fetch_repo_prs(owner, name, max_prs)
        repo_totals[repo_full] = total_open

        for pr in raw_prs:
            author = pr["author"]["login"] if pr.get("author") else "ghost"
            if is_bot(author):
                continue
            author_low = author.lower()

            files = [f["path"] for f in pr.get("files", {}).get("nodes", [])]
            current_owners = get_current_owners(pr, author)
            proposed_owners = get_proposed_owners(model, files, author)

            if maintainer_filter:
                mf = maintainer_filter.lstrip("@").lower()
                all_owners_low = {o.lower() for o in current_owners + proposed_owners}
                if mf not in all_owners_low and author_low != mf:
                    continue

            hygiene = check_hygiene(model, pr)
            commits = pr.get("commits", {}).get("nodes", [])
            ci_state = "PENDING_APPROVAL"
            commit_date = pr["updatedAt"]
            if commits:
                c = commits[-1]["commit"]
                commit_date = c["committedDate"]
                rollup = c.get("statusCheckRollup")
                if rollup and rollup.get("state"):
                    ci_state = rollup["state"]

            opinionated_reviews = [
                r
                for r in pr.get("latestOpinionatedReviews", {}).get("nodes", [])
                if r.get("state") != "PENDING"
                and r.get("author")
                and not is_bot(r["author"].get("login"))
            ]
            latest_reviews = [
                r
                for r in pr.get("latestReviews", {}).get("nodes", [])
                if r.get("state") != "PENDING"
                and r.get("author")
                and not is_bot(r["author"].get("login"))
            ]
            reviews = []
            seen_reviews = set()
            for r in opinionated_reviews + latest_reviews:
                key = (
                    r["author"]["login"].lower(),
                    r.get("state") or "",
                    r.get("submittedAt") or "",
                )
                if key not in seen_reviews:
                    seen_reviews.add(key)
                    reviews.append(r)
            comments = [
                c
                for c in pr.get("comments", {}).get("nodes", [])
                if c.get("author") and not is_bot(c["author"].get("login"))
            ]
            threads = pr.get("reviewThreads", {}).get("nodes", [])

            events = [(commit_date, author, "commit")]
            author_comment_times = []
            author_top_comment_times = []
            author_comments_after_commit = []

            for r in reviews:
                if r.get("submittedAt"):
                    events.append((r["submittedAt"], r["author"]["login"], f"review:{r['state']}"))
            for c in comments:
                c_login = c["author"]["login"]
                c_ts = c.get("createdAt") or ""
                c_body = c.get("body") or ""
                if c_ts:
                    events.append((c_ts, c_login, "comment"))
                    if c_login.lower() == author_low:
                        author_comment_times.append(c_ts)
                        author_top_comment_times.append(c_ts)
                        if c_ts >= commit_date:
                            author_comments_after_commit.append((c_ts, c_body))

            for t in threads:
                for tc in t.get("comments", {}).get("nodes", []):
                    if (
                        tc.get("state") != "PENDING"
                        and tc.get("author")
                        and not is_bot(tc["author"].get("login"))
                    ):
                        tc_login = tc["author"]["login"]
                        tc_ts = tc.get("createdAt") or ""
                        tc_body = tc.get("body") or ""
                        if tc_ts:
                            events.append((tc_ts, tc_login, "review-thread"))
                            if tc_login.lower() == author_low:
                                author_comment_times.append(tc_ts)
                                if tc_ts >= commit_date:
                                    author_comments_after_commit.append((tc_ts, tc_body))

            events.sort(key=lambda x: x[0])
            _, last_actor, last_type = events[-1]

            latest_author_activity = max([commit_date] + author_comment_times)
            latest_author_top_comment = (
                max(author_top_comment_times) if author_top_comment_times else ""
            )

            approved_at_by_reviewer = {
                r["author"]["login"].lower(): r["submittedAt"]
                for r in reviews
                if r.get("state") == "APPROVED" and r.get("submittedAt")
            }

            unanswered_events = []
            for t in threads:
                if t.get("isResolved"):
                    continue
                t_comments = [
                    tc
                    for tc in t.get("comments", {}).get("nodes", [])
                    if tc.get("state") != "PENDING"
                    and tc.get("author")
                    and not is_bot(tc["author"].get("login"))
                ]
                if not t_comments:
                    continue
                last_tc = t_comments[-1]
                last_tc_login = last_tc["author"]["login"]
                last_tc_ts = last_tc.get("createdAt") or ""
                if last_tc_login.lower() != author_low:
                    if latest_author_top_comment > last_tc_ts:
                        continue
                    author_in_thread = any(
                        tc["author"]["login"].lower() == author_low for tc in t_comments
                    )
                    app_ts = approved_at_by_reviewer.get(last_tc_login.lower(), "")
                    if author_in_thread and app_ts >= last_tc_ts:
                        continue
                    unanswered_events.append((last_tc_ts, last_tc_login, "review-thread"))

            for c in comments:
                c_login = c["author"]["login"]
                c_ts = c.get("createdAt") or ""
                if c_login.lower() != author_low and c_ts > latest_author_activity:
                    unanswered_events.append((c_ts, c_login, "comment"))

            for r in reviews:
                r_login = r["author"]["login"]
                r_ts = r.get("submittedAt") or ""
                r_state = r.get("state") or ""
                r_body = (r.get("body") or "").strip()
                if r_login.lower() == author_low:
                    continue
                if r_state == "CHANGES_REQUESTED" and r_ts > latest_author_activity:
                    unanswered_events.append((r_ts, r_login, "review:CHANGES_REQUESTED"))
                elif r_state == "COMMENTED" and r_body and r_ts > latest_author_activity:
                    unanswered_events.append((r_ts, r_login, "review:COMMENTED"))

            if unanswered_events:
                unanswered_events.sort(key=lambda x: x[0])
                _, last_actor, last_type = unanswered_events[-1]

            non_author_reviews = [
                r for r in reviews if r["author"]["login"].lower() != author_low
            ]
            non_author_opinionated = [
                r
                for r in opinionated_reviews
                if r["author"]["login"].lower() != author_low
            ]
            latest_review_ts = max(
                (r.get("submittedAt") or "" for r in non_author_reviews), default=""
            )
            changes_requested = (
                pr.get("reviewDecision") == "CHANGES_REQUESTED"
                or any(
                    r.get("state") == "CHANGES_REQUESTED" for r in non_author_opinionated
                )
                or any(
                    r.get("state") == "CHANGES_REQUESTED" for r in non_author_reviews
                )
            )
            approved = not changes_requested and (
                pr.get("reviewDecision") == "APPROVED"
                or any(r.get("state") == "APPROVED" for r in non_author_opinionated)
                or any(r.get("state") == "APPROVED" for r in non_author_reviews)
            )
            labels = [l["name"] for l in pr.get("labels", {}).get("nodes", [])]

            ci_green = ci_state == "SUCCESS"
            ci_explained = (not ci_green) and has_author_ci_explanation(
                author_comments_after_commit
            )

            # A PR is waiting on a maintainer only when all comments/threads are
            # resolved or responded to, and CI is green (or explained by the author).
            if pr["isDraft"]:
                status_bucket = "Draft"
            elif pr.get("mergeable") == "CONFLICTING" or "needs-rebase" in labels:
                status_bucket = "Needs Rebase"
            elif ci_state == "PENDING_APPROVAL" and pr.get("authorAssociation") in (
                "FIRST_TIME_CONTRIBUTOR",
                "FIRST_TIMER",
                "NONE",
            ):
                status_bucket = "Needs /workflow-approve"
            elif unanswered_events:
                status_bucket = "Waiting on Author"
            elif not (ci_green or ci_explained):
                status_bucket = "Failing CI"
            elif non_author_reviews and latest_author_activity > latest_review_ts:
                status_bucket = "Updated After Review"
            else:
                status_bucket = "Waiting on Maintainer"

            loc = pr.get("additions", 0) + pr.get("deletions", 0)
            all_prs.append(
                {
                    "repo_full": repo_full,
                    "repo": name,
                    "number": pr["number"],
                    "title": pr["title"],
                    "url": pr["url"],
                    "author": author,
                    "owners_cell": format_owners_cell(current_owners, proposed_owners),
                    "status_bucket": status_bucket,
                    "ci_state": ci_state,
                    "ci_explained": ci_explained,
                    "hygiene": hygiene,
                    "additions": pr.get("additions", 0),
                    "deletions": pr.get("deletions", 0),
                    "loc": loc,
                    "changedFiles": pr.get("changedFiles", 0),
                    "updatedAt": pr["updatedAt"],
                    "commitDate": commit_date,
                    "lastActor": last_actor,
                    "lastEventType": last_type,
                    "approved": approved,
                    "touchesWorkflow": any(
                        p.startswith(".github/") or p.startswith("scripts/") or p == "Makefile"
                        for p in files
                    ),
                }
            )
    return now, repo_totals, all_prs


def render_markdown(repos, now, repo_totals, prs, max_prs, maintainer_filter=None):
    multi_repo = len(repos) > 1
    repo_order = {r: i for i, r in enumerate(repos)}

    def age_str(iso):
        d = datetime.datetime.fromisoformat(iso.replace("Z", "+00:00"))
        days = max(0, (now - d).days)
        return "today" if days == 0 else f"{days}d"

    def fmt_hygiene(h):
        return "clean" if not h else ", ".join(f"`{x}`" for x in h)

    def fmt_ci(state, explained=False):
        if state == "SUCCESS":
            return "pass"
        if explained:
            return "fail (commented)" if state in ("FAILURE", "ERROR") else "running (commented)"
        if state == "PENDING_APPROVAL":
            return "awaiting-approval"
        if state == "PENDING":
            return "running"
        return "fail"

    def pr_link(p):
        prefix = f"{p['repo']}" if multi_repo else ""
        return f"[{prefix}#{p['number']}]({p['url']})"

    def clean_title(t):
        return t.replace("|", "\\|")

    now_str = now.strftime("%Y-%m-%d %H:%M UTC")

    lines = [
        f"> **Auto-updated snapshot**: `{now_str}` · Analyzed `{len(prs)}` non-bot open PRs (up to {max_prs} most recently active per repo).",
        ">",
        "> **Automated via [`.agents/skills/pr-triage-board`](https://github.com/prometheus/prometheus/tree/bwplotka/setup_pr_triage_bot/.agents/skills/pr-triage-board)** · Refreshed **every hour on weekdays (Mon–Fri, 07:00–18:00 CET)**.",
        "",
    ]

    buckets = [
        "Updated After Review",
        "Waiting on Maintainer",
        "Needs /workflow-approve",
        "Waiting on Author",
        "Failing CI",
        "Needs Rebase",
        "Draft",
    ]

    lines.append("## Queue Health")
    lines.append("")
    lines.append(
        "| Repository | 🔄 Updated After Review | 👀 Waiting on Maintainer | ⏸️ Needs `/workflow-approve` | ✍️ Waiting on Author | 🔴 Failing CI | ⚠️ Needs Rebase | 📝 Draft | **Analyzed (Total Open)** |"
    )
    lines.append("| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |")
    for r in repos:
        r_prs = [p for p in prs if p["repo_full"] == r]
        counts = [sum(1 for p in r_prs if p["status_bucket"] == b) for b in buckets]
        tot_open = repo_totals.get(r, len(r_prs))
        lines.append(
            f"| `{r}` | **{counts[0]}** | **{counts[1]}** | **{counts[2]}** | {counts[3]} | {counts[4]} | {counts[5]} | {counts[6]} | **{len(r_prs)}** ({tot_open}) |"
        )
    if multi_repo:
        tot_counts = [sum(1 for p in prs if p["status_bucket"] == b) for b in buckets]
        lines.append(
            f"| **Total** | **{tot_counts[0]}** | **{tot_counts[1]}** | **{tot_counts[2]}** | {tot_counts[3]} | {tot_counts[4]} | {tot_counts[5]} | {tot_counts[6]} | **{len(prs)}** |"
        )
    lines.append("")

    def sort_key(x):
        return (repo_order.get(x["repo_full"], 99), 0 if x["approved"] else 1, x["commitDate"])

    # Quick Wins
    quick_wins = [
        p
        for p in prs
        if p["status_bucket"] in ("Updated After Review", "Waiting on Maintainer")
        and p["ci_state"] == "SUCCESS"
        and p["loc"] <= 60
        and not p["hygiene"]
    ]
    quick_wins.sort(key=sort_key)

    lines.append("## Quick Wins (<= 60 LOC, Green CI, Clean Hygiene)")
    lines.append("*Small, self-contained PRs with passing CI and clean hygiene, ready to review and merge in < 5 minutes.*")
    lines.append("")
    if quick_wins:
        lines.append("| PR | Title | Author | Diff | Waiting | Owners |")
        lines.append("| :--- | :--- | :--- | :--- | :--- | :--- |")
        for p in quick_wins:
            app = " *(approved)*" if p["approved"] else ""
            lines.append(
                f"| {pr_link(p)}{app} | {clean_title(p['title'])} | @{p['author']} | `+{p['additions']}/-{p['deletions']}` ({p['changedFiles']}f) | {age_str(p['commitDate'])} | {p['owners_cell']} |"
            )
    else:
        lines.append("_No quick wins currently matching._")
    lines.append("")

    # Updated After Review
    updated = [p for p in prs if p["status_bucket"] == "Updated After Review"]
    updated.sort(key=sort_key)

    lines.append("## Updated After Review (Author Pushed Fixes)")
    lines.append("*A maintainer already reviewed these. All comments are resolved or responded to, and CI is green (or the author commented why it is not green).*")
    lines.append("")
    if updated:
        lines.append("| PR | Title | Author | CI | Diff | Hygiene | Pushed | Owners |")
        lines.append("| :--- | :--- | :--- | :---: | :--- | :--- | :--- | :--- |")
        for p in updated:
            app = " *(approved)*" if p["approved"] else ""
            lines.append(
                f"| {pr_link(p)}{app} | {clean_title(p['title'])} | @{p['author']} | `{fmt_ci(p['ci_state'], p.get('ci_explained', False))}` | `+{p['additions']}/-{p['deletions']}` | {fmt_hygiene(p['hygiene'])} | {age_str(p['commitDate'])} | {p['owners_cell']} |"
            )
    else:
        lines.append("_No PRs currently in this bucket._")
    lines.append("")

    # Needs /workflow-approve
    need_approve = [p for p in prs if p["status_bucket"] == "Needs /workflow-approve"]
    need_approve.sort(key=sort_key)

    if need_approve:
        lines.append("## Blocked on `/workflow-approve` (First-Time Contributors)")
        lines.append("*New contributors whose CI workflows have not run yet. Verify diff safety before commenting `/workflow-approve`.*")
        lines.append("")
        lines.append("| PR | Title | Author | Diff | Touches CI/Build? | Hygiene | Waiting | Owners |")
        lines.append("| :--- | :--- | :--- | :--- | :---: | :--- | :--- | :--- |")
        for p in need_approve:
            safety = "**YES (`Makefile`/`.github`)**" if p["touchesWorkflow"] else "No (Code/Docs)"
            lines.append(
                f"| {pr_link(p)} | {clean_title(p['title'])} | @{p['author']} | `+{p['additions']}/-{p['deletions']}` ({p['changedFiles']}f) | {safety} | {fmt_hygiene(p['hygiene'])} | {age_str(p['commitDate'])} | {p['owners_cell']} |"
            )
        lines.append("")
        safe_cmds = [
            f'gh pr comment {p["number"]} --repo {p["repo_full"]} --body "/workflow-approve"'
            for p in need_approve
            if not p["touchesWorkflow"]
        ]
        if safe_cmds:
            lines.append("<details>")
            lines.append("<summary><b>One-liner to approve all safe CI runs</b></summary>")
            lines.append("")
            lines.append("```bash")
            lines.append("\n".join(safe_cmds))
            lines.append("```")
            lines.append("</details>")
            lines.append("")

    # Waiting on Maintainer
    waiting = [p for p in prs if p["status_bucket"] == "Waiting on Maintainer"]
    waiting.sort(key=sort_key)

    lines.append("## Waiting on Maintainer (Oldest First)")
    lines.append("*All comments are resolved or responded to, CI is green (or the author commented why it is not green), and there are no merge conflicts.*")
    lines.append("")
    if waiting:
        lines.append("| PR | Title | Author | CI | Diff | Hygiene | Waiting Since | Owners |")
        lines.append("| :--- | :--- | :--- | :---: | :--- | :--- | :--- | :--- |")
        for p in waiting:
            app = " *(approved)*" if p["approved"] else ""
            lines.append(
                f"| {pr_link(p)}{app} | {clean_title(p['title'])} | @{p['author']} | `{fmt_ci(p['ci_state'], p.get('ci_explained', False))}` | `+{p['additions']}/-{p['deletions']}` ({p['changedFiles']}f) | {fmt_hygiene(p['hygiene'])} | {age_str(p['commitDate'])} ({p['commitDate'][:10]}) | {p['owners_cell']} |"
            )
    else:
        lines.append("_No PRs currently waiting on maintainer._")
    lines.append("")

    # Collapsed non-maintainer buckets
    for b_title, b_key, b_desc in [
        (
            "Waiting on Author (Changes Requested / Open Review Threads)",
            "Waiting on Author",
            "Maintainer left feedback or requested changes; ball is in the author's court.",
        ),
        ("Failing CI", "Failing CI", "CI is not green and there is no author comment explaining why."),
        ("Needs Rebase (Merge Conflicts)", "Needs Rebase", "Conflicting with base branch."),
    ]:
        b_prs = [p for p in prs if p["status_bucket"] == b_key]
        b_prs.sort(key=sort_key)
        lines.append("<details>")
        lines.append(f"<summary><b>{b_title} ({len(b_prs)})</b>: {b_desc}</summary>")
        lines.append("")
        if b_prs:
            lines.append("| PR | Title | Author | Last Actor | Hygiene | Updated | Owners |")
            lines.append("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |")
            for p in b_prs:
                lines.append(
                    f"| {pr_link(p)} | {clean_title(p['title'])} | @{p['author']} | @{p['lastActor']} ({p['lastEventType']}) | {fmt_hygiene(p['hygiene'])} | {age_str(p['updatedAt'])} | {p['owners_cell']} |"
                )
        else:
            lines.append("_None._")
        lines.append("</details>")
        lines.append("")

    return "\n".join(lines)


def detect_default_repo():
    out = run_cmd(["gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"])
    if out and "/" in out.strip():
        return out.strip()
    return "prometheus/prometheus"


def main():
    parser = argparse.ArgumentParser(description="Generate a Markdown PR Triage Board for any GitHub repository.")
    parser.add_argument(
        "--repo",
        "-r",
        action="append",
        help="Target GitHub repository in owner/name format (can be passed multiple times). Defaults to current repo or prometheus/prometheus.",
    )
    parser.add_argument(
        "--limit",
        "-l",
        type=int,
        default=100,
        help="Max open PRs to fetch per repository (ordered by recently updated). Default: 100.",
    )
    parser.add_argument(
        "--maintainer",
        "-m",
        help="Optional GitHub handle to filter PRs where this maintainer is current or proposed owner.",
    )
    parser.add_argument(
        "--output",
        "-o",
        help="Optional file path to write the Markdown output.",
    )
    parser.add_argument(
        "--update-issue",
        type=int,
        help="Optional GitHub issue number in the primary repo whose description should be updated with the generated board.",
    )
    args = parser.parse_args()

    repos = args.repo if args.repo else [detect_default_repo()]
    now, repo_totals, prs = analyze_prs(repos, args.limit, args.maintainer)
    if not prs and not args.maintainer:
        print("Error: 0 PRs fetched from GitHub GraphQL API; aborting to avoid overwriting with empty board.", file=sys.stderr)
        sys.exit(1)
    md = render_markdown(repos, now, repo_totals, prs, args.limit, args.maintainer)

    if args.output:
        out_abs = os.path.abspath(args.output)
        os.makedirs(os.path.dirname(out_abs), exist_ok=True)
        tmp_out = f"{out_abs}.tmp.{os.getpid()}"
        with open(tmp_out, "w") as f:
            f.write(md)
        os.replace(tmp_out, out_abs)
        print(f"Wrote triage board ({len(prs)} PRs) to {args.output}", file=sys.stderr)
    else:
        print(md)

    if args.update_issue:
        target_repo = repos[0]
        payload = json.dumps({"body": md})
        res = subprocess.run(
            ["gh", "api", "--method", "PATCH", f"repos/{target_repo}/issues/{args.update_issue}", "--input", "-"],
            input=payload,
            capture_output=True,
            text=True,
        )
        if res.returncode == 0:
            print(f"Updated issue {target_repo}#{args.update_issue}", file=sys.stderr)
        else:
            print(f"Failed to update issue: {res.stderr}", file=sys.stderr)
            sys.exit(1)


if __name__ == "__main__":
    main()
