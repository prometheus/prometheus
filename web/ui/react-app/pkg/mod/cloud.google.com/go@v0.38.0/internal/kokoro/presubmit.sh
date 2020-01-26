#!/bin/bash

# TODO(deklerk) Add integration tests when it's secure to do so. b/64723143

# Fail on any error
set -eo pipefail

# Display commands being run
set -x

# cd to project dir on Kokoro instance
cd git/gocloud

go version

# Set $GOPATH
export GOPATH="$HOME/go"
export GOCLOUD_HOME=$GOPATH/src/cloud.google.com/go/
export PATH="$GOPATH/bin:$PATH"

# Move code into $GOPATH and get dependencies
mkdir -p $GOCLOUD_HOME
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

        go get github.com/jstemmer/go-junit-report
    fi
}

download_deps
go install github.com/jstemmer/go-junit-report
./internal/kokoro/vet.sh
./internal/kokoro/check_incompat_changes.sh

mkdir $KOKORO_ARTIFACTS_DIR/tests

# Takes the kokoro output log (raw stdout) and creates a machine-parseable xml
# file (xUnit). Then it exits with whatever exit code the last command had.
create_junit_xml() {
  last_status_code=$?

  cat $KOKORO_ARTIFACTS_DIR/$KOKORO_GERRIT_CHANGE_NUMBER.txt \
    | go-junit-report > $KOKORO_ARTIFACTS_DIR/tests/sponge_log.xml

  exit $last_status_code
}

trap create_junit_xml EXIT ERR

# Run tests and tee output to log file, to be pushed to GCS as artifact.
go test -race -v -timeout 15m -short ./... 2>&1 \
  | tee $KOKORO_ARTIFACTS_DIR/$KOKORO_GERRIT_CHANGE_NUMBER.txt
