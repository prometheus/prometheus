#!/bin/bash

# TODO(deklerk) Add integration tests when it's secure to do so. b/64723143

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# cd to project dir on Kokoro instance
cd git/google-api-go-client

go version

# Set $GOPATH
export GOPATH="$HOME/go"
export GOCLOUD_HOME=$GOPATH/src/google.golang.org/api/
export PATH="$GOPATH/bin:$PATH"
mkdir -p $GOCLOUD_HOME

# Move code into $GOPATH and get dependencies
git clone . $GOCLOUD_HOME
cd $GOCLOUD_HOME

try3() { eval "$*" || eval "$*" || eval "$*"; }

download_deps() {
    if [[ `go version` == *"go1.11"* ]] || [[ `go version` == *"go1.12"* ]]; then
        export GO111MODULE=on
        # All packages, including +build tools, are fetched.
        try3 go mod download
    else
        # Because we don't provide -tags tools, the +build tools
        # dependencies aren't fetched.
        try3 go get -v -t ./...
    fi
}

download_deps
./internal/kokoro/vet.sh

# Run tests and tee output to log file, to be pushed to GCS as artifact.
go test -race -v -short ./... 2>&1 | tee $KOKORO_ARTIFACTS_DIR/$KOKORO_GERRIT_CHANGE_NUMBER.txt
