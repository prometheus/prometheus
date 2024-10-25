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

# Since we run go install, go mod download, the go.sum will change.
# Make a backup.
cp go.sum go.sum.bak

INSTALL_PKGS="golang.org/x/tools/cmd/goimports github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger"
for pkg in ${INSTALL_PKGS}; do
    GO111MODULE=on go install "$pkg"
done

DIRS="prompb"

echo "generating code"
for dir in ${DIRS}; do
	pushd ${dir}
		buf generate
	popd
done

mv go.sum.bak go.sum
