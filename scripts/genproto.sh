#!/usr/bin/env bash
#
# Generate all protobuf bindings.
# Run from repository root.
set -e
set -u

if ! [[ "$0" =~ "scripts/genproto.sh" ]]; then
	echo "must be run from repository root"
	exit 255
fi

pushd "internal/tools"
INSTALL_PKGS="github.com/bufbuild/buf/cmd/buf github.com/gogo/protobuf/protoc-gen-gogofast github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2 golang.org/x/tools/cmd/goimports"
for pkg in ${INSTALL_PKGS}; do
    GO111MODULE=on go install "$pkg"
done
popd

DIRS="prompb"

echo "generating code"
for dir in ${DIRS}; do
	pushd ${dir}
		buf generate
		sed -i.bak -E 's/import _ \"github.com\/gogo\/protobuf\/gogoproto\"//g' *.pb.go
		sed -i.bak -E 's/import _ \"google\/protobuf\"//g' *.pb.go
		sed -i.bak -E 's/\t_ \"google\/protobuf\"//g' *.pb.go
		sed -i.bak -E 's/golang\/protobuf\/descriptor/gogo\/protobuf\/protoc-gen-gogo\/descriptor/g' *.go
		sed -i.bak -E 's/golang\/protobuf/gogo\/protobuf/g' *.go
		rm -f -- *.bak
		goimports -w ./*.go ./io/prometheus/client/*.go
	popd
done
