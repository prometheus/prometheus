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

# if ! [[ $(protoc --version) =~ "3.15.8" ]]; then
# 	echo "could not find protoc 3.15.8, is it installed + in PATH?"
# 	exit 255
# fi

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


MAPPINGS=(
  "google/protobuf/descriptor.proto=github.com/golang/protobuf/protoc-gen-go/descriptor"
)
MAPPING_ARG=""
for mapping in "${MAPPINGS[@]}"
do
  MAPPING_ARG+="M$mapping"
done

PROM_ROOT="${PWD}"
PROM_PATH="${PROM_ROOT}/prompb"
# GOGOPROTO_ROOT="$(GO111MODULE=on go list -mod=readonly -f '{{ .Dir }}' -m github.com/gogo/protobuf)"
# GOGOPROTO_PATH="${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"
GRPC_GATEWAY_ROOT="$(GO111MODULE=on go list -mod=readonly -f '{{ .Dir }}' -m github.com/grpc-ecosystem/grpc-gateway)"

DIRS="prompb"

echo "generating code"
for dir in ${DIRS}; do
	pushd ${dir}
			protoc \
			--go_out="$MAPPING_ARG":. \
			--go_opt=paths=source_relative \
			--go-grpc_out="$MAPPING_ARG":. \
			--go-grpc_opt=paths=source_relative \
			-I=. \
            -I="${PROM_PATH}" \
            -I="${GRPC_GATEWAY_ROOT}/third_party/googleapis" \
            ./*.proto
		# protoc --gogofast_out=Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,paths=source_relative:. -I=. \
        #     -I="${GOGOPROTO_PATH}" \
        #     ./io/prometheus/client/*.proto
		# sed -i.bak -E 's/import _ \"github.com\/gogo\/protobuf\/gogoproto\"//g' -- *.pb.go
		# sed -i.bak -E 's/import _ \"google\/protobuf\"//g' -- *.pb.go
		# sed -i.bak -E 's/\t_ \"google\/protobuf\"//g' -- *.pb.go
		# sed -i.bak -E 's/golang\/protobuf\/descriptor/gogo\/protobuf\/protoc-gen-gogo\/descriptor/g' -- *.go
		# sed -i.bak -E 's/golang\/protobuf/gogo\/protobuf/g' -- *.go
		# rm -f -- *.bak
		rm -f ./*.bak
		goimports -w ./*.go ./io/prometheus/client/*.go
	popd
done

mv go.sum.bak go.sum
