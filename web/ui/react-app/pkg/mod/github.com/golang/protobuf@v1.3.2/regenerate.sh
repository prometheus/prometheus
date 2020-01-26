#!/bin/bash

set -e

# Install the working tree's protoc-gen-gen in a tempdir.
tmpdir=$(mktemp -d -t regen-wkt.XXXXXX)
trap 'rm -rf $tmpdir' EXIT
mkdir -p $tmpdir/bin
PATH=$tmpdir/bin:$PATH
GOBIN=$tmpdir/bin go install ./protoc-gen-go

# Public imports require at least Go 1.9.
supportTypeAliases=""
if go list -f '{{context.ReleaseTags}}' runtime | grep -q go1.9; then
  supportTypeAliases=1
fi

# Generate various test protos.
PROTO_DIRS=(
  jsonpb/jsonpb_test_proto
  proto
  protoc-gen-go/testdata
)
for dir in ${PROTO_DIRS[@]}; do
  for p in `find $dir -name "*.proto"`; do
    if [[ $p == */import_public/* && ! $supportTypeAliases ]]; then
      echo "# $p (skipped)"
      continue;
    fi
    echo "# $p"
    protoc -I$dir --go_out=plugins=grpc,paths=source_relative:$dir $p
  done
done

# Deriving the location of the source protos from the path to the
# protoc binary may be a bit odd, but this is what protoc itself does.
PROTO_INCLUDE=$(dirname $(dirname $(which protoc)))/include

# Well-known types.
WKT_PROTOS=(any duration empty struct timestamp wrappers)
for p in ${WKT_PROTOS[@]}; do
  echo "# google/protobuf/$p.proto"
  protoc --go_out=paths=source_relative:$tmpdir google/protobuf/$p.proto
  cp $tmpdir/google/protobuf/$p.pb.go ptypes/$p
  cp $PROTO_INCLUDE/google/protobuf/$p.proto ptypes/$p
done

# descriptor.proto.
echo "# google/protobuf/descriptor.proto"
protoc --go_out=paths=source_relative:$tmpdir google/protobuf/descriptor.proto
cp $tmpdir/google/protobuf/descriptor.pb.go protoc-gen-go/descriptor
cp $PROTO_INCLUDE/google/protobuf/descriptor.proto protoc-gen-go/descriptor
