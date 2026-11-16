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
"""Unit tests for generate_triage_board.py."""
import datetime
import importlib.util
import io
import json
import os
import shutil
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import generate_triage_board as gtb  # noqa: E402


class TestGenerateTriageBoard(unittest.TestCase):
    def test_cli_default_limit_includes_older_prometheus_pr(self):
        # GitHub returns all open PRs by updatedAt, including bots and drafts.
        raw_prs = []
        for index in range(160):
            number = 20100 if index == 140 else index + 1
            updated = (
                datetime.datetime(2026, 11, 16, tzinfo=datetime.timezone.utc)
                - datetime.timedelta(hours=index)
            ).isoformat()
            raw_prs.append({
                "number": number,
                "title": "promtool: support CFAccess",
                "url": f"https://github.com/prometheus/prometheus/pull/{number}",
                "author": {"login": "dependabot[bot]" if index == 0 else "jaymzh"},
                "authorAssociation": "CONTRIBUTOR",
                "updatedAt": updated,
                "isDraft": index == 1,
                "mergeable": "MERGEABLE",
                "commits": {"nodes": [{"commit": {
                    "message": "promtool: support CFAccess\n\nSigned-off-by: Jay <jay@example.com>",
                    "committedDate": updated,
                    "statusCheckRollup": {"state": "SUCCESS"},
                }}]},
            })

        for flags, expected_limit in (([], 150), (["--limit", "100"], 100), (["-l", "151"], 151)):
            with self.subTest(flags=flags):
                pages = []

                def github_page(cmd):
                    fields = dict(arg.split("=", 1) for arg in cmd if "=" in arg)
                    start = int(fields.get("after", "0"))
                    end = min(start + int(fields["limit"]), len(raw_prs))
                    pages.append((start, end))
                    return json.dumps({"data": {"repository": {"pullRequests": {
                        "totalCount": len(raw_prs),
                        "nodes": raw_prs[start:end],
                        "pageInfo": {"hasNextPage": end < len(raw_prs), "endCursor": str(end)},
                    }}}})

                output = io.StringIO()
                with (
                    mock.patch.object(sys, "argv", ["generate_triage_board.py", "--repo", "prometheus/prometheus", *flags]),
                    mock.patch.object(gtb, "run_cmd", side_effect=github_page),
                    mock.patch.object(gtb, "fetch_file_from_repo", return_value=None),
                    mock.patch.object(gtb.sc, "fetch_repo_contributor_profiles", return_value={}),
                    mock.patch.object(sys, "stdout", output),
                ):
                    gtb.main()

                md = output.getvalue()
                if expected_limit >= 141:
                    self.assertTrue("[#20100](" in md, "PR #20100 at updatedAt rank 141 fell off the default board")
                else:
                    self.assertNotIn("[#20100](", md)
                self.assertEqual(pages[-1][1], expected_limit)
                self.assertIn(f"up to {expected_limit} most recently active per repo", md)
                self.assertNotIn("[#1](", md)
                self.assertNotIn("[#2](", md)
                self.assertIn(f"[#{expected_limit}](", md)
                self.assertNotIn(f"[#{expected_limit + 1}](", md)

    def test_waiting_queue_sorted_by_approved_then_trust_plus_aging(self):
        now = datetime.datetime(2026, 11, 2, 5, 0, tzinfo=datetime.timezone.utc)
        prs = [
            {
                "repo_full": "prometheus/prometheus",
                "repo": "prometheus",
                "number": 1,
                "title": "feat: fresh new contributor PR",
                "url": "https://github.com/prometheus/prometheus/pull/1",
                "author": "newbie",
                "author_cell": "@newbie<br>*(New Contributor)*",
                "trust_score": 30.0,
                "priority_score": gtb.sc.compute_priority_score(
                    gtb.sc.effective_queue_trust_score(
                        {"role": "New Contributor", "trust_score": 0.0}
                    ),
                    1.0,
                ),
                "owners_cell": "`@bwplotka`",
                "status_bucket": "Waiting on Maintainer",
                "ci_state": "SUCCESS",
                "ci_explained": False,
                "hygiene": [],
                "additions": 100,
                "deletions": 10,
                "loc": 110,
                "changedFiles": 2,
                "updatedAt": "2026-11-01T05:00:00Z",
                "commitDate": "2026-11-01T05:00:00Z",
                "lastActor": "newbie",
                "lastEventType": "commit",
                "approved": False,
                "touchesWorkflow": False,
            },
            {
                "repo_full": "prometheus/prometheus",
                "repo": "prometheus",
                "number": 2,
                "title": "fix: maintainer PR pushed today",
                "url": "https://github.com/prometheus/prometheus/pull/2",
                "author": "bwplotka",
                "author_cell": "@bwplotka<br>*(Maintainer, 56.2/mo)*",
                "trust_score": 90.0,
                "priority_score": gtb.sc.compute_priority_score(90.0, 0.0),
                "owners_cell": "`@roidelapluie`",
                "status_bucket": "Waiting on Maintainer",
                "ci_state": "SUCCESS",
                "ci_explained": False,
                "hygiene": [],
                "additions": 80,
                "deletions": 20,
                "loc": 100,
                "changedFiles": 3,
                "updatedAt": "2026-11-02T05:00:00Z",
                "commitDate": "2026-11-02T05:00:00Z",
                "lastActor": "bwplotka",
                "lastEventType": "commit",
                "approved": False,
                "touchesWorkflow": False,
            },
            {
                "repo_full": "prometheus/prometheus",
                "repo": "prometheus",
                "number": 3,
                "title": "docs: approved contributor PR",
                "url": "https://github.com/prometheus/prometheus/pull/3",
                "author": "contrib",
                "author_cell": "@contrib<br>*(Contributor)*",
                "trust_score": 15.0,
                "priority_score": gtb.sc.compute_priority_score(15.0, 2.0),
                "owners_cell": "`@bboreham`",
                "status_bucket": "Waiting on Maintainer",
                "ci_state": "SUCCESS",
                "ci_explained": False,
                "hygiene": [],
                "additions": 70,
                "deletions": 5,
                "loc": 75,
                "changedFiles": 1,
                "updatedAt": "2026-10-31T05:00:00Z",
                "commitDate": "2026-10-31T05:00:00Z",
                "lastActor": "bboreham",
                "lastEventType": "review:APPROVED",
                "approved": True,
                "touchesWorkflow": False,
            },
            {
                "repo_full": "prometheus/prometheus",
                "repo": "prometheus",
                "number": 4,
                "title": "fix: long-waiting new contributor PR (10d)",
                "url": "https://github.com/prometheus/prometheus/pull/4",
                "author": "patient",
                "author_cell": "@patient<br>*(New Contributor)*",
                "trust_score": 30.0,
                # 30.0 welcome + 1.5 * 10 = 45.0, bubbles above fresh New Contributor (#1, 31.5) and inactive Contributor (#5, 15.0)
                "priority_score": gtb.sc.compute_priority_score(30.0, 10.0),
                "owners_cell": "`@codesome`",
                "status_bucket": "Waiting on Maintainer",
                "ci_state": "SUCCESS",
                "ci_explained": False,
                "hygiene": [],
                "additions": 90,
                "deletions": 10,
                "loc": 100,
                "changedFiles": 2,
                "updatedAt": "2026-10-23T05:00:00Z",
                "commitDate": "2026-10-23T05:00:00Z",
                "lastActor": "patient",
                "lastEventType": "commit",
                "approved": False,
                "touchesWorkflow": False,
            },
            {
                "repo_full": "prometheus/prometheus",
                "repo": "prometheus",
                "number": 5,
                "title": "chore: fresh PR from inactive returning contributor",
                "url": "https://github.com/prometheus/prometheus/pull/5",
                "author": "returning",
                "author_cell": "@returning<br>*(Contributor)*",
                "trust_score": 15.0,
                "priority_score": gtb.sc.compute_priority_score(15.0, 0.0),
                "owners_cell": "`@codesome`",
                "status_bucket": "Waiting on Maintainer",
                "ci_state": "SUCCESS",
                "ci_explained": False,
                "hygiene": [],
                "additions": 75,
                "deletions": 5,
                "loc": 80,
                "changedFiles": 1,
                "updatedAt": "2026-11-02T05:00:00Z",
                "commitDate": "2026-11-02T05:00:00Z",
                "lastActor": "returning",
                "lastEventType": "commit",
                "approved": False,
                "touchesWorkflow": False,
            },
        ]

        md = gtb.render_markdown(
            ["prometheus/prometheus"],
            now,
            {"prometheus/prometheus": 5},
            prs,
            100,
        )
        # Expected order: #3 (approved), #2 (Maintainer, 90.0), #4 (New Contributor aged 10d, 45.0), #1 (New Contributor 1d, 31.5), #5 (inactive Contributor 0d, 15.0).
        pos_3 = md.index("[#3](")
        pos_2 = md.index("[#2](")
        pos_4 = md.index("[#4](")
        pos_1 = md.index("[#1](")
        pos_5 = md.index("[#5](")
        self.assertLess(pos_3, pos_2)
        self.assertLess(pos_2, pos_4)
        self.assertLess(pos_4, pos_1)
        self.assertLess(pos_1, pos_5)

    def test_non_maintainer_buckets_ignore_stale_commit_age(self):
        now = datetime.datetime(2026, 11, 2, 5, 0, tzinfo=datetime.timezone.utc)
        prs = [
            {
                "repo_full": "prometheus/prometheus",
                "repo": "prometheus",
                "number": 10,
                "title": "stale conflicting PR from 2023",
                "url": "https://github.com/prometheus/prometheus/pull/10",
                "author": "oldcontrib",
                "author_cell": "@oldcontrib<br>*(Contributor)*",
                "trust_score": 15.0,
                "priority_score": 15.0,
                "waitingSince": "2023-01-01T00:00:00Z",
                "updatedEpoch": datetime.datetime(
                    2026, 10, 28, tzinfo=datetime.timezone.utc
                ).timestamp(),
                "owners_cell": "`@bwplotka`",
                "status_bucket": "Needs Rebase",
                "ci_state": "FAILURE",
                "ci_explained": False,
                "hygiene": ["merge-conflict"],
                "additions": 50,
                "deletions": 10,
                "loc": 60,
                "changedFiles": 1,
                "updatedAt": "2026-10-28T00:00:00Z",
                "commitDate": "2023-01-01T00:00:00Z",
                "lastActor": "oldcontrib",
                "lastEventType": "comment",
                "approved": False,
                "touchesWorkflow": False,
            },
            {
                "repo_full": "prometheus/prometheus",
                "repo": "prometheus",
                "number": 11,
                "title": "active maintainer PR needing rebase",
                "url": "https://github.com/prometheus/prometheus/pull/11",
                "author": "aknuds1",
                "author_cell": "@aknuds1<br>*(Maintainer, 102.8/mo)*",
                "trust_score": 90.0,
                "priority_score": 90.0,
                "waitingSince": "2026-10-30T00:00:00Z",
                "updatedEpoch": datetime.datetime(
                    2026, 10, 30, tzinfo=datetime.timezone.utc
                ).timestamp(),
                "owners_cell": "`@bwplotka`",
                "status_bucket": "Needs Rebase",
                "ci_state": "SUCCESS",
                "ci_explained": False,
                "hygiene": ["merge-conflict"],
                "additions": 200,
                "deletions": 20,
                "loc": 220,
                "changedFiles": 4,
                "updatedAt": "2026-10-30T00:00:00Z",
                "commitDate": "2026-10-30T00:00:00Z",
                "lastActor": "aknuds1",
                "lastEventType": "commit",
                "approved": False,
                "touchesWorkflow": False,
            },
        ]

        md = gtb.render_markdown(
            ["prometheus/prometheus"],
            now,
            {"prometheus/prometheus": 2},
            prs,
            100,
        )
        self.assertLess(md.index("[#11]("), md.index("[#10]("))

    def test_standalone_without_contributor_trust_skill(self):
        src_script = os.path.join(
            os.path.dirname(os.path.abspath(__file__)), "generate_triage_board.py"
        )
        with tempfile.TemporaryDirectory() as tmpdir:
            isolated_dir = os.path.join(tmpdir, "pr-triage-board", "scripts")
            os.makedirs(isolated_dir, exist_ok=True)
            isolated_script = os.path.join(isolated_dir, "generate_triage_board.py")
            shutil.copy2(src_script, isolated_script)

            saved_mod = sys.modules.pop("score_contributors", None)
            saved_path = list(sys.path)
            sys.path = [p for p in sys.path if "contributor" not in p]
            try:
                spec = importlib.util.spec_from_file_location(
                    "isolated_triage_board", isolated_script
                )
                mod = importlib.util.module_from_spec(spec)
                spec.loader.exec_module(mod)
            finally:
                sys.path = saved_path
                if saved_mod is not None:
                    sys.modules["score_contributors"] = saved_mod

            # Verify no colocated contributor-trust module was loaded.
            self.assertIsNone(mod._sc_mod)

            # Verify built-in standalone scorer matches role, table MAINTAINERS.md, OWNERS, and trust math.
            gen, _, all_m = mod.sc.parse_maintainers_md(
                "| Giedrius | `@Giedrius Statkevičius` | [@GiedriusS](https://github.com/GiedriusS) |\n## Triage\n| T | [@t](https://github.com/t) |\n"
            )
            self.assertEqual(gen, ["GiedriusS"])
            self.assertEqual(all_m, ["GiedriusS"])

            approvers = mod.sc.parse_owners_yaml(
                "approvers:\n  - team-a\n", "aliases:\n  team-a:\n    - dashpole\n"
            )
            self.assertEqual(approvers, ["dashpole"])

            role, base = mod.sc.classify_governance_role(
                "bwplotka", "OWNER", set(), repo_owner="bwplotka"
            )
            self.assertEqual((role, base), ("Maintainer", 50.0))

            self.assertEqual(
                mod.sc.effective_queue_trust_score(
                    {"role": "New Contributor", "trust_score": 0.0}
                ),
                30.0,
            )
            score = mod.sc.compute_trust_score(15.0, merged_count=8, reviewed_count=0, window_days=90)
            self.assertAlmostEqual(score["trust_score"], 37.5, places=1)
            self.assertEqual(
                mod.sc.compute_priority_score(score["trust_score"], waiting_days=10.0),
                52.5,
            )

            now = datetime.datetime(2026, 11, 2, 5, 0, tzinfo=datetime.timezone.utc)
            non_prom_md = mod.render_markdown(
                ["thanos-io/thanos"], now, {"thanos-io/thanos": 0}, [], 50
            )
            self.assertIn("Generated via [`.agents/skills/pr-triage-board`]", non_prom_md)
            self.assertIn("repository roles", non_prom_md)
            self.assertIn("`30` welcome baseline", non_prom_md)
            self.assertNotIn("07:00–18:00 CET", non_prom_md)

            pr_no_dco = {
                "title": "feat: add foo",
                "body": "",
                "mergeable": "MERGEABLE",
                "commits": {"nodes": [{"commit": {"message": "feat: add foo"}}]},
            }
            self.assertEqual(
                mod.check_hygiene(
                    {"requires_dco": False, "requires_area_title": False, "requires_release_notes": False},
                    pr_no_dco,
                ),
                [],
            )
            self.assertEqual(
                mod.check_hygiene(
                    {"requires_dco": True, "requires_area_title": False, "requires_release_notes": False},
                    pr_no_dco,
                ),
                ["missing-dco"],
            )


if __name__ == "__main__":
    unittest.main()
