#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

cd web/ui
cp embed.go.tmpl embed.go

# gzip option '-k' does not exist on RHEL7 based instances, so remove it
GZIP_OPTS="-fk" && [[ $(awk -F= '/^NAME/{print $2}' /etc/os-release) =~ (CentOS|Red Hat).* ]] && [[ $(awk -F= '/^VERSION_ID/{print $2}' /etc/os-release) =~ 7.* ]] && GZIP_OPTS="-f"

find static -type f -name '*.gz' -delete
find static -type f -exec gzip $GZIP_OPTS '{}' \; -print0 | xargs -0 -I % echo %.gz | xargs echo //go:embed >> embed.go
echo var EmbedFS embed.FS >> embed.go
