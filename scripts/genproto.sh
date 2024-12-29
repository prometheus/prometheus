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

SED=$(which gsed 2>/dev/null || which sed)

# Since we run go install, go mod download, the go.sum will change.
# Make a backup.
cp go.sum go.sum.bak

INSTALL_PKGS="golang.org/x/tools/cmd/goimports github.com/gogo/protobuf/protoc-gen-gogofast github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger github.com/bufbuild/buf/cmd/buf@v1.48.0"
for pkg in ${INSTALL_PKGS}; do
    GO111MODULE=on go install "$pkg"
done

# Build client proto via buf.
pushd ./prompb
buf generate --path=./io/prometheus/client
# We have to hack global registry which blocks multiple generated types
# from the same proto file (the same path+file and full name).
${SED} -i.bak -E 's/protoimpl.DescBuilder\{/protoimpl.DescBuilder\{FileRegistry:nopFileRegistry{},/g' ./io/prometheus/client/*.pb.go
${SED} -i.bak -E 's/protoimpl.TypeBuilder\{/protoimpl.TypeBuilder\{TypeRegistry:nopTypeRegistry{},/g' ./io/prometheus/client/*.pb.go
goimports -w ./io/prometheus/client/*.go
rm -f ./io/prometheus/client/*.bak
popd

# TODO(bwplotka): Move write building to buf e.g. once moved out of gogo.
if ! [[ $(protoc --version) =~ "3.15.8" ]]; then
	echo "could not find protoc 3.15.8, is it installed + in PATH? Consider commenting out this check for local flow"
	exit 255
fi

PROM_ROOT="${PWD}"
PROM_PATH="${PROM_ROOT}/prompb"
GOGOPROTO_ROOT="$(GO111MODULE=on go list -mod=readonly -f '{{ .Dir }}' -m github.com/gogo/protobuf)"
GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"
GRPC_GATEWAY_ROOT="$(GO111MODULE=on go list -mod=readonly -f '{{ .Dir }}' -m github.com/grpc-ecosystem/grpc-gateway)"
DIRS="prompb"

echo "generating code"
for dir in ${DIRS}; do
	pushd ${dir}
		protoc --gogofast_out=plugins=grpc:. -I=. \
            -I="${GOGOPROTO_PATH}" \
            -I="${PROM_PATH}" \
            -I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
            ./*.proto
		protoc --gogofast_out=plugins=grpc:. -I=. \
            -I="${GOGOPROTO_PATH}" \
            ./io/prometheus/write/v2/*.proto
		${SED} -i.bak -E 's/import _ \"github.com\/gogo\/protobuf\/gogoproto\"//g' *.pb.go
		${SED} -i.bak -E 's/import _ \"google\/protobuf\"//g' *.pb.go
		${SED} -i.bak -E 's/\t_ \"google\/protobuf\"//g' *.pb.go
		${SED} -i.bak -E 's/golang\/protobuf\/descriptor/gogo\/protobuf\/protoc-gen-gogo\/descriptor/g' *.go
		${SED} -i.bak -E 's/golang\/protobuf/gogo\/protobuf/g' *.go
		rm -f -- *.bak
		goimports -w ./*.go ./io/prometheus/client/*.go
	popd
done

mv go.sum.bak go.sum
