#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

export STATIC_DIR=static
PREBUILT_ASSETS_STATIC_DIR=${PREBUILT_ASSETS_STATIC_DIR:-}
if [ -n "$PREBUILT_ASSETS_STATIC_DIR" ]; then
    STATIC_DIR=$(realpath $PREBUILT_ASSETS_STATIC_DIR)
fi

cd web/ui
cp embed.go.tmpl embed.go

GZIP_OPTS="-fkn"
# gzip option '-k' may not always exist in the latest gzip available on different distros.
if ! gzip -k -h &>/dev/null; then GZIP_OPTS="-fn"; fi

mkdir -p static
find static -type f -name '*.gz' -delete

# Compress files from the prebuilt static directory and replicate the structure in the current static directory
find "${STATIC_DIR}" -type f ! -name '*.gz' -exec bash -c '
    for file; do
        dest="${file#${STATIC_DIR}}"
        mkdir -p "static/$(dirname "$dest")"
        gzip '"$GZIP_OPTS"' "$file" -c > "static/${dest}.gz"
    done
' bash {} +

# Append the paths of gzipped files to embed.go
find static -type f -name '*.gz' -print0 | sort -z | xargs -0 echo //go:embed >> embed.go

echo var EmbedFS embed.FS >> embed.go
