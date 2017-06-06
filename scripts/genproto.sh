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

if ! [[ $(protoc --version) =~ "3.2.0" ]]; then
	echo "could not find protoc 3.2.0, is it installed + in PATH?"
	exit 255
fi

GOGOPROTO_ROOT="${GOPATH}/src/github.com/gogo/protobuf"
GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"
GRPC_GATEWAY_ROOT="${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway"

DIRS="web/api/v2/v2pb"

for dir in ${DIRS}; do
	pushd ${dir}
		protoc --gogofast_out=plugins=grpc:. -I=. \
            -I="${GOGOPROTO_PATH}" \
            -I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
            *.proto

		sed -i.bak -E 's/import _ \"gogoproto\"//g' *.pb.go
		sed -i.bak -E 's/import _ \"google\/protobuf\"//g' *.pb.go
		rm -f *.bak
		goimports -w *.pb.go
	popd
done

protoc -I. \
    -I="${GOGOPROTO_PATH}" \
    -I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
    --grpc-gateway_out=logtostderr=true:. \
    --swagger_out=logtostderr=true:./documentation/dev/apiv2/swagger/. \
    web/api/v2/v2pb/api.proto

mv documentation/dev/apiv2/swagger/web/api/v2/v2pb/api.swagger.json documentation/dev/apiv2/swagger
rm -rf documentation/dev/apiv2/swagger/web