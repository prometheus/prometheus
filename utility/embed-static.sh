#!/bin/sh

set -e

cat <<EOF
package blob
var files = map [string] map [string] []byte {
EOF

ORIGINAL_PWD=${PWD}

for dir in $@
do
  cd "${dir}"
  echo "\"$(basename ${dir})\": {"

  # Do not embed map files and the non-minified bootstrap files.
  # TODO(beorn7): There should be a better solution than hardcoding the
  # exclusion here. We might want to switch to a less makeshift way of
  # embedding files into the binary anyway...
  find . -type f \! -name \*.map \! -name bootstrap.js \! -name bootstrap-theme.css \! -name bootstrap.css | while read file
  do
    name=$(echo "${file}"|sed 's|\.\/||')
    echo "\"$name\": {"
    gzip -9 -c "${file}" | xxd -p |sed 's/\(..\)/0x\1, /g'
    echo "},"
    echo
  done
  echo "},"
  cd "${ORIGINAL_PWD}"
done
echo '}'
