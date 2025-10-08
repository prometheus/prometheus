#!/usr/bin/env bash
#
# Generate all protobuf bindings.
# Run from repository root.
set -e
set -u

if ! [[ "$0" =~ "scripts/gentextlex.sh" ]]; then
	echo "must be run from repository root"
	exit 255
fi

pushd "internal/tools"
INSTALL_PKGS="modernc.org/golex"
for pkg in ${INSTALL_PKGS}; do
    GO111MODULE=on go install "$pkg"
done
popd

echo "generating lex code"
pushd model/textparse
	  golex -o=promlex.l.go promlex.l
    golex -o=openmetricslex.l.go openmetricslex.l
    golex -o=openmetrics2lex.l.go openmetrics2lex.l
popd
