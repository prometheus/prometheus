#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

cd web/ui
find static -type f -not -name '*.gz' -print0 | xargs -0 tar czf static.tar.gz
