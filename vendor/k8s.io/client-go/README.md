# client-go

Go clients for talking to a [kubernetes](http://kubernetes.io/) cluster.

We currently recommend using the v7.0.0 tag. See [INSTALL.md](/INSTALL.md) for
detailed installation instructions. `go get k8s.io/client-go/...` works, but
will give you head and doesn't handle the dependencies well.

[![BuildStatus Widget]][BuildStatus Result]
[![GoReport Widget]][GoReport Status]
[![GoDocWidget]][GoDocReference]

[BuildStatus Result]: https://travis-ci.org/kubernetes/client-go
[BuildStatus Widget]: https://travis-ci.org/kubernetes/client-go.svg?branch=master

[GoReport Status]: https://goreportcard.com/report/github.com/kubernetes/client-go
[GoReport Widget]: https://goreportcard.com/badge/github.com/kubernetes/client-go

[GoDocWidget]: https://godoc.org/k8s.io/client-go?status.svg
[GoDocReference]:https://godoc.org/k8s.io/client-go 

## Table of Contents

- [What's included](#whats-included)
- [Versioning](#versioning)
  - [Compatibility: your code <-> client-go](#compatibility-your-code---client-go)
  - [Compatibility: client-go <-> Kubernetes clusters](#compatibility-client-go---kubernetes-clusters)
  - [Compatibility matrix](#compatibility-matrix)
  - [Why do the 1.4 and 1.5 branch contain top-level folder named after the version?](#why-do-the-14-and-15-branch-contain-top-level-folder-named-after-the-version)
- [Kubernetes tags](#kubernetes-tags)
- [How to get it](#how-to-get-it)
- [How to use it](#how-to-use-it)
- [Dependency management](#dependency-management)
- [Contributing code](#contributing-code)

### What's included

* The `kubernetes` package contains the clientset to access Kubernetes API.
* The `discovery` package is used to discover APIs supported by a Kubernetes API server.
* The `dynamic` package contains a dynamic client that can perform generic operations on arbitrary Kubernetes API objects.
* The `transport` package is used to set up auth and start a connection.
* The `tools/cache` package is useful for writing controllers.

### Versioning

`client-go` follows [semver](http://semver.org/). We will not make
backwards-incompatible changes without incrementing the major version number. A
change is backwards-incompatible either if it *i)* changes the public interfaces
of `client-go`, or *ii)* makes `client-go` incompatible with otherwise supported
versions of Kubernetes clusters.

Changes that add features in a backwards-compatible way will result in bumping
the minor version (second digit) number.

Bugfixes will result in the patch version (third digit) changing. PRs that are
cherry-picked into an older Kubernetes release branch will result in an update
to the corresponding branch in `client-go`, with a corresponding new tag
changing the patch version.

A consequence of this is that `client-go` version numbers will be unrelated to
Kubernetes version numbers.

#### Branches and tags.

We will create a new branch and tag for each increment in the major version number or
minor version number. We will create only a new tag for each increment in the patch
version number. See [semver](http://semver.org/) for definitions of major,
minor, and patch.

The master branch will track HEAD in the main Kubernetes repo and
accumulate changes. Consider HEAD to have the version `x.(y+1).0-alpha` or
`(x+1).0.0-alpha` (depending on whether it has accumulated a breaking change or
not), where `x` and `y` are the current major and minor versions.

#### Compatibility: your code <-> client-go

`client-go` follows [semver](http://semver.org/), so until the major version of
client-go gets increased, your code will compile and will continue to work with
explicitly supported versions of Kubernetes clusters. You must use a dependency
management system and pin a specific major version of `client-go` to get this
benefit, as HEAD follows the upstream Kubernetes repo.

#### Compatibility: client-go <-> Kubernetes clusters

Since Kubernetes is backwards compatible with clients, older `client-go`
versions will work with many different Kubernetes cluster versions.

We will backport bugfixes--but not new features--into older versions of
`client-go`.


#### Compatibility matrix

|                     | Kubernetes 1.4 | Kubernetes 1.5 | Kubernetes 1.6 | Kubernetes 1.7 | Kubernetes 1.8 | Kubernetes 1.9 | Kubernetes 1.10 |
|---------------------|----------------|----------------|----------------|----------------|----------------|----------------|-----------------|
| client-go 1.4       | ✓              | -              | -              | -              | -              | -              | -               |
| client-go 1.5       | +              | -              | -              | -              | -              | -              | -               |
| client-go 2.0       | +-             | ✓              | +-             | +-             | +-             | +-             | +-              |
| client-go 3.0       | +-             | +-             | ✓              | -              | +-             | +-             | +-              |
| client-go 4.0       | +-             | +-             | +-             | ✓              | +-             | +-             | +-              |
| client-go 5.0       | +-             | +-             | +-             | +-             | ✓              | +-             | +-              |
| client-go 6.0       | +-             | +-             | +-             | +-             | +-             | ✓              | +-              |
| client-go 7.0       | +-             | +-             | +-             | +-             | +-             | +-             | ✓               |
| client-go HEAD      | +-             | +-             | +-             | +-             | +-             | +              | +               |

Key:

* `✓` Exactly the same features / API objects in both client-go and the Kubernetes
  version.
* `+` client-go has features or API objects that may not be present in the
  Kubernetes cluster, either due to that client-go has additional new API, or
  that the server has removed old API. However, everything they have in
  common (i.e., most APIs) will work. Please note that alpha APIs may vanish or
  change significantly in a single release.
* `-` The Kubernetes cluster has features the client-go library can't use,
  either due to the server has additional new API, or that client-go has
  removed old API. However, everything they share in common (i.e., most APIs)
  will work.

See the [CHANGELOG](./CHANGELOG.md) for a detailed description of changes
between client-go versions.

| Branch         | Canonical source code location       | Maintenance status            |
|----------------|--------------------------------------|-------------------------------|
| client-go 1.4  | Kubernetes main repo, 1.4 branch     | = -                           |
| client-go 1.5  | Kubernetes main repo, 1.5 branch     | = -                           |
| client-go 2.0  | Kubernetes main repo, 1.5 branch     | = -                           |
| client-go 3.0  | Kubernetes main repo, 1.6 branch     | = -                           |
| client-go 4.0  | Kubernetes main repo, 1.7 branch     | = -                           |
| client-go 5.0  | Kubernetes main repo, 1.8 branch     | ✓                             |
| client-go 6.0  | Kubernetes main repo, 1.9 branch     | ✓                             |
| client-go 7.0  | Kubernetes main repo, 1.10 branch    | ✓                             |
| client-go HEAD | Kubernetes main repo, master branch  | ✓                             |

Key:

* `✓` Changes in main Kubernetes repo are actively published to client-go by a bot
* `=` Maintenance is manual, only severe security bugs will be patched.
* `-` Deprecated; please upgrade.

#### Deprecation policy

We will maintain branches for at least six months after their first stable tag
is cut. (E.g., the clock for the release-2.0 branch started ticking when we
tagged v2.0.0, not when we made the first alpha.) This policy applies to
every version greater than or equal to 2.0.

#### Why do the 1.4 and 1.5 branch contain top-level folder named after the version?

For the initial release of client-go, we thought it would be easiest to keep
separate directories for each minor version. That soon proved to be a mistake.
We are keeping the top-level folders in the 1.4 and 1.5 branches so that
existing users won't be broken.

### Kubernetes tags

As of April 2018, client-go is still a mirror of
[k8s.io/kubernetes/staging/src/client-go](https://github.com/kubernetes/kubernetes/tree/master/staging/src/k8s.io/client-go),
the code development is still done in the staging area. Since Kubernetes 1.8
release, when syncing the code from the staging area, we also sync the Kubernetes
version tags to client-go, prefixed with "kubernetes-". For example, if you check
out the `kubernetes-v1.8.0` tag in client-go, the code you get is exactly the
same as if you check out the `v1.8.0` tag in kubernetes, and change directory to
`staging/src/k8s.io/client-go`. The purpose is to let users quickly find matching
commits among published repos, like
[sample-apiserver](https://github.com/kubernetes/sample-apiserver),
[apiextension-apiserver](https://github.com/kubernetes/apiextensions-apiserver),
etc. The Kubernetes version tag does NOT claim any backwards compatibility
guarantees for client-go. Please check the [semantic versions](#versioning) if
you care about backwards compatibility.

### How to get it

You can use `go get k8s.io/client-go/...` to get client-go, but **you will get
the unstable master branch** and `client-go`'s vendored dependencies will not be
added to your `$GOPATH`. So we think most users will want to use a dependency
management system. See [INSTALL.md](/INSTALL.md) for detailed instructions.

### How to use it

If your application runs in a Pod in the cluster, please refer to the
in-cluster [example](examples/in-cluster-client-configuration), otherwise please
refer to the out-of-cluster [example](examples/out-of-cluster-client-configuration).

### Dependency management

If your application depends on a package that client-go depends on, and you let the Go compiler find the dependency in `GOPATH`, you will end up with duplicated dependencies: one copy from the `GOPATH`, and one from the vendor folder of client-go. This will cause unexpected runtime error like flag redefinition, since the go compiler ends up importing both packages separately, even if they are exactly the same thing. If this happens, you can either
* run `godep restore` ([godep](https://github.com/tools/godep)) in the client-go/ folder, then remove the vendor folder of client-go. Then the packages in your GOPATH will be the only copy
* or run `godep save` in your application folder to flatten all dependencies.

### Contributing code
Please send pull requests against the client packages in the Kubernetes main [repository](https://github.com/kubernetes/kubernetes). Changes in the staging area will be published to this repository every day.
