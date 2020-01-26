#!/bin/bash

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# cd to project dir on Kokoro instance
cd github/go-genproto

go version

# Set $GOPATH
export GOPATH="$HOME/go"
export GENPROTO_HOME=$GOPATH/src/google.golang.org/genproto
export PATH="$GOPATH/bin:$PATH"
mkdir -p $GENPROTO_HOME

# Move code into $GOPATH and get dependencies
git clone . $GENPROTO_HOME
cd $GENPROTO_HOME

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
go test -race -v ./... 2>&1 | tee $KOKORO_ARTIFACTS_DIR/$KOKORO_GERRIT_CHANGE_NUMBER.txt
