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

if ! [[ $(protoc --version) =~ "3.11.4" ]]; then
	echo "could not find protoc 3.11.4, is it installed + in PATH?"
	exit 255
fi

echo "installing plugins"
GO111MODULE=on go mod download

INSTALL_PKGS="golang.org/x/tools/cmd/goimports github.com/gogo/protobuf/protoc-gen-gogofast github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger"
for pkg in ${INSTALL_PKGS}; do
    GO111MODULE=on go install -mod=vendor "$pkg"
done

PROM_ROOT="${PWD}"
PROM_PATH="${PROM_ROOT}/prompb"
GOGOPROTO_ROOT="$(GO111MODULE=on go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)"
GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"
GRPC_GATEWAY_ROOT="$(GO111MODULE=on go list -f '{{ .Dir }}' -m github.com/grpc-ecosystem/grpc-gateway)"

DIRS="prompb"

echo "generating code"
for dir in ${DIRS}; do
	pushd ${dir}
		protoc --gogofast_out=plugins=grpc:. -I=. \
            -I="${GOGOPROTO_PATH}" \
            -I="${PROM_PATH}" \
            -I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
            ./*.proto

		protoc -I. \
			-I="${GOGOPROTO_PATH}" \
			-I="${PROM_PATH}" \
			-I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
			--grpc-gateway_out=logtostderr=true:. \
			--swagger_out=logtostderr=true:../documentation/dev/api/ \
			rpc.proto
		mv ../documentation/dev/api/rpc.swagger.json ../documentation/dev/api/swagger.json

		sed -i.bak -E 's/import _ \"github.com\/gogo\/protobuf\/gogoproto\"//g' -- *.pb.go
		sed -i.bak -E 's/import _ \"google\/protobuf\"//g' -- *.pb.go
		sed -i.bak -E 's/\t_ \"google\/protobuf\"//g' -- *.pb.go
		sed -i.bak -E 's/golang\/protobuf\/descriptor/gogo\/protobuf\/protoc-gen-gogo\/descriptor/g' -- *.go
		sed -i.bak -E 's/golang\/protobuf/gogo\/protobuf/g' -- *.go
		rm -f -- *.bak
		goimports -w ./*.go
	popd
done
