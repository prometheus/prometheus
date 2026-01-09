# Releases

This page describes the release process and the currently planned schedule for upcoming releases as well as the respective release shepherd. Release shepherds are chosen on a voluntary basis.

## Release schedule

Release cadence of first pre-releases being cut is 6 weeks.
Please see [the v2.55 RELEASE.md](https://github.com/prometheus/prometheus/blob/release-2.55/RELEASE.md) for the v2 release series schedule.

| release series | date of first pre-release (year-month-day) | release shepherd                   |
|----------------|--------------------------------------------|------------------------------------|
| v3.0           | 2024-11-14                                 | Jan Fajerski (GitHub: @jan--f)     |
| v3.1           | 2024-12-17                                 | Bryan Boreham (GitHub: @bboreham)  |
| v3.2           | 2025-01-28                                 | Jan Fajerski (GitHub: @jan--f)     |
| v3.3           | 2025-03-11                                 | Ayoub Mrini (Github: @machine424)  |
| v3.4           | 2025-04-29                                 | Jan-Otto Kröpke (Github: @jkroepke)|
| v3.5 LTS       | 2025-06-03                                 | Bryan Boreham (GitHub: @bboreham)  |
| v3.6           | 2025-08-01                                 | Ayoub Mrini (Github: @machine424)  |
| v3.7           | 2025-09-25                                 | Arthur Sens and George Krajcsovits (Github: @ArthurSens and @krajorama)|
| v3.8           | 2025-11-06                                 | Jan Fajerski (GitHub: @jan--f)     |
| v3.9           | 2025-12-18                                 | Bryan Boreham (GitHub: @bboreham)  |
| v3.10          | 2026-02-05                                 | **volunteer welcome**              |

If you are interested in volunteering please create a pull request against the [prometheus/prometheus](https://github.com/prometheus/prometheus) repository and propose yourself for the release series of your choice.

## Release shepherd responsibilities

The release shepherd is responsible for the entire release series of a minor release, meaning all pre- and patch releases of a minor release. The process formally starts with the initial pre-release, but some preparations should be done a few days in advance.

* We aim to keep the main branch in a working state at all times. In principle, it should be possible to cut a release from main at any time. In practice, things might not work out as nicely. A few days before the pre-release is scheduled, the shepherd should check the state of main. Following their best judgement, the shepherd should try to expedite bug fixes that are still in progress but should make it into the release. On the other hand, the shepherd may hold back merging last-minute invasive and risky changes that are better suited for the next minor release.
* On the date listed in the table above, the release shepherd cuts the first pre-release (using the suffix `-rc.0`) and creates a new branch called  `release-<major>.<minor>` starting at the commit tagged for the pre-release. In general, a pre-release is considered a release candidate (that's what `rc` stands for) and should therefore not contain any known bugs that are planned to be fixed in the final release.
* With the pre-release, the release shepherd is responsible for running and monitoring a benchmark run of the pre-release for 3 days, after which, if successful, the pre-release is promoted to a stable release.
* If regressions or critical bugs are detected, they need to get fixed before cutting a new pre-release (called `-rc.1`, `-rc.2`, etc.).

See the next section for details on cutting an individual release.

## How to cut an individual release

These instructions are currently valid for the Prometheus server, i.e. the [prometheus/prometheus repository](https://github.com/prometheus/prometheus). Applicability to other Prometheus repositories depends on the current state of each repository. We aspire to unify the release procedures as much as possible.

### Branch management and versioning strategy

We use [Semantic Versioning](https://semver.org/).

We maintain a separate branch for each minor release, named `release-<major>.<minor>`, e.g. `release-1.1`, `release-2.0`.

Note that branch protection kicks in automatically for any branches whose name starts with `release-`. Never use names starting with `release-` for branches that are not release branches.

The usual flow is to merge new features and changes into the main branch and to merge bug fixes into the latest release branch. Bug fixes are then merged into main from the latest release branch. The main branch should always contain all commits from the latest release branch. As long as main hasn't deviated from the release branch, new commits can also go to main, followed by merging main back into the release branch.

If a bug fix got accidentally merged into main after non-bug-fix changes in main, the bug-fix commits have to be cherry-picked into the release branch, which then have to be merged back into main. Try to avoid that situation.

Maintaining the release branches for older minor releases happens on a best effort basis.

### 0. Updating dependencies and promoting/demoting experimental features

A few days before a major or minor release, consider updating the dependencies.

Note that we use [Dependabot](.github/dependabot.yml) to continuously update most things automatically. Therefore, most dependencies should be up to date.
Check the [dependencies GitHub label](https://github.com/prometheus/prometheus/labels/dependencies) to see if there are any pending updates.

This bot currently does not manage `+incompatible` and `v0.0.0` in the version specifier for Go modules.

Note that after a dependency update, you should look out for any weirdness that
might have happened. Such weirdnesses include but are not limited to: flaky
tests, differences in resource usage, panic.

In case of doubt or issues that can't be solved in a reasonable amount of time,
you can skip the dependency update or only update select dependencies. In such a
case, you have to create an issue or pull request in the GitHub project for
later follow-up.

This is also a good time to consider any experimental features and feature
flags for promotion to stable or for deprecation or ultimately removal. Do any
of these in pull requests, one per feature.

> NOTE: As a validation step check if all security alerts are closed after this step: https://github.com/prometheus/prometheus/security/dependabot. Sometimes it's ok
> if not critical and e.g. fix is not released yet (or it does not relate to 
> upgrading) or when we are unaffected.
 
#### Manually updating Go dependencies

This is usually only needed for `+incompatible` and `v0.0.0` non-semver updates.

```bash
make update-all-go-deps
```

#### Manually updating React dependencies

The React application recently moved to a monorepo system with multiple internal npm packages. Dependency upgrades are
quite sensitive for the time being.

In case you want to update the UI dependencies, you can run the following command:

```bash
make update-npm-deps
```

Once this step completes, please verify that no additional `node_modules` directory was created in any of the module subdirectories
(which could indicate conflicting dependency versions across modules). Then run `make ui-build` to verify that the build is still working.

Note: Once in a while, the npm dependencies should also be updated to their latest release versions (major or minor) with `make upgrade-npm-deps`,
though this may be done at convenient times (e.g. by the UI maintainers) that are out-of-sync with Prometheus releases.

### 1. Prepare your release

At the start of a new major or minor release cycle create the corresponding release branch based on the main branch. For example if we're releasing `2.17.0` and the previous stable release is `2.16.0` we need to create a `release-2.17` branch. Note that all releases are handled in protected release branches, see the above `Branch management and versioning` section. Release candidates and patch releases for any given major or minor release happen in the same `release-<major>.<minor>` branch. Do not create `release-<version>` for patch or release candidate releases.

Changes for a patch release or release candidate should be merged into the previously mentioned release branch via pull request.

Bump the version in the `VERSION` file and update `CHANGELOG.md`. Do this in a proper PR pointing to the release branch as this gives others the opportunity to chime in on the release in general and on the addition to the changelog in particular. For a release candidate, append something like `-rc.0` to the version (with the corresponding changes to the tag name, the release name etc.).

When updating the `CHANGELOG.md` look at all PRs included in the release since the last release and verify if they need a changelog entry. Most PRs will have their changelog entries specified in the `release-notes` blocks within their PR descriptions.

Note that `CHANGELOG.md` should only document changes relevant to users of Prometheus, including external API changes, performance improvements, and new features. Do not document changes of internal interfaces, code refactorings and clean-ups, changes to the build process, etc. People interested in these are asked to refer to the git history.

For release candidates still update `CHANGELOG.md`, but when you cut the final release later, merge all the changes from the pre-releases into the one final update.

Entries in the `CHANGELOG.md` are meant to be in this order:

* `[SECURITY]` - A bugfix that specifically fixes a security issue.
* `[CHANGE]`
* `[FEATURE]`
* `[ENHANCEMENT]`
* `[PERF]`
* `[BUGFIX]`

Then bump the UI module version:

```bash
make ui-bump-version
```

All of the above goes to make a PR, which you should create targeting the `release-x.y` branch, then wait for CI to complete and get an approval.
Once the PR is ready, merge it into the release branch and pull it down to your work area. Then you are ready for the next step:

### 2. Draft the new release

Tag the new release via the following commands:

```bash
tag="v$(< VERSION)"
git tag -s "${tag}" -m "${tag}"
git push origin "${tag}"
```

Alternatively, you can use this handy `.gitconfig` alias.

```ini
[alias]
  tag-release = "!f() { tag=v${1:-$(cat VERSION)} ; git tag -s ${tag} -m ${tag} && git push origin ${tag}; }; f"
```

Then release with `git tag-release`.

Signing a tag with a GPG key is appreciated, but in case you can't add a GPG key to your Github account using the following [procedure](https://help.github.com/articles/generating-a-gpg-key/), you can replace the `-s` flag by `-a` flag of the `git tag` command to only annotate the tag without signing.

Once a tag is created, the release process through Github Actions will be triggered for this tag and Github Actions will draft the GitHub release using the `prombot` account.

Finally, wait for the build step for the tag to finish. The point here is to wait for tarballs to be uploaded to the Github release and the container images to be pushed to the Docker Hub and Quay.io. Once that has happened, click _Publish release_, which will make the release publicly visible and create a GitHub notification.
**Note:** for a release candidate version ensure the _This is a pre-release_ box is checked when drafting the release in the Github UI. The CI job should take care of this but it's a good idea to double check before clicking _Publish release_.`

### 3. Tag the library release

Go modules versioning requires strict use of semver. Because we do not commit to
avoid code-level breaking changes for the libraries between minor releases of
the Prometheus server, we use major version zero releases for the libraries.

Tagging the new library release works similar to the normal release tagging,
but without the subsequent build and publish steps. Use the following commands:

```bash
tag="v$(./scripts/get_module_version.sh)"
git tag -s "${tag}" -m "${tag}"
git push origin "${tag}"
```

### 4. Wrapping up

For release candidate versions (`v2.16.0-rc.0`), run the benchmark for 3 days using the `/prombench vX.Y.Z` command, `vX.Y.Z` being the latest stable patch release's tag of the previous minor release series, such as `v2.15.2`.

If the release has happened in the latest release branch, merge the changes into main.

Once the binaries have been uploaded, announce the release on `prometheus-announce@googlegroups.com`. (Please do not use `prometheus-users@googlegroups.com` for announcements anymore.) Check out previous announcement mails for inspiration.

Finally, in case there is no release shepherd listed for the next release yet, find a volunteer.
