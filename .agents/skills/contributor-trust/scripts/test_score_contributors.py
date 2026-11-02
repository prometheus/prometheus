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
"""Unit tests for score_contributors.py."""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import score_contributors as sc  # noqa: E402


class TestScoreContributors(unittest.TestCase):
    def test_parse_maintainers_md_variants(self):
        cases = [
            (
                "prometheus_nested",
                """# Maintainers
General maintainers:
* Bartłomiej Płotka (<bwplotka@gmail.com> / @bwplotka)
* Julien Pivotto (roidelapluie@prometheus.io / @roidelapluie)

Maintainers for specific parts of the codebase:
* `storage`
  * `remote`: Callum Styan (<callumstyan@gmail.com> / @cstyan)
    * `otlptranslator`: Arthur Silva Sens (<arthursens2005@gmail.com> / @ArthurSens)
* `web`
  * `ui`: Julius Volz (<julius.volz@gmail.com> / @juliusv)
    * `module`: Augustin Husson (<husson.augustin@gmail.com> / @nexucis)

## Emeritus
* Past Maintainer (@oldhandle)
""",
                ["bwplotka", "roidelapluie"],
                [
                    ("storage/remote", ["cstyan"]),
                    ("storage/remote/otlptranslator", ["ArthurSens"]),
                    ("web/ui", ["juliusv"]),
                    ("web/ui/module", ["nexucis"]),
                ],
                {"bwplotka", "roidelapluie", "cstyan", "ArthurSens", "juliusv", "nexucis"},
            ),
            (
                "client_golang_bullet_only",
                """* Arianna Vespri <arianna.vespri@proton.me> @vesari
* Bartłomiej Płotka <bwplotka@gmail.com> @bwplotka
""",
                ["vesari", "bwplotka"],
                [],
                {"vesari", "bwplotka"},
            ),
            (
                "compliance_named_maintainers",
                """There is currently no main/default maintainer, all parts of the codebase have named maintainers:

* `remote_write`: Bartek Płotka (<bwplotka@gmail.com> / @bwplotka)
* `promql`: Julius Volz (<julius.volz@gmail.com> / @juliusv)
""",
                [],
                [("remote_write", ["bwplotka"]), ("promql", ["juliusv"])],
                {"bwplotka", "juliusv"},
            ),
            (
                "thanos_markdown_table_with_triage_section",
                """## Maintainers

| Name | Email | Slack | GitHub | Company |
|---|---|---|---|---|
| Bartlomiej Plotka | bwplotka@gmail.com | `@bwplotka` | [@bwplotka](https://github.com/bwplotka) | Google |
| Giedrius Statkevičius | giedriuswork@gmail.com | `@Giedrius Statkevičius` | [@GiedriusS](https://github.com/GiedriusS) | Shopify |

## Triage
| Name | GitHub |
|---|---|
| Triager Person | [@triager1](https://github.com/triager1) |
""",
                ["bwplotka", "GiedriusS"],
                [],
                {"bwplotka", "GiedriusS"},
            ),
        ]

        for name, content, want_gen, want_spec, want_all in cases:
            with self.subTest(name=name):
                gen, spec, all_m = sc.parse_maintainers_md(content)
                self.assertEqual(gen, want_gen)
                self.assertEqual(spec, want_spec)
                self.assertEqual(set(all_m), want_all)

    def test_parse_owners_yaml(self):
        owners = """
# Kubernetes/Prow OWNERS file
approvers:
  - sig-instrumentation-approvers
  - dashpole
  - dependabot[bot]
reviewers:
  - reviewer-only
"""
        aliases = """
aliases:
  sig-instrumentation-approvers:
    - bwplotka
    - hanleybrand
"""
        self.assertEqual(
            sc.parse_owners_yaml(owners, aliases),
            ["bwplotka", "hanleybrand", "dashpole"],
        )

    def test_parse_codeowners(self):
        content = """
* @prometheus/default-maintainers
/discovery/aws/ @prometheus/default-maintainers @matt-gp @sysadmind
"""
        default_indiv, rules, all_indiv = sc.parse_codeowners(content)
        self.assertEqual(default_indiv, [])
        self.assertEqual(
            rules,
            [("discovery/aws/", ["matt-gp", "sysadmind"], ["prometheus/default-maintainers"])],
        )
        self.assertEqual(all_indiv, ["matt-gp", "sysadmind"])

    def test_classify_and_score_roles(self):
        maintainers = {"bwplotka", "matt-gp"}
        cases = [
            # Active maintainer hits the +40 contribution cap -> 90.0.
            ("bwplotka", "MEMBER", 28, 169, None, sc.ROLE_MAINTAINER, 90.0),
            # Subtree maintainer listed in CODEOWNERS with COLLABORATOR association.
            ("matt-gp", "COLLABORATOR", 1, 0, None, sc.ROLE_MAINTAINER, 55.0),
            # Active org member with 4 merges + 12 reviews in 90d ((4 + 18)/3 = 7.33/mo).
            ("dashpole", "MEMBER", 4, 12, None, sc.ROLE_MEMBER, 71.7),
            # Inactive org member gets role base 35.0.
            ("waltherlee", "MEMBER", 0, 0, None, sc.ROLE_MEMBER, 35.0),
            # Prolific external contributor (8 merges in 90d = 2.67/mo) surpasses inactive Member.
            ("fallintoplace", "CONTRIBUTOR", 8, 0, None, sc.ROLE_CONTRIBUTOR, 37.5),
            # Returning contributor with no merges/reviews in the last 90d keeps base 15.0.
            ("vijaygovindaraja", "CONTRIBUTOR", 0, 0, None, sc.ROLE_CONTRIBUTOR, 15.0),
            # Personal repository owner maps to Maintainer (50.0) even without MAINTAINERS.md.
            ("bwplotka", "OWNER", 0, 0, "bwplotka", sc.ROLE_MAINTAINER, 50.0),
            # First-time contributor starts at 0.0.
            ("newuser", "FIRST_TIME_CONTRIBUTOR", 0, 0, None, sc.ROLE_NEW_CONTRIBUTOR, 0.0),
        ]

        for login, assoc, merged, reviewed, repo_owner, want_role, want_score in cases:
            with self.subTest(login=login, assoc=assoc):
                role, base = sc.classify_governance_role(
                    login,
                    assoc,
                    maintainers if not repo_owner else set(),
                    has_accepted_work=(merged + reviewed) > 0,
                    repo_owner=repo_owner,
                )
                self.assertEqual(role, want_role)
                res = sc.compute_trust_score(base, merged, reviewed, window_days=90)
                self.assertAlmostEqual(res["trust_score"], want_score, places=1)

    def test_aggregate_excludes_self_reviews_bots_and_old_events(self):
        cutoff = "2026-08-04T00:00:00Z"
        pr_nodes = [
            {
                "number": 101,
                "state": "MERGED",
                "updatedAt": "2026-10-15T10:00:00Z",
                "mergedAt": "2026-10-15T10:00:00Z",
                "author": {"__typename": "User", "login": "alice"},
                "authorAssociation": "CONTRIBUTOR",
                "reviews": {
                    "nodes": [
                        # Self-review (inline reply on own PR) must be ignored.
                        {
                            "submittedAt": "2026-10-14T10:00:00Z",
                            "authorAssociation": "CONTRIBUTOR",
                            "author": {"__typename": "User", "login": "Alice"},
                        },
                        # Two review rounds by bob on the same PR count as 1 reviewed PR.
                        {
                            "submittedAt": "2026-10-13T10:00:00Z",
                            "authorAssociation": "MEMBER",
                            "author": {"__typename": "User", "login": "bob"},
                        },
                        {
                            "submittedAt": "2026-10-15T09:00:00Z",
                            "authorAssociation": "MEMBER",
                            "author": {"__typename": "User", "login": "bob"},
                        },
                        # Bot review must be ignored.
                        {
                            "submittedAt": "2026-10-14T11:00:00Z",
                            "authorAssociation": "NONE",
                            "author": {"__typename": "Bot", "login": "codecov"},
                        },
                        # Review before cutoff must be ignored.
                        {
                            "submittedAt": "2026-07-01T00:00:00Z",
                            "authorAssociation": "MEMBER",
                            "author": {"__typename": "User", "login": "carol"},
                        },
                    ]
                },
            },
            {
                # Merged before cutoff, updated recently via comment: merge must not count.
                "number": 99,
                "state": "MERGED",
                "updatedAt": "2026-10-10T00:00:00Z",
                "mergedAt": "2026-07-20T00:00:00Z",
                "author": {"__typename": "User", "login": "alice"},
                "authorAssociation": "CONTRIBUTOR",
                "reviews": {"nodes": []},
            },
        ]

        profiles = sc.aggregate_contributor_profiles(
            pr_nodes, maintainer_handles_lower={"bob"}, cutoff_iso=cutoff, window_days=90
        )
        self.assertEqual(profiles["alice"]["merged_count"], 1)
        self.assertEqual(profiles["alice"]["reviewed_count"], 0)
        self.assertEqual(profiles["bob"]["merged_count"], 0)
        self.assertEqual(profiles["bob"]["reviewed_count"], 1)
        self.assertEqual(profiles["bob"]["role"], sc.ROLE_MAINTAINER)
        self.assertNotIn("carol", profiles)
        self.assertNotIn("codecov", profiles)

    def test_infer_maintainers_when_repo_has_no_ownership_files(self):
        cutoff = "2026-08-04T00:00:00Z"
        pr_nodes = [
            {
                "number": 201,
                "state": "MERGED",
                "updatedAt": "2026-10-10T00:00:00Z",
                "mergedAt": "2026-10-10T00:00:00Z",
                "mergedBy": {"__typename": "User", "login": "bernot-dev"},
                "author": {"__typename": "User", "login": "ext-contrib"},
                "authorAssociation": "CONTRIBUTOR",
                "reviews": {"nodes": []},
            },
            {
                "number": 202,
                "state": "OPEN",
                "updatedAt": "2026-10-12T00:00:00Z",
                "mergedAt": None,
                "mergedBy": None,
                "author": {"__typename": "User", "login": "org-member"},
                "authorAssociation": "MEMBER",
                "reviews": {"nodes": []},
            },
        ]
        profiles = sc.aggregate_contributor_profiles(
            pr_nodes, maintainer_handles_lower=set(), cutoff_iso=cutoff, window_days=90
        )
        self.assertEqual(profiles["bernot-dev"]["role"], sc.ROLE_MAINTAINER)
        self.assertEqual(profiles["org-member"]["role"], sc.ROLE_MEMBER)
        self.assertEqual(profiles["ext-contrib"]["role"], sc.ROLE_CONTRIBUTOR)

    def test_priority_aging_prevents_starvation(self):
        new_profile = {"role": sc.ROLE_NEW_CONTRIBUTOR, "trust_score": 0.0}
        inactive_contrib = {"role": sc.ROLE_CONTRIBUTOR, "trust_score": 15.0}
        self.assertEqual(sc.effective_queue_trust_score(new_profile), 30.0)
        self.assertEqual(sc.effective_queue_trust_score(inactive_contrib), 15.0)

        # A New Contributor (30.0 welcome baseline) waiting 4 days (36.0) bubbles past a fresh Member PR (35.0).
        new_waiting_4d = sc.compute_priority_score(
            sc.effective_queue_trust_score(new_profile), waiting_days=4.0
        )
        fresh_member = sc.compute_priority_score(35.0, waiting_days=0.0)
        self.assertGreater(new_waiting_4d, fresh_member)


if __name__ == "__main__":
    unittest.main()
