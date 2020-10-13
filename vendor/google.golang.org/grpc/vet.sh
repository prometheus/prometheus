#!/bin/bash

set -ex  # Exit on error; debugging enabled.
set -o pipefail  # Fail a pipe if any sub-command fails.

# not makes sure the command passed to it does not exit with a return code of 0.
not() {
  # This is required instead of the earlier (! $COMMAND) because subshells and
  # pipefail don't work the same on Darwin as in Linux.
  ! "$@"
}

die() {
  echo "$@" >&2
  exit 1
}

fail_on_output() {
  tee /dev/stderr | not read
}

# Check to make sure it's safe to modify the user's git repo.
git status --porcelain | fail_on_output

# Undo any edits made by this script.
cleanup() {
  git reset --hard HEAD
}
trap cleanup EXIT

PATH="${GOPATH}/bin:${GOROOT}/bin:${PATH}"

if [[ "$1" = "-install" ]]; then
  # Check for module support
  if go help mod >& /dev/null; then
    # Install the pinned versions as defined in module tools.
    pushd ./test/tools
    go install \
      golang.org/x/lint/golint \
      golang.org/x/tools/cmd/goimports \
      honnef.co/go/tools/cmd/staticcheck \
      github.com/client9/misspell/cmd/misspell
    popd
  else
    # Ye olde `go get` incantation.
    # Note: this gets the latest version of all tools (vs. the pinned versions
    # with Go modules).
    go get -u \
      golang.org/x/lint/golint \
      golang.org/x/tools/cmd/goimports \
      honnef.co/go/tools/cmd/staticcheck \
      github.com/client9/misspell/cmd/misspell
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
    elif not which protoc > /dev/null; then
      die "Please install protoc into your path"
    fi
  fi
  exit 0
elif [[ "$#" -ne 0 ]]; then
  die "Unknown argument(s): $*"
fi

# - Ensure all source files contain a copyright message.
not git grep -L "\(Copyright [0-9]\{4,\} gRPC authors\)\|DO NOT EDIT" -- '*.go'

# - Make sure all tests in grpc and grpc/test use leakcheck via Teardown.
not grep 'func Test[^(]' *_test.go
not grep 'func Test[^(]' test/*.go

# - Do not import x/net/context.
not git grep -l 'x/net/context' -- "*.go"

# - Do not import math/rand for real library code.  Use internal/grpcrand for
#   thread safety.
git grep -l '"math/rand"' -- "*.go" 2>&1 | not grep -v '^examples\|^stress\|grpcrand\|^benchmark\|wrr_test'

# - Do not call grpclog directly. Use grpclog.Component instead.
git grep -l 'grpclog.I\|grpclog.W\|grpclog.E\|grpclog.F\|grpclog.V' -- "*.go" | not grep -v '^grpclog/component.go\|^internal/grpctest/tlogger_test.go'

# - Ensure all ptypes proto packages are renamed when importing.
not git grep "\(import \|^\s*\)\"github.com/golang/protobuf/ptypes/" -- "*.go"

# - Ensure all xds proto imports are renamed to *pb or *grpc.
git grep '"github.com/envoyproxy/go-control-plane/envoy' -- '*.go' ':(exclude)*.pb.go' | not grep -v 'pb "\|grpc "'

# - gofmt, goimports, golint (with exceptions for generated code), go vet.
gofmt -s -d -l . 2>&1 | fail_on_output
goimports -l . 2>&1 | not grep -vE "\.pb\.go"
golint ./... 2>&1 | not grep -vE "\.pb\.go:"
go vet -all ./...

misspell -error .

# - Check that generated proto files are up to date.
if [[ -z "${VET_SKIP_PROTO}" ]]; then
  PATH="/home/travis/bin:${PATH}" make proto && \
    git status --porcelain 2>&1 | fail_on_output || \
    (git status; git --no-pager diff; exit 1)
fi

# - Check that our modules are tidy.
if go help mod >& /dev/null; then
  find . -name 'go.mod' | xargs -IXXX bash -c 'cd $(dirname XXX); go mod tidy'
  git status --porcelain 2>&1 | fail_on_output || \
    (git status; git --no-pager diff; exit 1)
fi

# - Collection of static analysis checks
#
# TODO(dfawley): don't use deprecated functions in examples or first-party
# plugins.
SC_OUT="$(mktemp)"
staticcheck -go 1.9 -checks 'inherit,-ST1015' ./... > "${SC_OUT}" || true
# Error if anything other than deprecation warnings are printed.
not grep -v "is deprecated:.*SA1019" "${SC_OUT}"
# Only ignore the following deprecated types/fields/functions.
not grep -Fv '.CredsBundle
.HeaderMap
.Metadata is deprecated: use Attributes
.NewAddress
.NewServiceConfig
.Type is deprecated: use Attributes
balancer.ErrTransientFailure
balancer.Picker
grpc.CallCustomCodec
grpc.Code
grpc.Compressor
grpc.CustomCodec
grpc.Decompressor
grpc.MaxMsgSize
grpc.MethodConfig
grpc.NewGZIPCompressor
grpc.NewGZIPDecompressor
grpc.RPCCompressor
grpc.RPCDecompressor
grpc.ServiceConfig
grpc.WithBalancerName
grpc.WithCompressor
grpc.WithDecompressor
grpc.WithDialer
grpc.WithMaxMsgSize
grpc.WithServiceConfig
grpc.WithTimeout
http.CloseNotifier
info.SecurityVersion
resolver.Backend
resolver.GRPCLB' "${SC_OUT}"

# - special golint on package comments.
lint_package_comment_per_package() {
  # Number of files in this go package.
  fileCount=$(go list -f '{{len .GoFiles}}' $1)
  if [ ${fileCount} -eq 0 ]; then
    return 0
  fi
  # Number of package errors generated by golint.
  lintPackageCommentErrorsCount=$(golint --min_confidence 0 $1 | grep -c "should have a package comment")
  # golint complains about every file that's missing the package comment. If the
  # number of files for this package is greater than the number of errors, there's
  # at least one file with package comment, good. Otherwise, fail.
  if [ ${fileCount} -le ${lintPackageCommentErrorsCount} ]; then
    echo "Package $1 (with ${fileCount} files) is missing package comment"
    return 1
  fi
}
lint_package_comment() {
  set +ex

  count=0
  for i in $(go list ./...); do
    lint_package_comment_per_package "$i"
    ((count += $?))
  done

  set -ex
  return $count
}
lint_package_comment

echo SUCCESS
