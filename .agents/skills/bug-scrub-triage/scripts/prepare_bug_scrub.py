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
Prepares a Bug Scrub triage queue and per-item action recommendations for open
Pull Requests and GitHub Issues in any GitHub repository (prometheus/prometheus
by default).

Combines GraphQL metadata, CODEOWNERS / MAINTAINERS.md routing, linked PR/issue
cross-references, prior "Hello from the bug scrub!" comment history, CI and
hygiene checks, and turn-based state to classify each item into a concrete
triage action (SKIP, CLOSE, or COMMENT with a drafted "Hello from the bug scrub!"
message and recommended gh CLI commands).
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


PR_GRAPHQL_QUERY = """
query($owner: String!, $name: String!, $limit: Int!, $after: String, $orderField: IssueOrderField!, $orderDir: OrderDirection!) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
    pullRequests(states: OPEN, first: $limit, after: $after, orderBy: {field: $orderField, direction: $orderDir}) {
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
        labels(first: 15) { nodes { name } }
        assignees(first: 10) { nodes { login } }
        reviewDecision
        reviewRequests(first: 10) {
          nodes {
            requestedReviewer {
              ... on User { login }
              ... on Team { name slug }
            }
          }
        }
        reviews(last: 30) {
          nodes {
            author { login }
            state
            submittedAt
            body
          }
        }
        comments(last: 30) {
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
            path
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
        closingIssuesReferences(first: 10) {
          nodes {
            number
            title
            state
            url
          }
        }
        files(first: 30) {
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

ISSUE_GRAPHQL_QUERY = """
query($owner: String!, $name: String!, $limit: Int!, $after: String, $orderField: IssueOrderField!, $orderDir: OrderDirection!) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
    issues(states: OPEN, first: $limit, after: $after, orderBy: {field: $orderField, direction: $orderDir}) {
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        number
        title
        url
        createdAt
        updatedAt
        author { login }
        authorAssociation
        body
        labels(first: 15) { nodes { name } }
        assignees(first: 10) { nodes { login } }
        comments(last: 30) {
          totalCount
          nodes {
            author { login }
            createdAt
            body
          }
        }
        timelineItems(itemTypes: [CROSS_REFERENCED_EVENT, CONNECTED_EVENT], last: 20) {
          nodes {
            __typename
            ... on CrossReferencedEvent {
              createdAt
              source {
                __typename
                ... on PullRequest {
                  number
                  title
                  state
                  isDraft
                  url
                  updatedAt
                  mergedAt
                  author { login }
                }
                ... on Issue {
                  number
                  title
                  state
                  url
                }
              }
            }
            ... on ConnectedEvent {
              createdAt
              subject {
                __typename
                ... on PullRequest {
                  number
                  title
                  state
                  isDraft
                  url
                  updatedAt
                  mergedAt
                  author { login }
                }
              }
            }
          }
        }
      }
    }
  }
}
"""

BUG_SCRUB_RE = re.compile(r"\b(bug[\s-]+scrub|hello from (?:the )?scrub)\b", re.I)
ABANDON_CHECK_RE = re.compile(
    r"(come back to this|still working on|still interested|still needed|still required|can we close|can this be closed|plan to continue|are you coming back)",
    re.I,
)
REVIEWER_PING_RE = re.compile(
    r"(take a(?:nother)? look|continu(?:e|ing) the review|offered to (?:review|take a look)|will take a(?: closer)? look|would you be able to|could you (?:have|take) a look|friendly ping|kind ping|plz take a look|WDYT about @)",
    re.I,
)
VOLUNTEER_RE = re.compile(
    r"\b(i(?:'d| would) like to (?:work on|pick up|take) this|can i (?:work on|take|be assigned)|assign (?:this to )?me|i can (?:work on|take) this)\b",
    re.I,
)
CI_TOPIC_RE = re.compile(
    r"\b(ci|checks?|tests?|jobs?|workflows?|actions?|build|lint\w*|windows|fuzz\w*)\b",
    re.I,
)
CI_ISSUE_RE = re.compile(
    r"\b(fail\w*|flak\w*|timeout|timed out|unrelated|re-?run|broken|stall\w*|hang\w*|red|don't run|didn't run|not running)\b",
    re.I,
)

COMPONENT_PATH_HINTS = {
    "component/tsdb": ["tsdb/"],
    "component/promql": ["promql/"],
    "component/remote storage": ["storage/remote/"],
    "component/scraping": ["scrape/"],
    "component/service discovery": ["discovery/"],
    "component/ui": ["web/ui/"],
    "component/api": ["web/api/"],
    "component/rules": ["rules/"],
    "component/promtool": ["cmd/promtool/"],
    "component/config": ["config/"],
    "component/notify": ["notifier/"],
    "component/agent": ["tsdb/agent/"],
}


def run_cmd(cmd):
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode != 0:
        return None
    return res.stdout


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

    path_rules = []
    for pat, indiv, _teams in co_rules:
        if indiv:
            path_rules.append((pat, indiv))
    for pat, indiv in spec_maint:
        if indiv and not any(p == pat for p, _ in path_rules):
            path_rules.append((pat, indiv))

    path_rules.sort(key=lambda x: len(x[0]), reverse=True)
    fallback_owners = default_co or gen_maint

    return {
        "path_rules": path_rules,
        "fallback_owners": fallback_owners,
        "requires_release_notes": "```release-notes" in pr_template_txt,
        "requires_area_title": "area: short description" in pr_template_txt,
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


def infer_issue_owners(model, title, labels, author):
    synthetic_paths = []
    for lbl in labels:
        for hint in COMPONENT_PATH_HINTS.get(lbl.lower(), []):
            synthetic_paths.append(hint)
    m_prefix = re.match(r"^([a-zA-Z0-9_/.-]+)\s*[:\-]", title.strip())
    if m_prefix:
        prefix = m_prefix.group(1).strip("/").lower()
        synthetic_paths.append(prefix + "/")
        if prefix in ("ui", "web/ui"):
            synthetic_paths.append("web/ui/")
        elif prefix in ("promtool", "cmd/promtool"):
            synthetic_paths.append("cmd/promtool/")
        elif prefix in ("remote", "remote_write", "remote-write", "storage/remote"):
            synthetic_paths.append("storage/remote/")
    title_low = title.lower()
    for kw, path_hint in [
        ("remote write", "storage/remote/"),
        ("remote_write", "storage/remote/"),
        ("remote-write", "storage/remote/"),
        ("remote read", "storage/remote/"),
        ("tsdb", "tsdb/"),
        ("wal", "tsdb/wlog/"),
        ("promql", "promql/"),
        ("promtool", "cmd/promtool/"),
        ("scrape", "scrape/"),
        ("relabel", "model/relabel/"),
        ("ui", "web/ui/"),
        ("rule", "rules/"),
        ("agent", "tsdb/agent/"),
    ]:
        if re.search(r"\b" + re.escape(kw) + r"\b", title_low):
            synthetic_paths.append(path_hint)
    if not synthetic_paths:
        return []
    matched = []
    for pat, owners in model["path_rules"]:
        if any(match_path(f.rstrip("/"), pat) or match_path(f + "dummy.go", pat) for f in synthetic_paths):
            for o in owners:
                if o.lower() != author.lower() and o not in matched:
                    matched.append(o)
    return matched[:3]


def get_current_pr_owners(pr, author):
    curr = []
    for a in pr.get("assignees", {}).get("nodes", []):
        login = a.get("login")
        if login and login.lower() != author.lower() and not is_bot(login):
            if login not in curr:
                curr.append(login)
    for r in pr.get("reviews", {}).get("nodes", []):
        if r.get("state") == "PENDING":
            continue
        login = (r.get("author") or {}).get("login")
        if login and login.lower() != author.lower() and not is_bot(login):
            if login not in curr:
                curr.append(login)
    for n in pr.get("reviewRequests", {}).get("nodes", []):
        rr = n.get("requestedReviewer") or {}
        login = rr.get("login")
        if login and login.lower() != author.lower() and not is_bot(login):
            if login not in curr:
                curr.append(login)
    return curr


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


def has_author_ci_explanation(author_comments_after_commit):
    for _, body in author_comments_after_commit:
        if CI_TOPIC_RE.search(body) and CI_ISSUE_RE.search(body):
            return True
    return False


def days_between(now, iso_str):
    if not iso_str:
        return 0
    dt = datetime.datetime.fromisoformat(iso_str.replace("Z", "+00:00"))
    return max(0, (now - dt).days)


def is_reviewer_ping_only(body, author_low):
    if not body:
        return False
    if ABANDON_CHECK_RE.search(body):
        return False
    if not REVIEWER_PING_RE.search(body):
        return False
    # Check if the comment directly addresses @author at the start of a line/sentence
    for line in body.splitlines():
        if re.match(rf"^\s*@{re.escape(author_low)}\b", line.strip(), re.I):
            return False
    return True


def extract_scrub_history(comments_and_reviews, author_low):
    scrubs = []
    for ts, login, body, kind in comments_and_reviews:
        if not body or not login:
            continue
        # Ignore quoted replies by the PR/issue author
        if login.lower() == author_low and body.lstrip().startswith(">"):
            continue
        if BUG_SCRUB_RE.search(body):
            scrubs.append(
                {
                    "timestamp": ts,
                    "author": login,
                    "kind": kind,
                    "body": body.strip(),
                    "asked_if_returning": bool(ABANDON_CHECK_RE.search(body)),
                    "pinged_reviewer_only": is_reviewer_ping_only(body, author_low),
                }
            )
    scrubs.sort(key=lambda x: x["timestamp"])
    return scrubs


def fetch_graphql_connection(
    owner, name, query, conn_key, max_items, order_field, order_dir, skip_draft_prs=True
):
    nodes = []
    total_count = 0
    cursor = None
    while len(nodes) < max_items:
        batch_size = min(35, max(15, (max_items - len(nodes)) * 2 if skip_draft_prs else (max_items - len(nodes))))
        cmd = [
            "gh",
            "api",
            "graphql",
            "-f",
            f"query={query}",
            "-F",
            f"owner={owner}",
            "-F",
            f"name={name}",
            "-F",
            f"limit={batch_size}",
            "-F",
            f"orderField={order_field}",
            "-F",
            f"orderDir={order_dir}",
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
            raise RuntimeError(f"Failed GraphQL fetch for {owner}/{name} ({conn_key})")
        payload = json.loads(out).get("data", {}).get("repository")
        if not payload:
            raise RuntimeError(f"Empty repository payload for {owner}/{name} ({conn_key})")
        conn = payload[conn_key]
        total_count = conn["totalCount"]
        for n in conn["nodes"]:
            author = (n.get("author") or {}).get("login")
            if is_bot(author):
                continue
            if skip_draft_prs and conn_key == "pullRequests" and n.get("isDraft"):
                continue
            nodes.append(n)
            if len(nodes) >= max_items:
                break
        if not conn["pageInfo"]["hasNextPage"]:
            break
        cursor = conn["pageInfo"]["endCursor"]
    return total_count, nodes


def analyze_pr_node(now, repo_full, model, pr, min_inactive_days):
    author = pr["author"]["login"] if pr.get("author") else "ghost"
    author_low = author.lower()
    files = [f["path"] for f in pr.get("files", {}).get("nodes", [])]
    current_owners = get_current_pr_owners(pr, author)
    proposed_owners = get_proposed_owners(model, files, author)
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

    reviews = [
        r
        for r in pr.get("reviews", {}).get("nodes", [])
        if r.get("state") != "PENDING" and r.get("author") and not is_bot(r["author"].get("login"))
    ]
    comments = [
        c
        for c in pr.get("comments", {}).get("nodes", [])
        if c.get("author") and not is_bot(c["author"].get("login"))
    ]
    threads = pr.get("reviewThreads", {}).get("nodes", [])

    events = [(commit_date, author, "commit", "")]
    all_text_events = []
    author_comment_times = []
    author_top_comment_times = []
    author_comments_after_commit = []

    for r in reviews:
        r_ts = r.get("submittedAt") or ""
        r_login = r["author"]["login"]
        r_body = r.get("body") or ""
        if r_ts:
            events.append((r_ts, r_login, f"review:{r['state']}", r_body))
            all_text_events.append((r_ts, r_login, r_body, f"review:{r['state']}"))

    for c in comments:
        c_login = c["author"]["login"]
        c_ts = c.get("createdAt") or ""
        c_body = c.get("body") or ""
        if c_ts:
            events.append((c_ts, c_login, "comment", c_body))
            all_text_events.append((c_ts, c_login, c_body, "comment"))
            if c_login.lower() == author_low:
                author_comment_times.append(c_ts)
                author_top_comment_times.append(c_ts)
                if c_ts >= commit_date:
                    author_comments_after_commit.append((c_ts, c_body))

    for t in threads:
        for tc in t.get("comments", {}).get("nodes", []):
            if tc.get("state") != "PENDING" and tc.get("author") and not is_bot(tc["author"].get("login")):
                tc_login = tc["author"]["login"]
                tc_ts = tc.get("createdAt") or ""
                tc_body = tc.get("body") or ""
                if tc_ts:
                    events.append((tc_ts, tc_login, "review-thread", tc_body))
                    all_text_events.append((tc_ts, tc_login, tc_body, "review-thread"))
                    if tc_login.lower() == author_low:
                        author_comment_times.append(tc_ts)
                        if tc_ts >= commit_date:
                            author_comments_after_commit.append((tc_ts, tc_body))

    events.sort(key=lambda x: x[0])
    last_event_ts, last_actor, last_type, _ = events[-1]
    latest_author_activity = max([commit_date] + author_comment_times)
    latest_author_top_comment = max(author_top_comment_times) if author_top_comment_times else ""

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
            if tc.get("state") != "PENDING" and tc.get("author") and not is_bot(tc["author"].get("login"))
        ]
        if not t_comments:
            continue
        last_tc = t_comments[-1]
        last_tc_login = last_tc["author"]["login"]
        last_tc_ts = last_tc.get("createdAt") or ""
        last_tc_body = (last_tc.get("body") or "").strip()
        if last_tc_login.lower() != author_low:
            if latest_author_top_comment > last_tc_ts:
                continue
            author_in_thread = any(tc["author"]["login"].lower() == author_low for tc in t_comments)
            app_ts = approved_at_by_reviewer.get(last_tc_login.lower(), "")
            if author_in_thread and app_ts >= last_tc_ts:
                continue
            unanswered_events.append((last_tc_ts, last_tc_login, f"review-thread ({t.get('path', '')})", last_tc_body))

    for c in comments:
        c_login = c["author"]["login"]
        c_ts = c.get("createdAt") or ""
        c_body = (c.get("body") or "").strip()
        if c_login.lower() != author_low and c_ts > latest_author_activity:
            if is_reviewer_ping_only(c_body, author_low):
                continue
            unanswered_events.append((c_ts, c_login, "comment", c_body))

    for r in reviews:
        r_login = r["author"]["login"]
        r_ts = r.get("submittedAt") or ""
        r_state = r.get("state") or ""
        r_body = (r.get("body") or "").strip()
        if r_login.lower() == author_low:
            continue
        if r_state == "CHANGES_REQUESTED" and r_ts > latest_author_activity:
            unanswered_events.append((r_ts, r_login, "review:CHANGES_REQUESTED", r_body))
        elif r_state == "COMMENTED" and r_body and r_ts > latest_author_activity:
            if is_reviewer_ping_only(r_body, author_low):
                continue
            unanswered_events.append((r_ts, r_login, "review:COMMENTED", r_body))

    unanswered_events.sort(key=lambda x: x[0])

    non_author_reviews = [r for r in reviews if r["author"]["login"].lower() != author_low]
    latest_review_ts = max((r.get("submittedAt") or "" for r in non_author_reviews), default="")
    latest_reviewer = None
    if non_author_reviews:
        latest_r = max(non_author_reviews, key=lambda r: r.get("submittedAt") or "")
        latest_reviewer = latest_r["author"]["login"]

    approved = any(r.get("state") == "APPROVED" for r in non_author_reviews)
    labels = [l["name"] for l in pr.get("labels", {}).get("nodes", [])]
    linked_issues = [
        {"number": i["number"], "title": i["title"], "state": i["state"], "url": i["url"]}
        for i in pr.get("closingIssuesReferences", {}).get("nodes", [])
        if i
    ]

    scrubs = extract_scrub_history(all_text_events, author_low)
    last_scrub = scrubs[-1] if scrubs else None
    days_since_scrub = days_between(now, last_scrub["timestamp"]) if last_scrub else None
    author_active_since_scrub = (
        (latest_author_activity > last_scrub["timestamp"]) if last_scrub else True
    )

    inactive_days = days_between(now, pr["updatedAt"])
    age_days = days_between(now, pr["createdAt"])
    ci_green = ci_state == "SUCCESS"
    ci_explained = (not ci_green) and has_author_ci_explanation(author_comments_after_commit)

    # Determine Bug Scrub triage recommendation
    ping_targets = (current_owners or proposed_owners)[:2]
    ping_str = " ".join(f"@{o}" for o in ping_targets) if ping_targets else "@maintainers"

    if pr["isDraft"]:
        action = "SKIP_DRAFT"
        reason = "Draft PR; bug scrub skips drafts unless ready for review."
        draft_comment = None
    elif is_bot(author):
        action = "SKIP_BOT"
        reason = f"Bot author (@{author})."
        draft_comment = None
    elif inactive_days < min_inactive_days:
        action = "SKIP_ACTIVE"
        reason = f"Active within the last {inactive_days}d (< {min_inactive_days}d threshold); under active discussion."
        draft_comment = None
    elif last_scrub and days_since_scrub is not None and days_since_scrub < 14 and not author_active_since_scrub:
        action = "SKIP_RECENTLY_SCRUBBED"
        reason = f"Already scrubbed {days_since_scrub}d ago by @{last_scrub['author']}; waiting for reply."
        draft_comment = None
    elif (
        last_scrub
        and not author_active_since_scrub
        and days_since_scrub is not None
        and days_since_scrub >= 21
        and not last_scrub.get("pinged_reviewer_only")
        and (last_scrub["asked_if_returning"] or unanswered_events)
    ):
        action = "CLOSE_ABANDONED_AFTER_SCRUB"
        reason = f"Scrubbed {days_since_scrub}d ago by @{last_scrub['author']} with no author response since."
        draft_comment = (
            "Hello from the bug scrub!\n\n"
            "Closing this PR due to inactivity since our last check-in. "
            "Please feel free to reopen or open a new PR if you'd like to come back to this. Thanks!"
        )
    elif pr.get("mergeable") == "CONFLICTING" or "needs-rebase" in labels:
        action = "COMMENT_PING_AUTHOR_CONFLICTS"
        reason = "PR has merge conflicts with the base branch."
        if approved or current_owners:
            rev_ping = f" @{latest_reviewer}" if latest_reviewer else (f" {ping_str}" if current_owners else "")
            draft_comment = (
                f"Hello from the bug scrub!\n\n"
                f"@{author} there are merge conflicts on this PR now; could you please rebase and resolve them?"
                + (f"\n\n{rev_ping.strip()} could you take another look once it is rebased?" if rev_ping else "")
            )
        else:
            draft_comment = (
                f"Hello from the bug scrub!\n\n"
                f"@{author} this PR currently has merge conflicts. Do you plan to come back to this and rebase?"
            )
    elif unanswered_events:
        last_u_ts, last_u_actor, last_u_kind, last_u_body = unanswered_events[-1]
        u_days = days_between(now, last_u_ts)
        snippet = last_u_body.replace("\n", " ")[:120]
        action = "COMMENT_PING_AUTHOR_FEEDBACK"
        reason = f"Unanswered {last_u_kind} from @{last_u_actor} ({u_days}d ago): {snippet!r}"
        draft_comment = (
            f"Hello from the bug scrub!\n\n"
            f"@{author} do you think you will come back to this to address the open review comments from @{last_u_actor}?"
        )
    elif ci_state == "PENDING_APPROVAL" and pr.get("authorAssociation") in (
        "FIRST_TIME_CONTRIBUTOR",
        "FIRST_TIMER",
        "NONE",
    ):
        touches_wf = any(p.startswith(".github/") or p.startswith("scripts/") or p == "Makefile" for p in files)
        action = "APPROVE_WORKFLOW_AND_ROUTE"
        reason = (
            f"First-time contributor blocked on /workflow-approve (touches workflow/build: {touches_wf})."
        )
        draft_comment = (
            f"Hello from the bug scrub!\n\n"
            f"Thank you for the contribution. {ping_str} would you be able to take a look?"
        )
    elif not (ci_green or ci_explained):
        action = "COMMENT_PING_AUTHOR_CI"
        reason = f"CI is {ci_state} with no author explanation comment after the latest commit."
        dco_note = (
            " Also please sign off your commits (`git commit -s`) to satisfy the DCO check."
            if "missing-dco" in hygiene
            else ""
        )
        draft_comment = (
            f"Hello from the bug scrub!\n\n"
            f"@{author} please take a look at the failing CI checks.{dco_note} "
            f"Do you plan to continue working on this?"
        )
    elif non_author_reviews and latest_author_activity > latest_review_ts:
        rev_handle = latest_reviewer or (current_owners[0] if current_owners else None)
        rev_mention = f"@{rev_handle}" if rev_handle else ping_str
        action = "COMMENT_FOLLOWUP_REVIEWER"
        reason = f"Author @{author} updated the PR after @{rev_handle or 'reviewer'}'s last review; CI is green."
        draft_comment = (
            f"Hello from the bug scrub!\n\n"
            f"{rev_mention} friendly ping to take another look, @{author}'s latest updates have addressed the previous comments."
        )
    else:
        action = "COMMENT_ROUTE_REVIEWER_OR_SANITY_CHECK"
        reason = (
            f"Waiting on maintainer review for {days_between(now, commit_date)}d "
            f"(current owners: {current_owners or 'none'}, proposed: {proposed_owners[:3]}). "
            f"Inspect diff for scope/design/test feedback before pinging."
        )
        draft_comment = (
            f"Hello from the bug scrub!\n\n"
            f"Thank you for the contribution (and apologies for the delay). "
            f"{ping_str} would you be able to take a look?"
        )

    return {
        "item_type": "PR",
        "repo": repo_full,
        "number": pr["number"],
        "title": pr["title"],
        "url": pr["url"],
        "author": author,
        "createdAt": pr["createdAt"],
        "updatedAt": pr["updatedAt"],
        "age_days": age_days,
        "inactive_days": inactive_days,
        "isDraft": pr["isDraft"],
        "mergeable": pr.get("mergeable"),
        "ci_state": ci_state,
        "ci_explained": ci_explained,
        "hygiene": hygiene,
        "additions": pr.get("additions", 0),
        "deletions": pr.get("deletions", 0),
        "changedFiles": pr.get("changedFiles", 0),
        "files": files[:15],
        "labels": labels,
        "current_owners": current_owners,
        "proposed_owners": proposed_owners[:3],
        "approved": approved,
        "last_actor": last_actor,
        "last_event_type": last_type,
        "linked_issues": linked_issues,
        "scrub_count": len(scrubs),
        "last_scrub": last_scrub,
        "author_active_since_scrub": author_active_since_scrub,
        "recommended_action": action,
        "reason": reason,
        "draft_comment": draft_comment,
    }


def analyze_issue_node(now, repo_full, model, iss, min_inactive_days):
    author = iss["author"]["login"] if iss.get("author") else "ghost"
    author_low = author.lower()
    labels = [l["name"] for l in iss.get("labels", {}).get("nodes", [])]
    assignees = [a["login"] for a in iss.get("assignees", {}).get("nodes", []) if a.get("login")]
    proposed_owners = infer_issue_owners(model, iss["title"], labels, author)

    comments = [
        c
        for c in iss.get("comments", {}).get("nodes", [])
        if c.get("author") and not is_bot(c["author"].get("login"))
    ]
    all_text_events = [
        (c.get("createdAt") or "", c["author"]["login"], c.get("body") or "", "comment")
        for c in comments
        if c.get("createdAt")
    ]
    scrubs = extract_scrub_history(all_text_events, author_low)
    last_scrub = scrubs[-1] if scrubs else None
    days_since_scrub = days_between(now, last_scrub["timestamp"]) if last_scrub else None

    author_comment_times = [
        c.get("createdAt")
        for c in comments
        if c["author"]["login"].lower() == author_low and c.get("createdAt")
    ]
    latest_author_activity = max([iss["createdAt"]] + author_comment_times)
    author_active_since_scrub = (
        (latest_author_activity > last_scrub["timestamp"]) if last_scrub else True
    )

    # Check volunteers asking to work on the issue
    volunteers = []
    for c in comments:
        c_login = c["author"]["login"]
        c_body = c.get("body") or ""
        if VOLUNTEER_RE.search(c_body) and c_login not in volunteers:
            volunteers.append(c_login)

    # Extract linked PRs from timeline
    linked_prs_by_num = {}
    for item in iss.get("timelineItems", {}).get("nodes", []):
        if not item:
            continue
        src = item.get("source") or item.get("subject")
        if src and src.get("__typename") == "PullRequest" and src.get("number"):
            linked_prs_by_num[src["number"]] = {
                "number": src["number"],
                "title": src.get("title"),
                "state": src.get("state"),
                "isDraft": src.get("isDraft"),
                "url": src.get("url"),
                "updatedAt": src.get("updatedAt"),
                "mergedAt": src.get("mergedAt"),
                "author": (src.get("author") or {}).get("login"),
            }
    linked_prs = list(linked_prs_by_num.values())
    open_prs = [p for p in linked_prs if p["state"] == "OPEN"]
    merged_prs = [p for p in linked_prs if p["state"] == "MERGED"]
    closed_prs = [p for p in linked_prs if p["state"] == "CLOSED"]

    inactive_days = days_between(now, iss["updatedAt"])
    age_days = days_between(now, iss["createdAt"])
    owners_ping = " ".join(f"@{o}" for o in (assignees or proposed_owners)[:2])

    if inactive_days < min_inactive_days:
        action = "SKIP_ACTIVE"
        reason = f"Active within the last {inactive_days}d (< {min_inactive_days}d threshold)."
        draft_comment = None
    elif last_scrub and days_since_scrub is not None and days_since_scrub < 14 and not author_active_since_scrub:
        action = "SKIP_RECENTLY_SCRUBBED"
        reason = f"Already scrubbed {days_since_scrub}d ago by @{last_scrub['author']}."
        draft_comment = None
    elif merged_prs and not open_prs:
        m_refs = ", ".join(f"#{p['number']}" for p in merged_prs)
        action = "CLOSE_FIXED_BY_PR"
        reason = f"Linked PR ({m_refs}) is already merged; verify if issue is completely resolved."
        draft_comment = (
            f"Hello from the bug scrub!\n\n"
            f"It looks like this was addressed by {m_refs}, so we're closing this issue. "
            f"Please feel free to reopen if anything remains unresolved."
        )
    elif open_prs:
        active_open = [p for p in open_prs if days_between(now, p["updatedAt"]) < 21]
        o_refs = ", ".join(f"#{p['number']}" for p in open_prs)
        if active_open:
            action = "SKIP_OR_NOTE_ACTIVE_PR"
            reason = f"Linked open PR ({o_refs}) is actively in progress."
            draft_comment = (
                f"Hello from the bug scrub!\n\n"
                f"PR {o_refs} is currently in progress, keeping this issue open."
            )
        else:
            stalled_pr = open_prs[0]
            action = "FOLLOWUP_STALLED_LINKED_PR"
            reason = f"Linked open PR #{stalled_pr['number']} by @{stalled_pr['author']} has been inactive for {days_between(now, stalled_pr['updatedAt'])}d."
            draft_comment = (
                f"Hello from the bug scrub!\n\n"
                f"There is an open PR #{stalled_pr['number']} for this issue. "
                + (f"{owners_ping} could you take a look at the PR?" if owners_ping else f"@{stalled_pr['author']} are you still working on #{stalled_pr['number']}?")
            )
    elif (
        last_scrub
        and not author_active_since_scrub
        and days_since_scrub is not None
        and days_since_scrub >= 30
        and ("kind/more-info-needed" in labels or "stale" in labels or last_scrub["asked_if_returning"])
    ):
        action = "CLOSE_STALE_NO_RESPONSE"
        reason = f"Scrubbed {days_since_scrub}d ago asking for info/status with no response from @{author}."
        draft_comment = (
            "Hello from the bug scrub!\n\n"
            "Closing this issue due to lack of response since our last check-in. "
            "Please feel free to reopen if this is still an issue and you can provide the requested details."
        )
    elif closed_prs and not open_prs and not merged_prs:
        c_refs = ", ".join(f"#{p['number']}" for p in closed_prs)
        action = "COMMENT_REVIVE_AFTER_CLOSED_PR"
        reason = f"Prior PR ({c_refs}) was closed without merge; decide whether to keep open for new contributors."
        draft_comment = (
            f"Hello from the bug scrub!\n\n"
            f"The previous PR ({c_refs}) was closed due to inactivity. "
            f"We're still open to contributions on this if someone would like to pick it up."
        )
    elif volunteers and not assignees:
        vol = volunteers[-1]
        action = "ASSIGN_VOLUNTEER_OR_CLARIFY"
        reason = f"@{vol} offered to work on this unassigned issue; verify suitability before assigning."
        draft_comment = (
            f"Hello from the bug scrub!\n\n"
            f"Thanks for looking into this @{vol}, we have assigned the issue to you."
        )
    else:
        action = "NEEDS_ISSUE_TRIAGE"
        reason = (
            f"Open issue ({age_days}d old, {inactive_days}d inactive, labels: {labels or 'none'}). "
            f"Inspect content to decide: close (out-of-scope / duplicate / works-as-designed), "
            f"accept (`help wanted`), defer (`prometheus-4.0`), escalate (Dev Summit / proposal), or ping owner ({owners_ping or 'none'})."
        )
        if owners_ping:
            draft_comment = (
                f"Hello from the bug scrub!\n\n"
                f"{owners_ping} WDYT about this issue?"
            )
        else:
            draft_comment = (
                "Hello from the bug scrub!\n\n"
                "Thanks for raising this. <Insert specific assessment: confirm bug + help wanted, ask for clarification, or close with reason>."
            )

    return {
        "item_type": "ISSUE",
        "repo": repo_full,
        "number": iss["number"],
        "title": iss["title"],
        "url": iss["url"],
        "author": author,
        "createdAt": iss["createdAt"],
        "updatedAt": iss["updatedAt"],
        "age_days": age_days,
        "inactive_days": inactive_days,
        "labels": labels,
        "assignees": assignees,
        "proposed_owners": proposed_owners,
        "volunteers": volunteers,
        "linked_prs": linked_prs,
        "scrub_count": len(scrubs),
        "last_scrub": last_scrub,
        "author_active_since_scrub": author_active_since_scrub,
        "recommended_action": action,
        "reason": reason,
        "draft_comment": draft_comment,
    }


def render_markdown(repos, now, items, order_mode, min_inactive_days):
    now_str = now.strftime("%Y-%m-%d %H:%M UTC")
    prs = [x for x in items if x["item_type"] == "PR"]
    issues = [x for x in items if x["item_type"] == "ISSUE"]
    actionable = [x for x in items if not x["recommended_action"].startswith("SKIP_")]
    skips = [x for x in items if x["recommended_action"].startswith("SKIP_")]

    lines = [
        f"# Bug Scrub Triage Queue ({', '.join(repos)})",
        "",
        f"> **Generated**: `{now_str}` · **Order**: `{order_mode}` · **Min Inactive Days**: `{min_inactive_days}d`",
        f"> **Inspected**: `{len(prs)}` PRs, `{len(issues)}` Issues · **Actionable**: `{len(actionable)}` · **Skipped (Active/Draft/Bot)**: `{len(skips)}`",
        "",
    ]

    # Action summary table
    action_counts = {}
    for x in items:
        key = (x["item_type"], x["recommended_action"])
        action_counts[key] = action_counts.get(key, 0) + 1

    lines.append("## Summary by Recommended Action")
    lines.append("")
    lines.append("| Type | Recommended Action | Count |")
    lines.append("| :--- | :--- | :---: |")
    for (itype, act), cnt in sorted(action_counts.items()):
        lines.append(f"| `{itype}` | `{act}` | **{cnt}** |")
    lines.append("")

    if actionable:
        lines.append("## Actionable Bug Scrub Candidates")
        lines.append("")
        lines.append("| Item | Age / Inactive | Author | Owners / Assignees | Prior Scrubs | Recommended Action | Reason |")
        lines.append("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |")
        for x in actionable:
            prefix = "PR" if x["item_type"] == "PR" else "Issue"
            link = f"[{prefix} #{x['number']}]({x['url']}): {x['title'].replace('|', '\\|')}"
            age_col = f"{x['age_days']}d / {x['inactive_days']}d"
            if x["item_type"] == "PR":
                owners = x["current_owners"] or [f"{o} (prop)" for o in x["proposed_owners"]]
            else:
                owners = x["assignees"] or [f"{o} (prop)" for o in x["proposed_owners"]]
            owners_str = ", ".join(f"`@{o}`" for o in owners[:3]) if owners else "_none_"
            if x["last_scrub"]:
                scrub_str = f"{x['scrub_count']}x (last {x['last_scrub']['timestamp'][:10]} by @{x['last_scrub']['author']})"
            else:
                scrub_str = "never"
            reason_short = x["reason"].replace("|", "\\|")
            lines.append(
                f"| {link} | `{age_col}` | @{x['author']} | {owners_str} | {scrub_str} | `{x['recommended_action']}` | {reason_short} |"
            )
        lines.append("")

        lines.append("## Drafted Bug Scrub Comments & Actions")
        lines.append("")
        for x in actionable:
            prefix = "PR" if x["item_type"] == "PR" else "Issue"
            lines.append(f"### {prefix} #{x['number']}: {x['title']}")
            lines.append(f"- **URL**: {x['url']}")
            lines.append(f"- **Author**: @{x['author']} · **Created**: `{x['createdAt'][:10]}` (`{x['age_days']}d` ago) · **Last Updated**: `{x['updatedAt'][:10]}` (`{x['inactive_days']}d` ago)")
            if x["item_type"] == "PR":
                lines.append(
                    f"- **State**: CI=`{x['ci_state']}` · Mergeable=`{x['mergeable']}` · Diff=`+{x['additions']}/-{x['deletions']}` ({x['changedFiles']} files) · Hygiene=`{', '.join(x['hygiene']) or 'clean'}`"
                )
                if x["linked_issues"]:
                    li_str = ", ".join(f"[#{i['number']}]({i['url']}) ({i['state']})" for i in x["linked_issues"])
                    lines.append(f"- **Linked Issues**: {li_str}")
            else:
                lines.append(f"- **Labels**: `{', '.join(x['labels']) or 'none'}` · **Assignees**: `{', '.join(x['assignees']) or 'none'}`")
                if x["linked_prs"]:
                    lp_str = ", ".join(f"[#{p['number']}]({p['url']}) ({p['state']})" for p in x["linked_prs"])
                    lines.append(f"- **Linked PRs**: {lp_str}")
            if x["last_scrub"]:
                lines.append(
                    f"- **Last Scrub**: `{x['last_scrub']['timestamp'][:10]}` by @{x['last_scrub']['author']} (Author active since scrub: `{x['author_active_since_scrub']}`)"
                )
            lines.append(f"- **Recommended Action**: `{x['recommended_action']}`: {x['reason']}")
            if x["draft_comment"]:
                lines.append("")
                lines.append("```markdown")
                lines.append(x["draft_comment"])
                lines.append("```")
            lines.append("")

    if skips:
        lines.append("<details>")
        lines.append(f"<summary><b>Skipped Items ({len(skips)}): Active, Recently Scrubbed, Draft, or Bot</b></summary>")
        lines.append("")
        lines.append("| Item | Updated | Action | Reason |")
        lines.append("| :--- | :--- | :--- | :--- |")
        for x in skips:
            prefix = "PR" if x["item_type"] == "PR" else "Issue"
            lines.append(
                f"| [{prefix} #{x['number']}]({x['url']}): {x['title'].replace('|', '\\|')} | `{x['inactive_days']}d` ago | `{x['recommended_action']}` | {x['reason'].replace('|', '\\|')} |"
            )
        lines.append("")
        lines.append("</details>")
        lines.append("")

    return "\n".join(lines)


def detect_default_repo():
    out = run_cmd(["gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"])
    if out and "/" in out.strip():
        return out.strip()
    return "prometheus/prometheus"


def main():
    parser = argparse.ArgumentParser(
        description="Prepare a Bug Scrub triage queue and recommended actions for Pull Requests and GitHub Issues."
    )
    parser.add_argument(
        "--repo",
        "-r",
        action="append",
        help="Target GitHub repository in owner/name format (default: current repo or prometheus/prometheus).",
    )
    parser.add_argument(
        "--type",
        "-t",
        choices=["all", "pr", "issue"],
        default="all",
        help="Item types to scrub: 'all' (default), 'pr', or 'issue'.",
    )
    parser.add_argument(
        "--limit",
        "-l",
        type=int,
        default=30,
        help="Max open items per type to fetch per repository (default: 30).",
    )
    parser.add_argument(
        "--order",
        choices=["oldest-updated", "oldest-created", "recently-updated", "newest-created"],
        default="oldest-updated",
        help="Ordering of open items to inspect (default: oldest-updated, surfacing neglected items first).",
    )
    parser.add_argument(
        "--min-inactive-days",
        type=int,
        default=7,
        help="Skip items updated within the last N days as actively worked on (default: 7).",
    )
    parser.add_argument(
        "--include-drafts",
        action="store_true",
        help="Include draft PRs (by default draft PRs are skipped during bug scrub).",
    )
    parser.add_argument(
        "--format",
        "-f",
        choices=["markdown", "json"],
        default="markdown",
        help="Output format: 'markdown' (default) or 'json'.",
    )
    parser.add_argument(
        "--output",
        "-o",
        help="Optional file path to write the output.",
    )
    args = parser.parse_args()

    order_map = {
        "oldest-updated": ("UPDATED_AT", "ASC"),
        "oldest-created": ("CREATED_AT", "ASC"),
        "recently-updated": ("UPDATED_AT", "DESC"),
        "newest-created": ("CREATED_AT", "DESC"),
    }
    order_field, order_dir = order_map[args.order]
    repos = args.repo if args.repo else [detect_default_repo()]
    now = datetime.datetime.now(datetime.timezone.utc)

    all_items = []
    for repo_full in repos:
        owner, name = repo_full.split("/", 1)
        model = build_repo_ownership_model(owner, name)
        if args.type in ("all", "pr"):
            _, raw_prs = fetch_graphql_connection(
                owner,
                name,
                PR_GRAPHQL_QUERY,
                "pullRequests",
                args.limit,
                order_field,
                order_dir,
                skip_draft_prs=not args.include_drafts,
            )
            for pr in raw_prs:
                all_items.append(analyze_pr_node(now, repo_full, model, pr, args.min_inactive_days))
        if args.type in ("all", "issue"):
            _, raw_issues = fetch_graphql_connection(
                owner,
                name,
                ISSUE_GRAPHQL_QUERY,
                "issues",
                args.limit,
                order_field,
                order_dir,
                skip_draft_prs=False,
            )
            for iss in raw_issues:
                all_items.append(analyze_issue_node(now, repo_full, model, iss, args.min_inactive_days))

    if args.format == "json":
        output_text = json.dumps(
            {
                "generated_at": now.isoformat(),
                "repos": repos,
                "order": args.order,
                "min_inactive_days": args.min_inactive_days,
                "items": all_items,
            },
            indent=2,
        )
    else:
        output_text = render_markdown(repos, now, all_items, args.order, args.min_inactive_days)

    if args.output:
        out_abs = os.path.abspath(args.output)
        os.makedirs(os.path.dirname(out_abs), exist_ok=True)
        with open(out_abs, "w") as f:
            f.write(output_text)
        print(f"Wrote bug scrub queue ({len(all_items)} items) to {args.output}", file=sys.stderr)
    else:
        print(output_text)


if __name__ == "__main__":
    main()
