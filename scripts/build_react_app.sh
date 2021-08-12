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

echo "building React app"
PUBLIC_URL=. npm run build
rm -rf ../static/react
mv build ../static/react
