#!/usr/bin/env bash

# Display commands being run
set -x

# Only run apidiff checks on go1.12 (we only need it once).
if [[ `go version` != *"go1.12"* ]]; then
    exit 0
fi

if git log -1 | grep BREAKING_CHANGE_ACCEPTABLE; then
  exit 0
fi

go install golang.org/x/exp/cmd/apidiff

# We compare against master@HEAD. This is unfortunate in some cases: if you're
# working on an out-of-date branch, and master gets some new feature (that has
# nothing to do with your work on your branch), you'll get an error message.
# Thankfully the fix is quite simple: rebase your branch.
git clone https://code.googlesource.com/gocloud /tmp/gocloud

MANUALS="bigquery bigtable datastore firestore pubsub spanner storage logging"
STABLE_GAPICS="container/apiv1 dataproc/apiv1 iam iam/admin/apiv1 iam/credentials/apiv1 kms/apiv1 language/apiv1 logging/apiv2 logging/logadmin pubsub/apiv1 spanner/apiv1 translate/apiv1 vision/apiv1"
for dir in $MANUALS $STABLE_GAPICS; do
  # turns things like ./foo/bar into foo/bar
  dir_without_junk=`echo $dir | sed -n "s#\(\.\/\)\(.*\)#\2#p"`
  pkg="cloud.google.com/go/$dir_without_junk"
  echo "Testing $pkg"

  cd /tmp/gocloud
  apidiff -w /tmp/pkg.master $pkg
  cd - > /dev/null

  # TODO(deklerk) there's probably a nicer way to do this that doesn't require
  # two invocations
  if ! apidiff -incompatible /tmp/pkg.master $pkg | (! read); then
    apidiff -incompatible /tmp/pkg.master $pkg
    exit 1
  fi
done