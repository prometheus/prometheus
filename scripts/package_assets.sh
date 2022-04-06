#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

cd web/ui
find static -type f -name '*.gz' -print0 | xargs -0 tar cf static.tar.gz
