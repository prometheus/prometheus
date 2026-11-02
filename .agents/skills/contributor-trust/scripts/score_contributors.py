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
Computes contributor trust scores for any GitHub repository using repository
governance roles (Maintainer, Member, Contributor, New Contributor) and a
rolling monthly contribution average (1.0x merged PRs + 1.5x reviewed PRs).
"""
import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
import datetime
import json
import math
import os
import re
import subprocess
import sys
import time


ROLE_MAINTAINER = "Maintainer"
ROLE_MEMBER = "Member"
ROLE_CONTRIBUTOR = "Contributor"
ROLE_NEW_CONTRIBUTOR = "New Contributor"

ROLE_BASE_SCORES = {
    ROLE_MAINTAINER: 50.0,
    ROLE_MEMBER: 35.0,
    ROLE_CONTRIBUTOR: 15.0,
    ROLE_NEW_CONTRIBUTOR: 0.0,
}

ROLE_RANK = {
    ROLE_NEW_CONTRIBUTOR: 0,
    ROLE_CONTRIBUTOR: 1,
    ROLE_MEMBER: 2,
    ROLE_MAINTAINER: 3,
}

ASSOC_RANK = {
    "NONE": 0,
    "FIRST_TIMER": 1,
    "FIRST_TIME_CONTRIBUTOR": 1,
    "CONTRIBUTOR": 2,
    "COLLABORATOR": 3,
    "MEMBER": 4,
    "OWNER": 5,
}

PROMETHEUS_ORGS = {"prometheus", "prometheus-community", "promcon"}
RESERVED_GH_PATHS = {
    "orgs",
    "organizations",
    "teams",
    "issues",
    "pulls",
    "pull",
    "discussions",
    "actions",
    "settings",
    "security",
    "sponsors",
    "about",
    "pricing",
    "features",
}

MERGED_PR_WEIGHT = 1.0
REVIEWED_PR_WEIGHT = 1.5
CONTRIB_LOG_SCALE = 12.0
MAX_CONTRIB_BONUS = 40.0
DEFAULT_WINDOW_DAYS = 90
DEFAULT_AGING_PER_DAY = 1.5
NEW_CONTRIBUTOR_WELCOME_SCORE = 30.0


# NOTE(bwplotka): We query repository.pullRequests ordered by UPDATED_AT DESC
# instead of the GraphQL search connection. GitHub search caps results at 1,000
# items and enforces strict secondary rate limits, whereas repository.pullRequests
# streams cleanly and can stop as soon as updatedAt falls behind the window cutoff.
ACTIVITY_GRAPHQL_QUERY = """
query($owner: String!, $name: String!, $states: [PullRequestState!]!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: $states, first: 100, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        number
        state
        updatedAt
        mergedAt
        mergedBy { __typename login }
        author { __typename login }
        authorAssociation
        reviews(last: 30, states: [APPROVED, CHANGES_REQUESTED, COMMENTED]) {
          nodes {
            submittedAt
            authorAssociation
            author { __typename login }
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


def is_bot(actor):
    if not actor:
        return True
    if isinstance(actor, dict):
        if actor.get("__typename") == "Bot":
            return True
        login = actor.get("login")
    else:
        login = str(actor)
    if not login:
        return True
    low = login.lower()
    return (
        "[bot]" in low
        or "dependabot" in low
        or "renovate" in low
        or "copilot" in low
        or low in ("prombot", "cla-bot", "netlify", "codecov")
    )


def _extract_handles_from_line(line):
    """Extracts GitHub handles from a line, preferring github.com/<handle> links over @tokens."""
    gh_links = [
        h
        for h in re.findall(r"github\.com/([a-zA-Z0-9-]+)(?:[/?#)\]\s]|$)", line)
        if h.lower() not in RESERVED_GH_PATHS
    ]
    if gh_links:
        seen = []
        for h in gh_links:
            if h not in seen:
                seen.append(h)
        return seen

    # Strip inline code spans containing whitespace (such as CNCF Slack `@Full Name`)
    # before matching @handles so display names are not mistaken for GitHub logins.
    cleaned = re.sub(r"`[^`]*\s[^`]*`", " ", line)
    handles = re.findall(r"(?:^|[\s/(\[|`])@([a-zA-Z0-9-]+)", cleaned)
    seen = []
    for h in handles:
        if h not in seen:
            seen.append(h)
    return seen


def parse_maintainers_md(content):
    """Extracts general maintainers, path-specific rules, and all active maintainer handles."""
    general = []
    specific = []
    all_maintainers = []
    if not content:
        return general, specific, all_maintainers

    # Default to "general" so simple bullet or Markdown-table MAINTAINERS.md files
    # (such as client_golang, common, and thanos-io/thanos) work without specific headers.
    mode = "general"
    prefix_stack = []

    for line in content.splitlines():
        stripped = line.strip()
        low = stripped.lower()
        if not stripped:
            continue
        if low.startswith("general maintainers") or "core maintainers" in low:
            mode = "general"
            prefix_stack = []
            continue
        if low.startswith("maintainers for specific") or "named maintainers" in low:
            mode = "specific"
            prefix_stack = []
            continue
        if (
            low.startswith("v3 release")
            or low.startswith("release coordinators")
            or low.startswith("for the sake of")
            or (
                low.startswith("#")
                and any(
                    stop in low
                    for stop in ("triage", "alumni", "emeritus", "retired", "former")
                )
            )
            or low.startswith("emeritus")
        ):
            mode = None
            prefix_stack = []
            continue
        if mode is None:
            continue

        # Skip Markdown table separator rows (e.g. |---|---|).
        if stripped.startswith("|") and re.match(r"^\|[\s:|-]+\|?$", stripped):
            continue

        handles = _extract_handles_from_line(line)
        indent = len(line) - len(line.lstrip(" "))
        m_code = re.match(r"^\s*[-*]\s*`([^`]+)`", line)

        if m_code:
            token = m_code.group(1).strip("/")
            while prefix_stack and prefix_stack[-1][0] >= indent:
                prefix_stack.pop()
            prefix_stack.append((indent, token))
            full_path = "/".join(t for _, t in prefix_stack)
            if handles:
                specific.append((full_path, handles))
                for h in handles:
                    if h not in all_maintainers:
                        all_maintainers.append(h)
        elif handles and stripped.startswith(("*", "-", "|")):
            for h in handles:
                if mode == "general" and h not in general:
                    general.append(h)
                if h not in all_maintainers:
                    all_maintainers.append(h)

    return general, specific, all_maintainers


def parse_codeowners(content):
    """Parses CODEOWNERS into default owners, path rules, and all individual handles."""
    rules = []
    default_individuals = []
    all_individuals = []
    if not content:
        return default_individuals, rules, all_individuals

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
        for i in indiv:
            if i not in all_individuals:
                all_individuals.append(i)
        if pattern == "*":
            for i in indiv:
                if i not in default_individuals:
                    default_individuals.append(i)
        else:
            rules.append((pattern.lstrip("/"), indiv, teams))
    return default_individuals, rules, all_individuals


def parse_owners_yaml(owners_content, aliases_content=""):
    """Parses Kubernetes/Prow-style OWNERS and OWNERS_ALIASES files for approvers."""
    if not owners_content:
        return []

    aliases = {}
    if aliases_content:
        current_alias = None
        in_aliases = False
        for raw in aliases_content.splitlines():
            line = raw.split("#", 1)[0].rstrip()
            stripped = line.strip()
            if not stripped:
                continue
            indent = len(line) - len(line.lstrip(" "))
            if indent == 0:
                in_aliases = stripped.startswith("aliases:")
                current_alias = None
                continue
            if not in_aliases:
                continue
            if stripped.endswith(":") and not stripped.startswith("-"):
                current_alias = stripped[:-1].strip()
                aliases.setdefault(current_alias, [])
            elif stripped.startswith("-") and current_alias:
                val = stripped[1:].strip().lstrip("@")
                if val:
                    aliases[current_alias].append(val)

    approvers = []
    in_approvers = False
    for raw in owners_content.splitlines():
        line = raw.split("#", 1)[0].rstrip()
        stripped = line.strip()
        if not stripped:
            continue
        indent = len(line) - len(line.lstrip(" "))
        if not stripped.startswith("-") and stripped.endswith(":"):
            key = stripped[:-1].strip().lower()
            if indent == 0 or key in ("approvers", "reviewers", "labels", "filters"):
                in_approvers = key == "approvers"
                continue
        if in_approvers and stripped.startswith("-"):
            token = stripped[1:].strip().lstrip("@")
            expanded = aliases.get(token, [token])
            for h in expanded:
                if h and "/" not in h and not is_bot(h) and h not in approvers:
                    approvers.append(h)
    return approvers


def fetch_repo_maintainer_handles(owner, repo):
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
    owners_txt = fetch_file_from_repo(owner, repo, "OWNERS") or ""
    aliases_txt = (
        fetch_file_from_repo(owner, repo, "OWNERS_ALIASES") if owners_txt else ""
    ) or ""

    _, _, co_all = parse_codeowners(codeowners_txt)
    _, _, maint_all = parse_maintainers_md(maintainers_txt)
    owners_approvers = parse_owners_yaml(owners_txt, aliases_txt)
    return {h.lower() for h in (maint_all + co_all + owners_approvers)}


def classify_governance_role(
    login,
    author_association,
    maintainer_handles_lower,
    has_accepted_work=False,
    repo_owner=None,
):
    """Maps a GitHub user to repository roles and base score."""
    assoc = (author_association or "NONE").upper()
    if (
        (login and login.lower() in maintainer_handles_lower)
        or assoc == "OWNER"
        or (login and repo_owner and login.lower() == repo_owner.lower())
    ):
        return ROLE_MAINTAINER, ROLE_BASE_SCORES[ROLE_MAINTAINER]

    if assoc in ("MEMBER", "COLLABORATOR"):
        return ROLE_MEMBER, ROLE_BASE_SCORES[ROLE_MEMBER]
    if assoc == "CONTRIBUTOR" or has_accepted_work:
        return ROLE_CONTRIBUTOR, ROLE_BASE_SCORES[ROLE_CONTRIBUTOR]
    return ROLE_NEW_CONTRIBUTOR, ROLE_BASE_SCORES[ROLE_NEW_CONTRIBUTOR]


def compute_trust_score(role_base, merged_count, reviewed_count, window_days=DEFAULT_WINDOW_DAYS):
    """Computes rolling monthly contribution metrics and total trust score."""
    months = max(1.0 / 30.0, window_days / 30.0)
    merged_rma = merged_count / months
    reviewed_rma = reviewed_count / months
    weighted_total = (MERGED_PR_WEIGHT * merged_count) + (REVIEWED_PR_WEIGHT * reviewed_count)
    weighted_rma = weighted_total / months
    contrib_bonus = min(
        MAX_CONTRIB_BONUS,
        CONTRIB_LOG_SCALE * math.log2(1.0 + weighted_rma),
    )
    trust_score = round(role_base + contrib_bonus, 1)
    return {
        "merged_count": merged_count,
        "reviewed_count": reviewed_count,
        "merged_rma": round(merged_rma, 2),
        "reviewed_rma": round(reviewed_rma, 2),
        "weighted_rma": round(weighted_rma, 2),
        "contrib_bonus": round(contrib_bonus, 1),
        "trust_score": trust_score,
    }


def effective_queue_trust_score(
    profile, new_contributor_welcome=NEW_CONTRIBUTOR_WELCOME_SCORE
):
    """Returns the trust score used for PR queue ordering, giving first-time contributors a welcome baseline."""
    if profile.get("role") == ROLE_NEW_CONTRIBUTOR:
        return max(profile["trust_score"], new_contributor_welcome)
    return profile["trust_score"]


def compute_priority_score(trust_score, waiting_days, aging_per_day=DEFAULT_AGING_PER_DAY):
    """Combines author trust with waiting age so older PRs steadily bubble up."""
    return round(trust_score + (aging_per_day * max(0.0, waiting_days)), 2)


def format_author_cell(login, profile):
    """Formats an Author table cell with repository role and 90d RMA."""
    role = profile["role"]
    w_rma = profile["weighted_rma"]
    if w_rma >= 0.1:
        return f"@{login}<br>*({role}, {w_rma:.1f}/mo)*"
    return f"@{login}<br>*({role})*"


def _fetch_pr_stream(owner, name, states, cutoff_iso):
    nodes = []
    cursor = None
    while True:
        cmd = [
            "gh",
            "api",
            "graphql",
            "-f",
            f"query={ACTIVITY_GRAPHQL_QUERY}",
            "-F",
            f"owner={owner}",
            "-F",
            f"name={name}",
        ]
        for s in states:
            cmd.extend(["-F", f"states[]={s}"])
        if cursor:
            cmd.extend(["-F", f"after={cursor}"])

        out = None
        for attempt in range(3):
            out = run_cmd(cmd)
            if out:
                break
            time.sleep(2 * (attempt + 1))
        if not out:
            raise RuntimeError(
                f"Failed to fetch PR activity from GitHub GraphQL for {owner}/{name} ({states})"
            )

        repo_data = (json.loads(out).get("data") or {}).get("repository")
        if not repo_data:
            raise RuntimeError(f"Empty repository payload from GitHub GraphQL for {owner}/{name}")
        conn = repo_data["pullRequests"]
        batch = conn["nodes"]
        if not batch:
            break
        nodes.extend(batch)
        if batch[-1]["updatedAt"] < cutoff_iso or not conn["pageInfo"]["hasNextPage"]:
            break
        cursor = conn["pageInfo"]["endCursor"]
    return nodes


def infer_maintainers_from_merges(pr_nodes, cutoff_iso):
    """Infers active maintainers from mergedBy when a repository has no maintainer files."""
    merged_others = set()
    merge_counts = {}
    for pr in pr_nodes:
        if not pr or pr.get("state") != "MERGED":
            continue
        merged_at = pr.get("mergedAt") or ""
        if merged_at < cutoff_iso:
            continue
        merger = pr.get("mergedBy")
        if not merger or is_bot(merger):
            continue
        m_low = merger["login"].lower()
        merge_counts[m_low] = merge_counts.get(m_low, 0) + 1
        author = pr.get("author")
        a_low = author["login"].lower() if author and author.get("login") else ""
        if a_low != m_low and not is_bot(author):
            merged_others.add(m_low)

    return {
        m_low
        for m_low, cnt in merge_counts.items()
        if m_low in merged_others or cnt >= 2
    }


def aggregate_contributor_profiles(
    pr_nodes,
    maintainer_handles_lower,
    cutoff_iso,
    window_days=DEFAULT_WINDOW_DAYS,
    repo_owner=None,
    infer_maintainers_if_empty=True,
):
    """Aggregates merged PRs and distinct reviewed PRs per contributor from raw PR nodes."""
    effective_maintainers = set(maintainer_handles_lower or ())
    if not effective_maintainers and infer_maintainers_if_empty:
        effective_maintainers = infer_maintainers_from_merges(pr_nodes, cutoff_iso)

    canonical_login = {}
    best_assoc = {}
    merged_prs = {}
    reviewed_prs = {}

    def record_user(login, assoc):
        low = login.lower()
        canonical_login.setdefault(low, login)
        prev = best_assoc.get(low, "NONE")
        if ASSOC_RANK.get((assoc or "NONE").upper(), 0) >= ASSOC_RANK.get(prev, 0):
            best_assoc[low] = (assoc or "NONE").upper()
        return low

    seen_pr_numbers = set()
    for pr in pr_nodes:
        if not pr or pr.get("number") in seen_pr_numbers:
            continue
        seen_pr_numbers.add(pr["number"])

        author_obj = pr.get("author")
        pr_author_low = ""
        if author_obj and not is_bot(author_obj):
            pr_author_low = record_user(author_obj["login"], pr.get("authorAssociation"))
            merged_at = pr.get("mergedAt") or ""
            if pr.get("state") == "MERGED" and merged_at >= cutoff_iso:
                merged_prs.setdefault(pr_author_low, set()).add(pr["number"])

        merger_obj = pr.get("mergedBy")
        if merger_obj and not is_bot(merger_obj):
            record_user(merger_obj["login"], "MEMBER")

        for rev in (pr.get("reviews") or {}).get("nodes") or []:
            r_author = rev.get("author")
            if not r_author or is_bot(r_author):
                continue
            r_submitted = rev.get("submittedAt") or ""
            if r_submitted < cutoff_iso:
                continue
            r_low = record_user(r_author["login"], rev.get("authorAssociation"))
            # Exclude self-reviews because GitHub records inline thread replies
            # by the PR author as PullRequestReview objects with state COMMENTED.
            if r_low == pr_author_low:
                continue
            reviewed_prs.setdefault(r_low, set()).add(pr["number"])

    profiles = {}
    for low, login in canonical_login.items():
        m_cnt = len(merged_prs.get(low, ()))
        r_cnt = len(reviewed_prs.get(low, ()))
        assoc = best_assoc.get(low, "NONE")
        role, role_base = classify_governance_role(
            login,
            assoc,
            effective_maintainers,
            has_accepted_work=(m_cnt + r_cnt) > 0,
            repo_owner=repo_owner,
        )
        metrics = compute_trust_score(role_base, m_cnt, r_cnt, window_days=window_days)
        profiles[low] = {
            "login": login,
            "role": role,
            "role_base": role_base,
            "author_association": assoc,
            **metrics,
        }
    return profiles


def fetch_repo_contributor_profiles(
    owner,
    name,
    now=None,
    window_days=DEFAULT_WINDOW_DAYS,
    maintainer_handles_lower=None,
):
    """Fetches rolling contributor trust profiles for a GitHub repository."""
    if now is None:
        now = datetime.datetime.now(datetime.timezone.utc)
    cutoff_dt = now - datetime.timedelta(days=window_days)
    cutoff_iso = cutoff_dt.strftime("%Y-%m-%dT%H:%M:%SZ")

    if maintainer_handles_lower is None:
        maintainer_handles_lower = fetch_repo_maintainer_handles(owner, name)

    with ThreadPoolExecutor(max_workers=2) as pool:
        f_closed = pool.submit(_fetch_pr_stream, owner, name, ["MERGED", "CLOSED"], cutoff_iso)
        f_open = pool.submit(_fetch_pr_stream, owner, name, ["OPEN"], cutoff_iso)
        pr_nodes = f_closed.result() + f_open.result()

    return aggregate_contributor_profiles(
        pr_nodes,
        maintainer_handles_lower,
        cutoff_iso,
        window_days=window_days,
        repo_owner=owner,
    )


def resolve_author_profile(
    profiles,
    login,
    author_association,
    maintainer_handles_lower,
    window_days=DEFAULT_WINDOW_DAYS,
    repo_owner=None,
):
    """Returns a contributor's profile, falling back to role-only scoring if inactive in the window."""
    low = login.lower()
    existing = profiles.get(low)
    if existing:
        role, role_base = classify_governance_role(
            login,
            author_association,
            maintainer_handles_lower,
            has_accepted_work=(existing["merged_count"] + existing["reviewed_count"]) > 0,
            repo_owner=repo_owner,
        )
        if ROLE_RANK[role] > ROLE_RANK[existing["role"]]:
            metrics = compute_trust_score(
                role_base,
                existing["merged_count"],
                existing["reviewed_count"],
                window_days=window_days,
            )
            return {
                **existing,
                "role": role,
                "role_base": role_base,
                **metrics,
            }
        return existing

    role, role_base = classify_governance_role(
        login,
        author_association,
        maintainer_handles_lower,
        has_accepted_work=False,
        repo_owner=repo_owner,
    )
    metrics = compute_trust_score(role_base, 0, 0, window_days=window_days)
    return {
        "login": login,
        "role": role,
        "role_base": role_base,
        "author_association": (author_association or "NONE").upper(),
        **metrics,
    }


def merge_multi_repo_profiles(repo_profiles_map, window_days=DEFAULT_WINDOW_DAYS):
    """Combines contributor profiles across multiple repositories."""
    combined = {}
    for repo_full, profiles in repo_profiles_map.items():
        for low, prof in profiles.items():
            entry = combined.get(low)
            if not entry:
                entry = {
                    "login": prof["login"],
                    "role": prof["role"],
                    "role_base": prof["role_base"],
                    "author_association": prof["author_association"],
                    "merged_count": 0,
                    "reviewed_count": 0,
                    "repos": {},
                }
                combined[low] = entry
            if ROLE_RANK[prof["role"]] > ROLE_RANK[entry["role"]]:
                entry["role"] = prof["role"]
                entry["role_base"] = prof["role_base"]
            if ASSOC_RANK.get(prof["author_association"], 0) > ASSOC_RANK.get(
                entry["author_association"], 0
            ):
                entry["author_association"] = prof["author_association"]
            entry["merged_count"] += prof["merged_count"]
            entry["reviewed_count"] += prof["reviewed_count"]
            entry["repos"][repo_full] = {
                "role": prof["role"],
                "merged_count": prof["merged_count"],
                "reviewed_count": prof["reviewed_count"],
                "trust_score": prof["trust_score"],
            }

    out = {}
    for low, entry in combined.items():
        metrics = compute_trust_score(
            entry["role_base"],
            entry["merged_count"],
            entry["reviewed_count"],
            window_days=window_days,
        )
        out[low] = {
            **entry,
            **metrics,
        }
    return out


def render_markdown_report(repos, now, window_days, profiles_list, limit):
    now_str = now.strftime("%Y-%m-%d %H:%M UTC")
    repos_str = ", ".join(f"`{r}`" for r in repos)
    shown = profiles_list[:limit] if limit > 0 else profiles_list
    all_prom_org = all(r.split("/", 1)[0].lower() in PROMETHEUS_ORGS for r in repos)
    role_desc = (
        "[Prometheus Governance](https://github.com/prometheus/governance/blob/main/ROLES.md)"
        if all_prom_org
        else "repository roles"
    )

    lines = [
        f"# Contributor trust report ({repos_str})",
        "",
        f"> **Snapshot**: `{now_str}` · **Window**: `{window_days}d` (`{window_days / 30.0:.1f}` months) · **Active/seen contributors**: `{len(profiles_list)}`",
        ">",
        f"> **Formula**: `role_base + min(40, 12 * log2(1 + weighted_rma))` where `weighted_rma = (1.0 * merged + 1.5 * reviewed) / {window_days / 30.0:.1f}` and roles follow {role_desc} (`Maintainer=50`, `Member=35`, `Contributor=15`, `New Contributor=0`).",
        "",
    ]

    if not shown:
        lines.append("_No matching contributors found._")
        return "\n".join(lines)

    lines.append(
        f"| Rank | Contributor | Role | Merged ({window_days}d) | Reviewed ({window_days}d) | Weighted / mo | Bonus | Trust Score |"
    )
    lines.append("| :---: | :--- | :--- | :---: | :---: | :---: | :---: | :---: |")
    for idx, p in enumerate(shown, start=1):
        lines.append(
            f"| {idx} | `@{p['login']}` | {p['role']} (`{int(p['role_base'])}`) | {p['merged_count']} (`{p['merged_rma']:.1f}/mo`) | {p['reviewed_count']} (`{p['reviewed_rma']:.1f}/mo`) | `{p['weighted_rma']:.2f}` | `+{p['contrib_bonus']:.1f}` | **{p['trust_score']:.1f}** |"
        )
    if limit > 0 and len(profiles_list) > limit:
        lines.append("")
        lines.append(f"_Showing top {limit} of {len(profiles_list)} contributors._")
    return "\n".join(lines)


def detect_default_repo():
    out = run_cmd(["gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"])
    if out and "/" in out.strip():
        return out.strip()
    return "prometheus/prometheus"


def main():
    parser = argparse.ArgumentParser(
        description="Compute contributor trust scores using repository governance roles and rolling monthly contributions."
    )
    parser.add_argument(
        "--repo",
        "-r",
        action="append",
        help="Target GitHub repository in owner/name format (repeatable). Defaults to current repo or prometheus/prometheus.",
    )
    parser.add_argument(
        "--window-days",
        "-w",
        type=int,
        default=DEFAULT_WINDOW_DAYS,
        help=f"Rolling window in days for the monthly contribution average (default: {DEFAULT_WINDOW_DAYS}).",
    )
    parser.add_argument(
        "--user",
        "-u",
        action="append",
        help="Filter output to specific GitHub handle(s).",
    )
    parser.add_argument(
        "--role",
        choices=[ROLE_MAINTAINER, ROLE_MEMBER, ROLE_CONTRIBUTOR, ROLE_NEW_CONTRIBUTOR],
        help="Filter output to a specific role.",
    )
    parser.add_argument(
        "--limit",
        "-l",
        type=int,
        default=50,
        help="Max contributors to show in Markdown output (default: 50, 0 for all).",
    )
    parser.add_argument(
        "--format",
        "-f",
        choices=["markdown", "json"],
        default="markdown",
        help="Output format (default: markdown).",
    )
    parser.add_argument(
        "--output",
        "-o",
        help="Optional file path to write output.",
    )
    args = parser.parse_args()

    repos = args.repo if args.repo else [detect_default_repo()]
    now = datetime.datetime.now(datetime.timezone.utc)

    repo_profiles_map = {}
    for repo_full in repos:
        owner, name = repo_full.split("/", 1)
        repo_profiles_map[repo_full] = fetch_repo_contributor_profiles(
            owner,
            name,
            now=now,
            window_days=args.window_days,
        )

    if len(repos) == 1:
        profiles = repo_profiles_map[repos[0]]
    else:
        profiles = merge_multi_repo_profiles(repo_profiles_map, window_days=args.window_days)

    user_filter = {u.lstrip("@").lower() for u in (args.user or [])}
    if user_filter:
        for u_low in user_filter:
            if u_low not in profiles:
                maint_any = any(
                    u_low == r.split("/", 1)[0].lower()
                    or u_low in fetch_repo_maintainer_handles(*r.split("/", 1))
                    for r in repos
                )
                role = ROLE_MAINTAINER if maint_any else ROLE_NEW_CONTRIBUTOR
                base = ROLE_BASE_SCORES[role]
                profiles[u_low] = {
                    "login": u_low,
                    "role": role,
                    "role_base": base,
                    "author_association": "NONE",
                    **compute_trust_score(base, 0, 0, window_days=args.window_days),
                }

    items = list(profiles.values())
    if user_filter:
        items = [p for p in items if p["login"].lower() in user_filter]
    if args.role:
        items = [p for p in items if p["role"] == args.role]

    items.sort(
        key=lambda p: (
            -p["trust_score"],
            -p["weighted_rma"],
            -ROLE_RANK[p["role"]],
            p["login"].lower(),
        )
    )

    if args.format == "json":
        rendered = json.dumps(
            {
                "generated_at": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
                "repos": repos,
                "window_days": args.window_days,
                "contributors": items if args.limit <= 0 else items[: args.limit],
            },
            indent=2,
        )
    else:
        rendered = render_markdown_report(repos, now, args.window_days, items, args.limit)

    if args.output:
        out_abs = os.path.abspath(args.output)
        os.makedirs(os.path.dirname(out_abs), exist_ok=True)
        with open(out_abs, "w", encoding="utf-8") as f:
            f.write(rendered + "\n")
        print(f"Wrote contributor trust report ({len(items)} contributors) to {args.output}", file=sys.stderr)
    else:
        print(rendered)


if __name__ == "__main__":
    main()
