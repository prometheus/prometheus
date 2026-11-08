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
from concurrent.futures import ThreadPoolExecutor
import datetime
import fnmatch
import json
import math
import os
import re
import subprocess
import sys
import time
import types

_sc_mod = None
if os.environ.get("PR_TRIAGE_FORCE_STANDALONE") != "1":
    _script_dir = os.path.dirname(os.path.abspath(__file__))
    for _cand in ("contributor-trust", "contributor_trust"):
        _cand_scripts = os.path.normpath(os.path.join(_script_dir, "..", "..", _cand, "scripts"))
        if os.path.isdir(_cand_scripts):
            if _cand_scripts not in sys.path:
                sys.path.insert(0, _cand_scripts)
            try:
                import score_contributors as _sc_mod  # noqa: E402
                break
            except ImportError:
                _sc_mod = None


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


def _build_standalone_scorer():
    """Provides built-in contributor trust scoring when contributor-trust is not installed."""
    role_maintainer = "Maintainer"
    role_member = "Member"
    role_contributor = "Contributor"
    role_new = "New Contributor"
    role_bases = {
        role_maintainer: 50.0,
        role_member: 35.0,
        role_contributor: 15.0,
        role_new: 0.0,
    }
    role_rank = {role_new: 0, role_contributor: 1, role_member: 2, role_maintainer: 3}
    assoc_rank = {
        "NONE": 0,
        "FIRST_TIMER": 1,
        "FIRST_TIME_CONTRIBUTOR": 1,
        "CONTRIBUTOR": 2,
        "COLLABORATOR": 3,
        "MEMBER": 4,
        "OWNER": 5,
    }
    default_window_days = 90
    activity_query = """
    query($owner: String!, $name: String!, $states: [PullRequestState!]!, $after: String) {
      repository(owner: $owner, name: $name) {
        pullRequests(states: $states, first: 100, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
          pageInfo { hasNextPage endCursor }
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

    def extract_handles(line):
        gh_links = [
            h
            for h in re.findall(r"github\.com/([a-zA-Z0-9-]+)(?:[/?#)\]\s]|$)", line)
            if h.lower() not in RESERVED_GH_PATHS
        ]
        if gh_links:
            return list(dict.fromkeys(gh_links))
        cleaned = re.sub(r"`[^`]*\s[^`]*`", " ", line)
        return list(dict.fromkeys(re.findall(r"(?:^|[\s/(\[|`])@([a-zA-Z0-9-]+)", cleaned)))

    def parse_maintainers_md(content):
        general, specific, all_maintainers = [], [], []
        if not content:
            return general, specific, all_maintainers
        mode = "general"
        prefix_stack = []
        for line in content.splitlines():
            stripped = line.strip()
            low = stripped.lower()
            if not stripped:
                continue
            if low.startswith("general maintainers") or "core maintainers" in low:
                mode, prefix_stack = "general", []
                continue
            if low.startswith("maintainers for specific") or "named maintainers" in low:
                mode, prefix_stack = "specific", []
                continue
            if (
                low.startswith("v3 release")
                or low.startswith("release coordinators")
                or low.startswith("for the sake of")
                or (
                    low.startswith("#")
                    and any(
                        s in low for s in ("triage", "alumni", "emeritus", "retired", "former")
                    )
                )
                or low.startswith("emeritus")
            ):
                mode, prefix_stack = None, []
                continue
            if mode is None:
                continue
            if stripped.startswith("|") and re.match(r"^\|[\s:|-]+\|?$", stripped):
                continue
            handles = extract_handles(line)
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
        rules, default_individuals, all_individuals = [], [], []
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
        if not owners_content:
            return []
        aliases = {}
        if aliases_content:
            current_alias, in_aliases = None, False
            for raw in aliases_content.splitlines():
                line = raw.split("#", 1)[0].rstrip()
                stripped = line.strip()
                if not stripped:
                    continue
                if len(line) == len(line.lstrip(" ")):
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
        approvers, in_approvers = [], False
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
                for h in aliases.get(token, [token]):
                    if h and "/" not in h and not is_bot(h) and h not in approvers:
                        approvers.append(h)
        return approvers

    def classify_governance_role(
        login, author_association, maintainer_handles_lower, has_accepted_work=False, repo_owner=None
    ):
        assoc = (author_association or "NONE").upper()
        if (
            (login and login.lower() in maintainer_handles_lower)
            or assoc == "OWNER"
            or (login and repo_owner and login.lower() == repo_owner.lower())
        ):
            return role_maintainer, role_bases[role_maintainer]
        if assoc in ("MEMBER", "COLLABORATOR"):
            return role_member, role_bases[role_member]
        if assoc == "CONTRIBUTOR" or has_accepted_work:
            return role_contributor, role_bases[role_contributor]
        return role_new, role_bases[role_new]

    def compute_trust_score(role_base, merged_count, reviewed_count, window_days=default_window_days):
        months = max(1.0 / 30.0, window_days / 30.0)
        weighted_rma = ((1.0 * merged_count) + (1.5 * reviewed_count)) / months
        contrib_bonus = min(40.0, 12.0 * math.log2(1.0 + weighted_rma))
        return {
            "merged_count": merged_count,
            "reviewed_count": reviewed_count,
            "merged_rma": round(merged_count / months, 2),
            "reviewed_rma": round(reviewed_count / months, 2),
            "weighted_rma": round(weighted_rma, 2),
            "contrib_bonus": round(contrib_bonus, 1),
            "trust_score": round(role_base + contrib_bonus, 1),
        }

    def effective_queue_trust_score(profile, new_contributor_welcome=30.0):
        if profile.get("role") == role_new:
            return max(profile["trust_score"], new_contributor_welcome)
        return profile["trust_score"]

    def compute_priority_score(trust_score, waiting_days, aging_per_day=1.5):
        return round(trust_score + (aging_per_day * max(0.0, waiting_days)), 2)

    def format_author_cell(login, profile):
        role = profile["role"]
        w_rma = profile["weighted_rma"]
        return f"@{login}<br>*({role}, {w_rma:.1f}/mo)*" if w_rma >= 0.1 else f"@{login}<br>*({role})*"

    def fetch_stream(owner, name, states, cutoff_iso):
        nodes, cursor = [], None
        while True:
            cmd = [
                "gh",
                "api",
                "graphql",
                "-f",
                f"query={activity_query}",
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
                raise RuntimeError(f"Failed to fetch PR activity for {owner}/{name}")
            repo_data = (json.loads(out).get("data") or {}).get("repository")
            if not repo_data:
                raise RuntimeError(f"Empty repository payload for {owner}/{name}")
            conn = repo_data["pullRequests"]
            batch = conn["nodes"]
            if not batch:
                break
            nodes.extend(batch)
            if batch[-1]["updatedAt"] < cutoff_iso or not conn["pageInfo"]["hasNextPage"]:
                break
            cursor = conn["pageInfo"]["endCursor"]
        return nodes

    def infer_maintainers(pr_nodes, cutoff_iso):
        merged_others, merge_counts = set(), {}
        for pr in pr_nodes:
            if not pr or pr.get("state") != "MERGED" or (pr.get("mergedAt") or "") < cutoff_iso:
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
        return {m for m, cnt in merge_counts.items() if m in merged_others or cnt >= 2}

    def aggregate_profiles(pr_nodes, maintainer_handles_lower, cutoff_iso, window_days=default_window_days, repo_owner=None):
        effective_maintainers = set(maintainer_handles_lower or ()) or infer_maintainers(pr_nodes, cutoff_iso)
        canonical_login, best_assoc, merged_prs, reviewed_prs = {}, {}, {}, {}

        def record_user(login, assoc):
            low = login.lower()
            canonical_login.setdefault(low, login)
            prev = best_assoc.get(low, "NONE")
            if assoc_rank.get((assoc or "NONE").upper(), 0) >= assoc_rank.get(prev, 0):
                best_assoc[low] = (assoc or "NONE").upper()
            return low

        seen = set()
        for pr in pr_nodes:
            if not pr or pr.get("number") in seen:
                continue
            seen.add(pr["number"])
            author_obj = pr.get("author")
            pr_author_low = ""
            if author_obj and not is_bot(author_obj):
                pr_author_low = record_user(author_obj["login"], pr.get("authorAssociation"))
                if pr.get("state") == "MERGED" and (pr.get("mergedAt") or "") >= cutoff_iso:
                    merged_prs.setdefault(pr_author_low, set()).add(pr["number"])
            merger_obj = pr.get("mergedBy")
            if merger_obj and not is_bot(merger_obj):
                record_user(merger_obj["login"], "MEMBER")
            for rev in (pr.get("reviews") or {}).get("nodes") or []:
                r_author = rev.get("author")
                if not r_author or is_bot(r_author) or (rev.get("submittedAt") or "") < cutoff_iso:
                    continue
                r_low = record_user(r_author["login"], rev.get("authorAssociation"))
                if r_low != pr_author_low:
                    reviewed_prs.setdefault(r_low, set()).add(pr["number"])

        profiles = {}
        for low, login in canonical_login.items():
            m_cnt, r_cnt = len(merged_prs.get(low, ())), len(reviewed_prs.get(low, ()))
            assoc = best_assoc.get(low, "NONE")
            role, base = classify_governance_role(
                login, assoc, effective_maintainers, (m_cnt + r_cnt) > 0, repo_owner=repo_owner
            )
            profiles[low] = {
                "login": login,
                "role": role,
                "role_base": base,
                "author_association": assoc,
                **compute_trust_score(base, m_cnt, r_cnt, window_days=window_days),
            }
        return profiles

    def fetch_repo_contributor_profiles(
        owner, name, now=None, window_days=default_window_days, maintainer_handles_lower=None
    ):
        if now is None:
            now = datetime.datetime.now(datetime.timezone.utc)
        cutoff_iso = (now - datetime.timedelta(days=window_days)).strftime("%Y-%m-%dT%H:%M:%SZ")
        with ThreadPoolExecutor(max_workers=2) as pool:
            f_closed = pool.submit(fetch_stream, owner, name, ["MERGED", "CLOSED"], cutoff_iso)
            f_open = pool.submit(fetch_stream, owner, name, ["OPEN"], cutoff_iso)
            pr_nodes = f_closed.result() + f_open.result()
        return aggregate_profiles(
            pr_nodes, maintainer_handles_lower, cutoff_iso, window_days=window_days, repo_owner=owner
        )

    def resolve_author_profile(
        profiles, login, author_association, maintainer_handles_lower, window_days=default_window_days, repo_owner=None
    ):
        low = login.lower()
        existing = profiles.get(low)
        if existing:
            role, base = classify_governance_role(
                login,
                author_association,
                maintainer_handles_lower,
                (existing["merged_count"] + existing["reviewed_count"]) > 0,
                repo_owner=repo_owner,
            )
            if role_rank[role] > role_rank[existing["role"]]:
                return {
                    **existing,
                    "role": role,
                    "role_base": base,
                    **compute_trust_score(
                        base, existing["merged_count"], existing["reviewed_count"], window_days=window_days
                    ),
                }
            return existing
        role, base = classify_governance_role(
            login, author_association, maintainer_handles_lower, False, repo_owner=repo_owner
        )
        return {
            "login": login,
            "role": role,
            "role_base": base,
            "author_association": (author_association or "NONE").upper(),
            **compute_trust_score(base, 0, 0, window_days=window_days),
        }

    return types.SimpleNamespace(
        ROLE_MAINTAINER=role_maintainer,
        DEFAULT_WINDOW_DAYS=default_window_days,
        parse_maintainers_md=parse_maintainers_md,
        parse_codeowners=parse_codeowners,
        parse_owners_yaml=parse_owners_yaml,
        classify_governance_role=classify_governance_role,
        compute_trust_score=compute_trust_score,
        effective_queue_trust_score=effective_queue_trust_score,
        compute_priority_score=compute_priority_score,
        format_author_cell=format_author_cell,
        fetch_repo_contributor_profiles=fetch_repo_contributor_profiles,
        resolve_author_profile=resolve_author_profile,
    )


sc = _sc_mod if _sc_mod is not None else _build_standalone_scorer()


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
        author { __typename login }
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
            author { __typename login }
            state
            submittedAt
            body
          }
        }
        latestOpinionatedReviews(first: 10) {
          nodes {
            author { __typename login }
            state
            submittedAt
            body
          }
        }
        comments(last: 10) {
          totalCount
          nodes {
            author { __typename login }
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
                author { __typename login }
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
    owners_txt = fetch_file_from_repo(owner, repo, "OWNERS") or ""
    aliases_txt = (
        fetch_file_from_repo(owner, repo, "OWNERS_ALIASES") if owners_txt else ""
    ) or ""
    pr_template_txt = (
        fetch_file_from_repo(owner, repo, ".github/PULL_REQUEST_TEMPLATE.md")
        or fetch_file_from_repo(owner, repo, "PULL_REQUEST_TEMPLATE.md")
        or ""
    )
    contributing_txt = (
        ""
        if owner.lower() in PROMETHEUS_ORGS
        else (
            fetch_file_from_repo(owner, repo, "CONTRIBUTING.md")
            or fetch_file_from_repo(owner, repo, ".github/CONTRIBUTING.md")
            or fetch_file_from_repo(owner, repo, "DCO")
            or ""
        )
    )

    default_co, co_rules, co_all = sc.parse_codeowners(codeowners_txt)
    gen_maint, spec_maint, maint_all = sc.parse_maintainers_md(maintainers_txt)
    owners_approvers = sc.parse_owners_yaml(owners_txt, aliases_txt)

    # Merge path rules (more specific paths first)
    path_rules = []
    for pat, indiv, _teams in co_rules:
        if indiv:
            path_rules.append((pat, indiv))
    for pat, indiv in spec_maint:
        if indiv and not any(p == pat for p, _ in path_rules):
            path_rules.append((pat, indiv))

    path_rules.sort(key=lambda x: len(x[0]), reverse=True)
    fallback_owners = default_co or gen_maint or owners_approvers
    maintainer_handles_lower = {h.lower() for h in (maint_all + co_all + owners_approvers)}

    policy_txt = (pr_template_txt + "\n" + contributing_txt).lower()
    requires_dco = owner.lower() in PROMETHEUS_ORGS or bool(
        re.search(
            r"(\bdco\b|signed-off-by|developer certificate of origin|git commit -s)",
            policy_txt,
        )
    )
    requires_release_notes = "```release-notes" in pr_template_txt
    requires_area_title = "area: short description" in pr_template_txt

    return {
        "path_rules": path_rules,
        "fallback_owners": fallback_owners,
        "maintainer_handles_lower": maintainer_handles_lower,
        "requires_dco": requires_dco,
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
        r_author = r.get("author") or {}
        login = r_author.get("login")
        if login and login.lower() != author.lower() and not is_bot(r_author):
            if login not in curr:
                curr.append(login)
    for n in pr.get("reviewRequests", {}).get("nodes", []):
        rr = n.get("requestedReviewer") or {}
        login = rr.get("login")
        if login and login.lower() != author.lower() and not is_bot(rr):
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
    if model.get("requires_dco", True) and "Signed-off-by:" not in last_msg:
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
        with ThreadPoolExecutor(max_workers=2) as pool:
            f_prs = pool.submit(fetch_repo_prs, owner, name, max_prs)
            f_profiles = pool.submit(
                sc.fetch_repo_contributor_profiles,
                owner,
                name,
                now,
                sc.DEFAULT_WINDOW_DAYS,
                model["maintainer_handles_lower"],
            )
            total_open, raw_prs = f_prs.result()
            contributor_profiles = f_profiles.result()
        repo_totals[repo_full] = total_open

        # When a repository has no explicit MAINTAINERS.md, CODEOWNERS, or OWNERS
        # file, populate fallback owners from maintainers inferred via merge history.
        if not model["fallback_owners"]:
            inferred = [
                p
                for p in contributor_profiles.values()
                if p["role"] == sc.ROLE_MAINTAINER
            ]
            inferred.sort(
                key=lambda p: (-p["trust_score"], -p["weighted_rma"], p["login"].lower())
            )
            model["fallback_owners"] = [p["login"] for p in inferred]
            model["maintainer_handles_lower"].update(p["login"].lower() for p in inferred)

        for pr in raw_prs:
            author = pr["author"]["login"] if pr.get("author") else "ghost"
            if is_bot(pr.get("author")):
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
                and not is_bot(r.get("author"))
            ]
            latest_reviews = [
                r
                for r in pr.get("latestReviews", {}).get("nodes", [])
                if r.get("state") != "PENDING"
                and r.get("author")
                and not is_bot(r.get("author"))
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
                if c.get("author") and not is_bot(c.get("author"))
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
                        and not is_bot(tc.get("author"))
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
                    and not is_bot(tc.get("author"))
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
            author_profile = sc.resolve_author_profile(
                contributor_profiles,
                author,
                pr.get("authorAssociation"),
                model["maintainer_handles_lower"],
                repo_owner=owner,
            )

            # Maintainer waiting time starts when the ball entered the maintainer's
            # court (the author's first commit/reply after the latest non-author event,
            # or commitDate if untouched), avoiding 100d+ inflation on old commits
            # that only recently received a comment reply.
            non_author_ts = [
                ts for ts, actor, _ in events if actor.lower() != author_low
            ]
            waiting_since = commit_date
            if non_author_ts:
                last_other_ts = max(non_author_ts)
                author_after = [
                    ts
                    for ts, actor, _ in events
                    if actor.lower() == author_low and ts >= last_other_ts
                ]
                if author_after:
                    waiting_since = min(author_after)

            waiting_dt = datetime.datetime.fromisoformat(waiting_since.replace("Z", "+00:00"))
            waiting_days = (
                max(0.0, (now - waiting_dt).total_seconds() / 86400.0)
                if status_bucket
                in ("Updated After Review", "Waiting on Maintainer", "Needs /workflow-approve")
                else 0.0
            )
            queue_trust_score = sc.effective_queue_trust_score(author_profile)
            priority_score = sc.compute_priority_score(
                queue_trust_score,
                waiting_days,
            )
            updated_epoch = datetime.datetime.fromisoformat(
                pr["updatedAt"].replace("Z", "+00:00")
            ).timestamp()
            all_prs.append(
                {
                    "repo_full": repo_full,
                    "repo": name,
                    "number": pr["number"],
                    "title": pr["title"],
                    "url": pr["url"],
                    "author": author,
                    "author_cell": sc.format_author_cell(author, author_profile),
                    "trust_score": queue_trust_score,
                    "priority_score": priority_score,
                    "waitingSince": waiting_since,
                    "updatedEpoch": updated_epoch,
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


def render_markdown(
    repos, now, repo_totals, prs, max_prs, maintainer_filter=None, automated=False
):
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
    all_prom_org = all(r.split("/", 1)[0].lower() in PROMETHEUS_ORGS for r in repos)

    skill_link = "[`.agents/skills/pr-triage-board`](https://github.com/prometheus/prometheus/tree/main/.agents/skills/pr-triage-board)"
    gen_line = (
        f"> **Automated via {skill_link}** · Refreshed **every hour on weekdays (Mon–Fri, 07:00–18:00 CET)**."
        if automated
        else f"> **Generated via {skill_link}**."
    )
    role_desc = (
        "[Prometheus Governance roles](https://github.com/prometheus/governance/blob/main/ROLES.md)"
        if all_prom_org
        else "repository roles (`Maintainer`, `Member`, `Contributor`, `New Contributor`)"
    )

    lines = [
        f"> **Auto-updated snapshot**: `{now_str}` · Analyzed `{len(prs)}` non-bot open PRs (up to {max_prs} most recently active per repo).",
        ">",
        gen_line,
        ">",
        f"> **Queue priority**: Approved PRs first, then `trust_score + 1.5 * waiting_days` using {role_desc} + 90d monthly contribution rate (`1.0x` merged + `1.5x` reviewed PRs/mo; `New Contributor` starts at a `30` welcome baseline).",
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
        return (
            repo_order.get(x["repo_full"], 99),
            0 if x["approved"] else 1,
            -x.get("priority_score", 0.0),
            x.get("waitingSince", x["commitDate"]),
        )

    def non_maintainer_sort_key(x):
        return (
            repo_order.get(x["repo_full"], 99),
            0 if x["approved"] else 1,
            -x.get("trust_score", 0.0),
            -x.get("updatedEpoch", 0.0),
        )

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
            w_iso = p.get("waitingSince", p["commitDate"])
            lines.append(
                f"| {pr_link(p)}{app} | {clean_title(p['title'])} | {p['author_cell']} | `+{p['additions']}/-{p['deletions']}` ({p['changedFiles']}f) | {age_str(w_iso)} | {p['owners_cell']} |"
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
        lines.append("| PR | Title | Author | CI | Diff | Hygiene | Waiting | Owners |")
        lines.append("| :--- | :--- | :--- | :---: | :--- | :--- | :--- | :--- |")
        for p in updated:
            app = " *(approved)*" if p["approved"] else ""
            w_iso = p.get("waitingSince", p["commitDate"])
            lines.append(
                f"| {pr_link(p)}{app} | {clean_title(p['title'])} | {p['author_cell']} | `{fmt_ci(p['ci_state'], p.get('ci_explained', False))}` | `+{p['additions']}/-{p['deletions']}` | {fmt_hygiene(p['hygiene'])} | {age_str(w_iso)} | {p['owners_cell']} |"
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
            w_iso = p.get("waitingSince", p["commitDate"])
            lines.append(
                f"| {pr_link(p)} | {clean_title(p['title'])} | {p['author_cell']} | `+{p['additions']}/-{p['deletions']}` ({p['changedFiles']}f) | {safety} | {fmt_hygiene(p['hygiene'])} | {age_str(w_iso)} | {p['owners_cell']} |"
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

    lines.append("## Waiting on Maintainer (By Priority: Trust + Waiting Age)")
    lines.append("*All comments are resolved or responded to, CI is green (or the author commented why it is not green), and there are no merge conflicts.*")
    lines.append("")
    if waiting:
        lines.append("| PR | Title | Author | CI | Diff | Hygiene | Waiting Since | Owners |")
        lines.append("| :--- | :--- | :--- | :---: | :--- | :--- | :--- | :--- |")
        for p in waiting:
            app = " *(approved)*" if p["approved"] else ""
            w_iso = p.get("waitingSince", p["commitDate"])
            lines.append(
                f"| {pr_link(p)}{app} | {clean_title(p['title'])} | {p['author_cell']} | `{fmt_ci(p['ci_state'], p.get('ci_explained', False))}` | `+{p['additions']}/-{p['deletions']}` ({p['changedFiles']}f) | {fmt_hygiene(p['hygiene'])} | {age_str(w_iso)} ({w_iso[:10]}) | {p['owners_cell']} |"
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
        b_prs.sort(key=non_maintainer_sort_key)
        lines.append("<details>")
        lines.append(f"<summary><b>{b_title} ({len(b_prs)})</b>: {b_desc}</summary>")
        lines.append("")
        if b_prs:
            lines.append("| PR | Title | Author | Last Actor | Hygiene | Updated | Owners |")
            lines.append("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |")
            for p in b_prs:
                app = " *(approved)*" if p["approved"] else ""
                lines.append(
                    f"| {pr_link(p)}{app} | {clean_title(p['title'])} | {p['author_cell']} | @{p['lastActor']} ({p['lastEventType']}) | {fmt_hygiene(p['hygiene'])} | {age_str(p['updatedAt'])} | {p['owners_cell']} |"
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
    md = render_markdown(
        repos,
        now,
        repo_totals,
        prs,
        args.limit,
        args.maintainer,
        automated=bool(args.update_issue),
    )

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
