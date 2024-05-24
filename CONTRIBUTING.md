# Contributing

Prometheus uses GitHub to manage reviews of pull requests.

* If you are a new contributor see: [Steps to Contribute](#steps-to-contribute)

* If you have a trivial fix or improvement, go ahead and create a pull request,
  addressing (with `@...`) a suitable maintainer of this repository (see
  [MAINTAINERS.md](MAINTAINERS.md)) in the description of the pull request.

* If you plan to do something more involved, first discuss your ideas
  on our [mailing list](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers).
  This will avoid unnecessary work and surely give you and us a good deal
  of inspiration. Also please see our [non-goals issue](https://github.com/prometheus/docs/issues/149) on areas that the Prometheus community doesn't plan to work on.

* Relevant coding style guidelines are the [Go Code Review
  Comments](https://code.google.com/p/go-wiki/wiki/CodeReviewComments)
  and the _Formatting and style_ section of Peter Bourgon's [Go: Best
  Practices for Production
  Environments](https://peter.bourgon.org/go-in-production/#formatting-and-style).

* Be sure to sign off on the [DCO](https://github.com/probot/dco#how-it-works).

## Steps to Contribute

Should you wish to work on an issue, please claim it first by commenting on the GitHub issue that you want to work on it. This is to prevent duplicated efforts from contributors on the same issue.

Please check the [`low-hanging-fruit`](https://github.com/prometheus/prometheus/issues?q=is%3Aissue+is%3Aopen+label%3A%22low+hanging+fruit%22) label to find issues that are good for getting started. If you have questions about one of the issues, with or without the tag, please comment on them and one of the maintainers will clarify it. For a quicker response, contact us over [IRC](https://prometheus.io/community).

You can [spin up a prebuilt dev environment](https://gitpod.io/#https://github.com/prometheus/prometheus) using Gitpod.io.

For complete instructions on how to compile see: [Building From Source](https://github.com/prometheus/prometheus#building-from-source)

For quickly compiling and testing your changes do:

```bash
# For building.
go build ./cmd/prometheus/
./prometheus

# For testing.
make test         # Make sure all the tests pass before you commit and push :)
```

To run a collection of Go linters through [`golangci-lint`](https://github.com/golangci/golangci-lint), do:
```bash
make lint
```

If it reports an issue and you think that the warning needs to be disregarded or is a false-positive, you can add a special comment `//nolint:linter1[,linter2,...]` before the offending line. Use this sparingly though, fixing the code to comply with the linter's recommendation is in general the preferred course of action. See [this section of the golangci-lint documentation](https://golangci-lint.run/usage/false-positives/#nolint-directive) for more information.

All our issues are regularly tagged so that you can also filter down the issues involving the components you want to work on. For our labeling policy refer [the wiki page](https://github.com/prometheus/prometheus/wiki/Label-Names-and-Descriptions).

## Pull Request Checklist

* Branch from the main branch and, if needed, rebase to the current main branch before submitting your pull request. If it doesn't merge cleanly with main you may be asked to rebase your changes.

* Commits should be as small as possible, while ensuring that each commit is correct independently (i.e., each commit should compile and pass tests).

* If your patch is not getting reviewed or you need a specific person to review it, you can @-reply a reviewer asking for a review in the pull request or a comment, or you can ask for a review on the IRC channel [#prometheus-dev](https://web.libera.chat/?channels=#prometheus-dev) on irc.libera.chat (for the easiest start, [join via Element](https://app.element.io/#/room/#prometheus-dev:matrix.org)).

* Add tests relevant to the fixed bug or new feature.

## Dependency management

The Prometheus project uses [Go modules](https://golang.org/cmd/go/#hdr-Modules__module_versions__and_more) to manage dependencies on external packages.

To add or update a new dependency, use the `go get` command:

```bash
# Pick the latest tagged release.
go get example.com/some/module/pkg@latest

# Pick a specific version.
go get example.com/some/module/pkg@vX.Y.Z
```

Tidy up the `go.mod` and `go.sum` files:

```bash
# The GO111MODULE variable can be omitted when the code isn't located in GOPATH.
GO111MODULE=on go mod tidy
```

You have to commit the changes to `go.mod` and `go.sum` before submitting the pull request.

## Working with the PromQL parser

The PromQL parser grammar is located in `promql/parser/generated_parser.y` and it can be built using `make parser`.
The parser is built using [goyacc](https://pkg.go.dev/golang.org/x/tools/cmd/goyacc)

If doing some sort of debugging, then it is possible to add some verbose output. After generating the parser, then you
can modify the `./promql/parser/generated_parser.y.go` manually.

```golang
// As of writing this was somewhere around line 600.
var (
	yyDebug        = 0 // This can be a number 0 -> 5.
	yyErrorVerbose = false  // This can be set to true.
)

```
