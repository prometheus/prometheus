#!/bin/sh

cat <<EOF
package blob
var files = map [string] map [string] []byte {
EOF

CDIR=`pwd`
for dir in $@
do
  cd "$dir"
  echo -e "\t\"`basename $dir`\": {"

  find -type f | while read file
  do
    name=`echo "$file"|sed 's|\.\/||'`
    echo -e "\t\t\"$name\": {"
    gzip -9 -c "$file" | xxd -p |sed 's/\(..\)/0x\1, /g'
    echo -e "\t\t},"
    echo
  done
  echo -e "\t},"
  cd $CDIR
done
echo '}'
