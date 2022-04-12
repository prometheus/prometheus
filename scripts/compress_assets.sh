#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

cd web/ui
cp embed.go.tmpl embed.go
find static -type f -name '*.gz' -delete
find static -type f -exec gzip -fk '{}' \; -print0 | xargs -0 -I % echo %.gz | xargs echo //go:embed >> embed.go
echo var EmbedFS embed.FS >> embed.go
