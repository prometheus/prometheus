#!/bin/sh

cat <<EOF
package blob
var files = map [string] map [string] []byte {
EOF

type_file=`tempfile`
cat <<EOF > $type_file
var types = map [string] map [string] string {
EOF

CDIR=`pwd`
for dir in $@
do
  cd "$dir"
  echo -e "\t\"`basename $dir`\": {"
  echo -e "\t\"`basename $dir`\": {" >> $type_file

  find -type f | while read file
  do
    mime=`mimetype -b "$file"`
    name=`echo "$file"|sed 's|\.\/||'`
    echo -e "\t\t\"$name\": \"$mime\","  >> $type_file
  
    echo -e "\t\t\"$name\": {"
    gzip -9 -c "$file" | xxd -p |sed 's/\(..\)/0x\1, /g'
    echo -e "\t\t},"
    echo
  done
  echo -e "\t}," >> $type_file
  echo -e "\t},"
  cd $CDIR
done
echo '}'
cat $type_file
echo '}'
rm $type_file
