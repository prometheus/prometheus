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
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import generate_triage_board as gtb  # noqa: E402


class TestGenerateTriageBoard(unittest.TestCase):
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
                "trust_score": 0.0,
                "priority_score": gtb.sc.compute_priority_score(0.0, 1.0),
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
                "title": "fix: long-waiting new contributor PR (30d)",
                "url": "https://github.com/prometheus/prometheus/pull/4",
                "author": "patient",
                "author_cell": "@patient<br>*(New Contributor)*",
                "trust_score": 0.0,
                # 0 + 1.5 * 30 = 45.0, bubbles above fresh New Contributor (#1, 1.5)
                "priority_score": gtb.sc.compute_priority_score(0.0, 30.0),
                "owners_cell": "`@codesome`",
                "status_bucket": "Waiting on Maintainer",
                "ci_state": "SUCCESS",
                "ci_explained": False,
                "hygiene": [],
                "additions": 90,
                "deletions": 10,
                "loc": 100,
                "changedFiles": 2,
                "updatedAt": "2026-10-03T05:00:00Z",
                "commitDate": "2026-10-03T05:00:00Z",
                "lastActor": "patient",
                "lastEventType": "commit",
                "approved": False,
                "touchesWorkflow": False,
            },
        ]

        md = gtb.render_markdown(
            ["prometheus/prometheus"],
            now,
            {"prometheus/prometheus": 4},
            prs,
            100,
        )
        # Expected order in Waiting on Maintainer: #3 (approved), #2 (Maintainer, 90.0), #4 (aged 30d, 45.0), #1 (1d, 1.5).
        pos_3 = md.index("[#3](")
        pos_2 = md.index("[#2](")
        pos_4 = md.index("[#4](")
        pos_1 = md.index("[#1](")
        self.assertLess(pos_3, pos_2)
        self.assertLess(pos_2, pos_4)
        self.assertLess(pos_4, pos_1)

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


if __name__ == "__main__":
    unittest.main()
