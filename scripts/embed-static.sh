#!/bin/sh

set -e

cat <<EOF
package blob
var files = map [string] map [string] []byte {
EOF

ORIGINAL_PWD=${PWD}

for dir in $@
do
  cd "${dir}" > /dev/null
  echo "\"$(basename ${dir})\": {"

  # Do not embed map files and the non-minified bootstrap files.
  # TODO(beorn7): There should be a better solution than hardcoding the
  # exclusion here. We might want to switch to a less makeshift way of
  # embedding files into the binary anyway...
  find . -type f \! -name \*.map \! -name bootstrap.js \! -name bootstrap-theme.css \! -name bootstrap.css | while read file
  do
    name=$(echo "${file}"|sed 's|\.\/||')
    # Using printf here instead of "echo -n" because the latter doesn't work on Mac OS X:
    # http://hints.macworld.com/article.php?story=20071106192548833.
    printf "\"$name\": []byte(\""
    # The second newline deletion at the end is required for Mac OS X as well,
    # as sed outputs a trailing newline there.
    gzip -9 -c "${file}" | xxd -p | tr -d '\n' | sed 's/\(..\)/\\x\1/g' | tr -d '\n'
    echo "\"),"
    echo
  done
  echo "},"
  cd "${ORIGINAL_PWD}"
done
echo '}'
