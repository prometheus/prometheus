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

if ! [[ $(protoc --version) =~ "3.4" ]]; then
	echo "could not find protoc 3.4.x, is it installed + in PATH?"
	exit 255
fi

PROM_ROOT="${GOPATH}/src/github.com/prometheus/prometheus"
PROM_PATH="${PROM_ROOT}/prompb"
GOGOPROTO_ROOT="${GOPATH}/src/github.com/gogo/protobuf"
GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"
GRPC_GATEWAY_ROOT="${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway"

DIRS="prompb"

for dir in ${DIRS}; do
	pushd ${dir}
		protoc --gogofast_out=plugins=grpc:. -I=. \
            -I="${GOGOPROTO_PATH}" \
            -I="${PROM_PATH}" \
            -I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
            *.proto

		protoc -I. \
			-I="${GOGOPROTO_PATH}" \
			-I="${PROM_PATH}" \
			-I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
			--grpc-gateway_out=logtostderr=true:. \
			--swagger_out=logtostderr=true:../documentation/dev/api/ \
			rpc.proto
		mv ../documentation/dev/api/rpc.swagger.json ../documentation/dev/api/swagger.json
		
		sed -i.bak -E 's/import _ \"gogoproto\"//g' *.pb.go
		sed -i.bak -E 's/import _ \"google\/protobuf\"//g' *.pb.go
		sed -i.bak -E 's/golang\/protobuf/gogo\/protobuf/g' *.go
		rm -f *.bak
		goimports -w *.pb.go
	popd
done
