#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

cd web/ui
cp embed.go.tmpl embed.go

# gzip option '-k' may not always exist in the latest gzip available on different distros
GZIP_OPTS="-fk" && [[ ! $(gzip -k -h 2>/dev/null) ]] && GZIP_OPTS="-f"

find static -type f -name '*.gz' -delete
find static -type f -exec gzip $GZIP_OPTS '{}' \; -print0 | xargs -0 -I % echo %.gz | xargs echo //go:embed >> embed.go
echo var EmbedFS embed.FS >> embed.go
