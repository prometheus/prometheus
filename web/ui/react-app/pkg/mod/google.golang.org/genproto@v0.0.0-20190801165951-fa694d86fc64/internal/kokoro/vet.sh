#!/bin/bash

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# Only run the linter on go1.12, since it needs type aliases (and we only care
# about its output once).
if [[ `go version` != *"go1.12"* ]]; then
    exit 0
fi

go install \
  github.com/golang/protobuf/protoc-gen-go \
  golang.org/x/tools/cmd/goimports

# Fail if a dependency was added without the necessary go.mod/go.sum change
# being part of the commit.
go mod tidy
git diff go.mod | tee /dev/stderr | (! read)
git diff go.sum | tee /dev/stderr | (! read)

# Easier to debug CI.
pwd

# Look at all .go files (ignoring .pb.go files) and make sure they have a Copyright. Fail if any don't.
git ls-files "*[^.pb].go" | xargs grep -L "\(Copyright [0-9]\{4,\}\)" 2>&1 | tee /dev/stderr | (! read)
gofmt -s -d -l . 2>&1 | tee /dev/stderr | (! read)
goimports -l . 2>&1 | tee /dev/stderr | (! read)

# No need to golint / staticcheck when it's just proto-generated files
