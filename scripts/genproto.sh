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

if ! [[ $(buf --version) =~ 1.28.1 ]]; then
	echo "could not find buf 1.28.1, is it installed + in PATH?"
	exit 255
fi

# Since we run go install, go mod download, the go.sum will change.
# Make a backup.
cp go.sum go.sum.bak

INSTALL_PKGS=(
  "golang.org/x/tools/cmd/goimports"
  "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway"
  "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger"
)
for pkg in "${INSTALL_PKGS[@]}"; do
    echo "installing $pkg"
done

GET_PKGS=(
  "google.golang.org/protobuf/cmd/protoc-gen-go@v1.31.0"
  "google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0"
)
for pkg in "${GET_PKGS[@]}"; do
    echo "getting $pkg"
    GO111MODULE=on go get "$pkg"
done


buf generate --verbose

mv go.sum.bak go.sum
