#!/bin/bash

if [[ `uname -a` = *"Darwin"* ]]; then
  echo "It seems you are running on Mac. This script does not work on Mac. See https://github.com/grpc/grpc-go/issues/2047"
  exit 1
fi

set -ex  # Exit on error; debugging enabled.
set -o pipefail  # Fail a pipe if any sub-command fails.

die() {
  echo "$@" >&2
  exit 1
}

# Check to make sure it's safe to modify the user's git repo.
if git status --porcelain | read; then
  die "Uncommitted or untracked files found; commit changes first"
fi

if [[ -d "${GOPATH}/src" ]]; then
  die "\${GOPATH}/src (${GOPATH}/src) exists; this script will delete it."
fi

# Undo any edits made by this script.
cleanup() {
  rm -rf "${GOPATH}/src"
  git reset --hard HEAD
}
trap cleanup EXIT

fail_on_output() {
  tee /dev/stderr | (! read)
}

PATH="${GOPATH}/bin:${GOROOT}/bin:${PATH}"

if [[ "$1" = "-install" ]]; then
  # Check for module support
  if go help mod >& /dev/null; then
    go install \
      golang.org/x/lint/golint \
      golang.org/x/tools/cmd/goimports \
      honnef.co/go/tools/cmd/staticcheck \
      github.com/client9/misspell/cmd/misspell \
      github.com/golang/protobuf/protoc-gen-go
  else
    # Ye olde `go get` incantation.
    # Note: this gets the latest version of all tools (vs. the pinned versions
    # with Go modules).
    go get -u \
      golang.org/x/lint/golint \
      golang.org/x/tools/cmd/goimports \
      honnef.co/go/tools/cmd/staticcheck \
      github.com/client9/misspell/cmd/misspell \
      github.com/golang/protobuf/protoc-gen-go
  fi
  if [[ -z "${VET_SKIP_PROTO}" ]]; then
    if [[ "${TRAVIS}" = "true" ]]; then
      PROTOBUF_VERSION=3.3.0
      PROTOC_FILENAME=protoc-${PROTOBUF_VERSION}-linux-x86_64.zip
      pushd /home/travis
      wget https://github.com/google/protobuf/releases/download/v${PROTOBUF_VERSION}/${PROTOC_FILENAME}
      unzip ${PROTOC_FILENAME}
      bin/protoc --version
      popd
    elif ! which protoc > /dev/null; then
      die "Please install protoc into your path"
    fi
  fi
  exit 0
elif [[ "$#" -ne 0 ]]; then
  die "Unknown argument(s): $*"
fi

# - Ensure all source files contain a copyright message.
git ls-files "*.go" | xargs grep -L "\(Copyright [0-9]\{4,\} gRPC authors\)\|DO NOT EDIT" 2>&1 | fail_on_output

# - Make sure all tests in grpc and grpc/test use leakcheck via Teardown.
(! grep 'func Test[^(]' *_test.go)
(! grep 'func Test[^(]' test/*.go)

# - Do not import math/rand for real library code.  Use internal/grpcrand for
#   thread safety.
git ls-files "*.go" | xargs grep -l '"math/rand"' 2>&1 | (! grep -v '^examples\|^stress\|grpcrand')

# - Ensure all ptypes proto packages are renamed when importing.
git ls-files "*.go" | (! xargs grep "\(import \|^\s*\)\"github.com/golang/protobuf/ptypes/")

# - Check imports that are illegal in appengine (until Go 1.11).
# TODO: Remove when we drop Go 1.10 support
go list -f {{.Dir}} ./... | xargs go run test/go_vet/vet.go

# - gofmt, goimports, golint (with exceptions for generated code), go vet.
gofmt -s -d -l . 2>&1 | fail_on_output
goimports -l . 2>&1 | fail_on_output
golint ./... 2>&1 | (! grep -vE "(_mock|\.pb)\.go:")
go tool vet -all .

# - Check that generated proto files are up to date.
if [[ -z "${VET_SKIP_PROTO}" ]]; then
  PATH="/home/travis/bin:${PATH}" make proto && \
    git status --porcelain 2>&1 | fail_on_output || \
    (git status; git --no-pager diff; exit 1)
fi

# - Check that our module is tidy.
if go help mod >& /dev/null; then
  go mod tidy && \
    git status --porcelain 2>&1 | fail_on_output || \
    (git status; git --no-pager diff; exit 1)
fi

# - Collection of static analysis checks
### HACK HACK HACK: Remove once staticcheck works with modules.
# Make a symlink in ${GOPATH}/src to its ${GOPATH}/pkg/mod equivalent for every package we use.
for x in $(find "${GOPATH}/pkg/mod" -name '*@*' | grep -v \/mod\/cache\/); do
  pkg="$(echo ${x#"${GOPATH}/pkg/mod/"} | cut -f1 -d@)";
  # If multiple versions exist, just use the existing one.
  if [[ -L "${GOPATH}/src/${pkg}" ]]; then continue; fi
  mkdir -p "$(dirname "${GOPATH}/src/${pkg}")";
  ln -s $x "${GOPATH}/src/${pkg}";
done
### END HACK HACK HACK

# TODO(menghanl): fix errors in transport_test.
staticcheck -go 1.9 -ignore '
balancer.go:SA1019
balancer_test.go:SA1019
clientconn_test.go:SA1019
balancer/roundrobin/roundrobin_test.go:SA1019
benchmark/benchmain/main.go:SA1019
internal/transport/handler_server.go:SA1019
internal/transport/handler_server_test.go:SA1019
internal/transport/transport_test.go:SA2002
stats/stats_test.go:SA1019
test/channelz_test.go:SA1019
test/end2end_test.go:SA1019
test/healthcheck_test.go:SA1019
' ./...
misspell -error .
