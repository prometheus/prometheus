#!/usr/bin/env bash
#
# Build React web UI.
# Run from repository root.
set -e
set -u

if ! [[ "$0" =~ "scripts/build_react_app.sh" ]]; then
	echo "must be run from repository root"
	exit 255
fi

cd web/ui/react-app

echo "installing npm dependencies"
yarn --frozen-lockfile

echo "building React app"
PUBLIC_URL=. yarn build
rm -rf ../static/graph-new
mv build ../static/graph-new
# Prevent bad redirect due to Go HTTP router treating index.html specially.
mv ../static/graph-new/index.html ../static/graph-new/app.html

echo "bundling npm licenses"
LICENSES_TARBALL="npm_licenses.tar.bz2"
rm -f "$LICENSES_TARBALL"
find node_modules -iname "license*" | tar cvfj "$LICENSES_TARBALL" --transform 's/^/npm_licenses\//' --files-from=-
