# How to Create a Release of OpenCensus Proto (for Maintainers Only)

## Build Environments

We re-generate gen-go files and deploy jars to Maven Central under the following systems:

-   Ubuntu 14.04

Other systems may also work, but we haven't verified them.

## Release Go files

To generate the Go files from protos, you'll need to install protoc, protoc-gen-go and grpc-gateway plugins first.
Follow the instructions [here](http://google.github.io/proto-lens/installing-protoc.html),
[here](https://github.com/golang/protobuf#installation) and [here](https://github.com/grpc-ecosystem/grpc-gateway#installation).

Then run the following commands to re-generate the gen-go files:

```bash
$ cd $(go env GOPATH)/src/github.com/census-instrumentation/opencensus-proto
$ git checkout -b update-gen-go
$ rm -rf gen-go
$ cd src
$ ./mkgogen.sh
$ git add -A
$ git commit -m "Update gen-go files."
```

Go through PR review and merge the changes to GitHub.

## Release Ruby files

To generate the Ruby files from protos, you'll need to install grpc-tools gem.

```bash
gem install grpc-tools
```

Then run the following commands to re-generate the gen-ruby files:

```bash
$ git@github.com:census-instrumentation/opencensus-proto.git
$ cd opencensus-proto
$ git checkout -b update-gen-ruby
$ rm -rf gen-ruby
$ cd src
$ ./mkrubygen.sh
$ git add -A
$ git commit -m "Update gen-ruby files."
```

## Release Python files

To generate the Python files from protos, you'll need to install grpc-tools from PIP.

```bash
python -m pip install grpcio-tools
```

Then run the following commands to re-generate the gen-python files:

```bash
$ git checkout -b update-gen-python # Assume you're under opencensus-proto/
$ cd src
$ ./mkpygen.sh
$ git add -A
$ git commit -m "Update gen-python files."
```

## Tagging the Release

Our release branches follow the naming convention of `v<major>.<minor>.x`, while the tags include the
patch version `v<major>.<minor>.<patch>`. For example, the same branch `v0.4.x` would be used to create
all `v0.4` tags (e.g. `v0.4.0`, `v0.4.1`).

In this section upstream repository refers to the main opencensus-proto github
repository.

Before any push to the upstream repository you need to create a [personal access
token](https://help.github.com/articles/creating-a-personal-access-token-for-the-command-line/).

1.  Create the release branch and push it to GitHub:

    ```bash
    $ MAJOR=0 MINOR=4 PATCH=0 # Set appropriately for new release
    $ JAVA_VERSION_FILES=(build.gradle)
    $ PYTHON_VERSION_FILES=(gen-python/version.py)
    $ git checkout -b v$MAJOR.$MINOR.x master
    $ git push upstream v$MAJOR.$MINOR.x
    ```

2. Enable branch protection for the new branch, if you have admin access.
   Otherwise, let someone with admin access know that there is a new release
   branch.

    - Open the branch protection settings for the new branch, by following
      [Github's instructions](https://help.github.com/articles/configuring-protected-branches/).
    - Copy the settings from a previous branch, i.e., check
      - `Protect this branch`
      - `Require pull request reviews before merging`
      - `Require status checks to pass before merging`
      - `Include administrators`

      Enable the following required status checks:
      - `cla/google`
      - `continuous-integration/travis-ci`
    - Uncheck everything else.
    - Click "Save changes".

3.  For `master` branch:

    -   Change root build files to the next minor snapshot (e.g.
        `0.5.0-SNAPSHOT`).

    ```bash
    $ git checkout -b bump-version master
    # Change version to next minor (and keep -SNAPSHOT and .dev0)
    $ sed -i 's/[0-9]\+\.[0-9]\+\.[0-9]\+\(.*CURRENT_OPENCENSUS_PROTO_VERSION\)/'$MAJOR.$((MINOR+1)).0'\1/' \
      "${JAVA_VERSION_FILES[@]}"
    $ sed -i 's/[0-9]\+\.[0-9]\+\(.*CURRENT_OPENCENSUS_PROTO_VERSION\)/'$MAJOR.$((MINOR+1))'\1/' \
      "${PYTHON_VERSION_FILES[@]}"
    $ ./gradlew build
    $ git commit -a -m "Start $MAJOR.$((MINOR+1)).0 development cycle"
    ```

    -   Go through PR review and push the master branch to GitHub:

    ```bash
    $ git checkout master
    $ git merge --ff-only bump-version
    $ git push upstream master
    ```

4.  For `vMajor.Minor.x` branch:

    -   Change root build files to remove "-SNAPSHOT" for the next release
        version (e.g. `0.4.0`). Commit the result and make a tag:

    ```bash
    $ git checkout -b release v$MAJOR.$MINOR.x
    # Change version to remove -SNAPSHOT and .dev0
    $ sed -i 's/-SNAPSHOT\(.*CURRENT_OPENCENSUS_PROTO_VERSION\)/\1/' "${JAVA_VERSION_FILES[@]}"
    $ sed -i 's/dev0\(.*CURRENT_OPENCENSUS_PROTO_VERSION\)/'0'\1/' "${PYTHON_VERSION_FILES[@]}"
    $ ./gradlew build
    $ git commit -a -m "Bump version to $MAJOR.$MINOR.$PATCH"
    $ git tag -a v$MAJOR.$MINOR.$PATCH -m "Version $MAJOR.$MINOR.$PATCH"
    ```

    -   Change root build files to the next snapshot version (e.g.
        `0.4.1-SNAPSHOT`). Commit the result:

    ```bash
    # Change version to next patch and add -SNAPSHOT and .dev1
    $ sed -i 's/[0-9]\+\.[0-9]\+\.[0-9]\+\(.*CURRENT_OPENCENSUS_PROTO_VERSION\)/'$MAJOR.$MINOR.$((PATCH+1))-SNAPSHOT'\1/' \
     "${JAVA_VERSION_FILES[@]}"
     $ sed -i 's/[0-9]\+\.[0-9]\+\.[0-9]\+\(.*CURRENT_OPENCENSUS_PROTO_VERSION\)/'$MAJOR.$MINOR.dev$((PATCH+1))'\1/' \
     "${PYTHON_VERSION_FILES[@]}"
    $ ./gradlew build
    $ git commit -a -m "Bump version to $MAJOR.$MINOR.$((PATCH+1))-SNAPSHOT"
    ```

    -   Go through PR review and push the release tag and updated release branch
        to GitHub:

    ```bash
    $ git checkout v$MAJOR.$MINOR.x
    $ git merge --ff-only release
    $ git push upstream v$MAJOR.$MINOR.$PATCH
    $ git push upstream v$MAJOR.$MINOR.x
    ```

## Release Java Jar

Deployment to Maven Central (or the snapshot repo) is for all of the artifacts
from the project.

### Prerequisites

If you haven't done already, please follow the instructions
[here](https://github.com/census-instrumentation/opencensus-java/blob/master/RELEASING.md#prerequisites)
to set up the OSSRH (OSS Repository Hosting) account and signing keys. This is required for releasing
to Maven Central.

### Branch

Before building/deploying, be sure to switch to the appropriate tag. The tag
must reference a commit that has been pushed to the main repository, i.e., has
gone through code review. For the current release use:

```bash
$ git checkout -b v$MAJOR.$MINOR.$PATCH tags/v$MAJOR.$MINOR.$PATCH
```

### Initial Deployment

The following command will build the whole project and upload it to Maven
Central. Parallel building [is not safe during
uploadArchives](https://issues.gradle.org/browse/GRADLE-3420).

```bash
$ ./gradlew clean build && ./gradlew -Dorg.gradle.parallel=false uploadArchives
```

If the version has the `-SNAPSHOT` suffix, the artifacts will automatically go
to the snapshot repository. Otherwise it's a release deployment and the
artifacts will go to a staging repository.

When deploying a Release, the deployment will create [a new staging
repository](https://oss.sonatype.org/#stagingRepositories). You'll need to look
up the ID in the OSSRH UI (usually in the form of `opencensus-*`).

### Releasing on Maven Central

Once all of the artifacts have been pushed to the staging repository, the
repository must first be `closed`, which will trigger several sanity checks on
the repository. If this completes successfully, the repository can then be
`released`, which will begin the process of pushing the new artifacts to Maven
Central (the staging repository will be destroyed in the process). You can see
the complete process for releasing to Maven Central on the [OSSRH
site](http://central.sonatype.org/pages/releasing-the-deployment.html).

## Push Python package to PyPI

We follow the same package distribution process outlined at
[Python Packaging User Guide](https://packaging.python.org/tutorials/packaging-projects/).

### Prerequisites

If you haven't already, install the latest versions of setuptools, wheel and twine:
```bash
$ python3 -m pip install --user --upgrade setuptools wheel twine
```

### Branch

Before building/deploying, be sure to switch to the appropriate tag. The tag
must reference a commit that has been pushed to the main repository, i.e., has
gone through code review. For the current release use:

```bash
$ git checkout -b v$MAJOR.$MINOR.$PATCH tags/v$MAJOR.$MINOR.$PATCH
```

### Generate and upload the distribution archives

```bash
$ cd gen-python
$ python3 setup.py sdist bdist_wheel
$ python3 -m twine upload dist/*
```

## Announcement

Once deployment is done, go to Github [release
page](https://github.com/census-instrumentation/opencensus-proto/releases), press
`Draft a new release` to write release notes about the new release.

You can use `git log upstream/v$MAJOR.$((MINOR-1)).x..upstream/v$MAJOR.$MINOR.x --graph --first-parent`
or the Github [compare tool](https://github.com/census-instrumentation/opencensus-proto/compare/)
to view a summary of all commits since last release as a reference.

Please pick major or important user-visible changes only.
