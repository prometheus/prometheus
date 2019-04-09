# Releases

This page describes the release process and the currently planned schedule for upcoming releases as well as the respective release shepherd. Release shepherd are chosen on a voluntary basis.

## Release schedule

Release cadence of first pre-releases being cut is 6 weeks.

| release series | date of first pre-release (year-month-day) | release shepherd                             |
|----------------|--------------------------------------------|---------------------------------------------|
| v2.4           | 2018-09-06                                 | Goutham Veeramachaneni (GitHub: @gouthamve) |
| v2.5           | 2018-10-24                                 | Frederic Branczyk (GitHub: @brancz)         |
| v2.6           | 2018-12-05                                 | Simon Pasquier (GitHub: @simonpasquier)     |
| v2.7           | 2019-01-16                                 | Goutham Veeramachaneni (GitHub: @gouthamve) |
| v2.8           | 2019-02-27                                 | Ganesh Vernekar (GitHub: @codesome)         |
| v2.9           | 2019-04-10                                 | Brian Brazil (GitHub: @brian-brazil)        |
| v2.10          | 2019-05-22                                 | **searching for volunteer**                 |

If you are interested in volunteering please create a pull request against the [prometheus/prometheus](https://github.com/prometheus/prometheus) repository and propose yourself for the release series of your choice.

## Release shepherd responsibilities

The release shepherd is responsible for the entire release series of a minor release, meaning all pre- and patch releases of a minor release. The process starts with the initial pre-release.

* The first pre-release is scheduled according to the above schedule.
* With the pre-release the release shepherd is responsible for running and monitoring a benchmark run of the pre-release for 3 days, after which, if successful, the pre-release is promoted to a stable release.
* Once a pre-release has been released, the `master` branch of the repository is frozen for any feature work, only critical bug fix work concerning the minor release is merged.
* Pre-releases are done from `master`, after pre-releases are promoted to the stable release a `release-major.minor` branch is created.

See the next section for details on cutting an individual release.

## How to cut an individual release

These instructions are currently valid for the Prometheus server, i.e. the [prometheus/prometheus repository](https://github.com/prometheus/prometheus). Applicability to other Prometheus repositories depends on the current state of each repository. We aspire to unify the release procedures as much as possible.

### Branch management and versioning strategy

We use [Semantic Versioning](https://semver.org/).

We maintain a separate branch for each minor release, named `release-<major>.<minor>`, e.g. `release-1.1`, `release-2.0`.

The usual flow is to merge new features and changes into the master branch and to merge bug fixes into the latest release branch. Bug fixes are then merged into master from the latest release branch. The master branch should always contain all commits from the latest release branch. Whether merging master back into a release branch makes more sense is left up to the shepherd's judgement.

If a bug fix got accidentally merged into master, cherry-pick commits have to be created in the latest release branch, which then have to be merged back into master. Try to avoid that situation.

Maintaining the release branches for older minor releases happens on a best effort basis.

### Prepare your release

For a patch release, work in the branch of the minor release you want to patch.

For a new major or minor release, create the corresponding release branch based on the master branch.

Bump the version in the `VERSION` file and update `CHANGELOG.md`. Do this in a proper PR as this gives others the opportunity to chime in on the release in general and on the addition to the changelog in particular.

Note that `CHANGELOG.md` should only document changes relevant to users of Prometheus, including external API changes, performance improvements, and new features. Do not document changes of internal interfaces, code refactorings and clean-ups, changes to the build process, etc. People interested in these are asked to refer to the git history.

Entries in the `CHANGELOG.md` are meant to be in this order:

* `[CHANGE]`
* `[FEATURE]`
* `[ENHANCEMENT]`
* `[BUGFIX]`

### Draft the new release

Tag the new release with a tag named `v<major>.<minor>.<patch>`, e.g. `v2.1.3`. Note the `v` prefix.

You can do the tagging on the commandline:

```bash
$ tag=$(< VERSION)
$ git tag -s "v${tag}" -m "v${tag}"
$ git push --tags
```

Signing a tag with a GPG key is appreciated, but in case you can't add a GPG key to your Github account using the following [procedure](https://help.github.com/articles/generating-a-gpg-key/), you can replace the `-s` flag by `-a` flag of the `git tag` command to only annotate the tag without signing.

Once a tag is created, the release process through CircleCI will be triggered for this tag.
You must create a Github Release using the UI for this tag, as otherwise CircleCI will not be able to upload tarballs for this tag. __Also, you must create the Github Release using a Github user that has granted access rights to CircleCI.__ If you did not or cannot grant those rights to your personal account, you can log in as `prombot` in an anonymous browser tab. (This will, however, prevent verified releases signed with your GPG key. For verified releases, the signing identity must be the same as the one creating the release.)

Go to the releases page of the project, click on the _Draft a new release_ button and select the tag you just pushed. The title of the release is formatted `x.y.z / YYYY-MM-DD`. Add the relevant part of `CHANGELOG.md` as description. Click _Save draft_ rather than _Publish release_ at this time. (This will prevent the release being visible before it has got the binaries attached to it.)

You can also create the tag and the Github release in one go through the Github UI by going to the releases page and then click on the `Draft a new release` button and enter your tag version.

Now all you can do is to wait for tarballs to be uploaded to the Github release and Docker images to be pushed to the Docker Hub and Quay.io. Once that has happened, click _Publish release_, which will make the release publicly visible and create a GitHub notification.

### Wrapping up

If the release has happened in the latest release branch, merge the changes into master.

To update the docs, a PR needs to be created to `prometheus/docs`. See [this PR](https://github.com/prometheus/docs/pull/952/files) for inspiration.

Once the binaries have been uploaded, announce the release on `prometheus-users@googlegroups.com` and `prometheus-announce@googlegroups.com`. Start the subject with `[ANN]`. Check out previous announcement mails for inspiration.

### Pre-releases

The following changes to the above procedures apply:

* In line with [Semantic Versioning](https://semver.org/), append something like `-rc.0` to the version (with the corresponding changes to the tag name, the release name etc.).
* Tick the _This is a pre-release_ box when drafting the release in the Github UI.
* Still update `CHANGELOG.md`, but when you cut the final release later, merge all the changes from the pre-releases into the one final update.

