#!/usr/bin/env bash
#
# Generate all protobuf bindings.
set -e
set -u

if ! [[ $(protoc --version) =~ "3.5.1" ]]; then
	echo "could not find protoc 3.5.1, is it installed + in PATH?"
	exit 255
fi

# Exact version of plugins to build.
PROTOC_GEN_GOGOFAST_SHA="971cbfd2e72b513a28c74af7462aee0800248d69"

echo "installing plugins"
go install "golang.org/x/tools/cmd/goimports"
go get -d -u "github.com/gogo/protobuf/protoc-gen-gogo"
pushd ${GOPATH}/src/github.com/gogo/protobuf
    git reset --hard "${PROTOC_GEN_GOGOFAST_SHA}"
    go install "github.com/gogo/protobuf/protoc-gen-gogofast"
popd

GOGOPROTO_ROOT="${GOPATH}/src/github.com/gogo/protobuf"
GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"

DIRS="model"

for dir in ${DIRS}; do
	pushd ${dir}
		protoc --gogofast_out=. -I=. \
            -I="${GOGOPROTO_PATH}" \
            *.proto
		
		sed -i.bak -E 's/import _ \"github.com\/gogo\/protobuf\/gogoproto\"//g' *.pb.go
		sed -i.bak -E 's/import _ \"google\/protobuf\"//g' *.pb.go
		sed -i.bak -E 's/golang\/protobuf/gogo\/protobuf/g' *.go
		rm -f *.bak
		goimports -w *.pb.go
	popd
done
