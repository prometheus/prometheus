#!/usr/bin/env bash
#
# Generate all protobuf bindings.
# Run from repository root.
set -e
set -u

if ! [[ "$0" =~ "scripts/gengoyacc.sh" ]]; then
	echo "must be run from repository root"
	exit 255
fi

pushd "internal/tools"
INSTALL_PKGS="github.com/daixiang0/gci golang.org/x/tools/cmd/goyacc"
for pkg in ${INSTALL_PKGS}; do
    GO111MODULE=on go install "$pkg"
done
popd

goyacc -l -o promql/parser/generated_parser.y.go promql/parser/generated_parser.y
gci write -s standard -s default -s "prefix(github.com/prometheus/prometheus)" promql/parser/generated_parser.y.go
