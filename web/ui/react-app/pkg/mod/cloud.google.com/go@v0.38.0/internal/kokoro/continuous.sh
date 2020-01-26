#!/bin/bash

export GOOGLE_APPLICATION_CREDENTIALS=$KOKORO_KEYSTORE_DIR/72523_go_integration_service_account
# Removing the GCLOUD_TESTS_GOLANG_PROJECT_ID setting may make some integration
# tests (like profiler's) silently skipped, so make sure you know what you are
# doing when changing / removing the next line.
export GCLOUD_TESTS_GOLANG_PROJECT_ID=dulcet-port-762
export GCLOUD_TESTS_GOLANG_KEY=$GOOGLE_APPLICATION_CREDENTIALS
export GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID=gcloud-golang-firestore-tests
export GCLOUD_TESTS_GOLANG_FIRESTORE_KEY=$KOKORO_KEYSTORE_DIR/72523_go_firestore_integration_service_account
export GCLOUD_TESTS_API_KEY=`cat $KOKORO_KEYSTORE_DIR/72523_go_gcloud_tests_api_key`
export GCLOUD_TESTS_GOLANG_KEYRING=projects/dulcet-port-762/locations/global/keyRings/go-integration-test
export GCLOUD_TESTS_GOLANG_PROFILER_ZONE="us-west1-b"

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
go test -race -v -timeout 45m ./... 2>&1 \
  | tee $KOKORO_ARTIFACTS_DIR/$KOKORO_GERRIT_CHANGE_NUMBER.txt

