#!/bin/bash

find . -name '*.proto' -print0 |
while IFS= read -r -d '' f
do
  commands=(
    # Import mangling.
    -e 's#import "gogoproto/gogo.proto";##'
    # Remove references to gogo.proto extensions.
    -e 's#option (gogoproto\.[a-z_]\+) = \(true\|false\);##'
    -e 's#\(, \)\?(gogoproto\.[a-z_]\+) = \(true\|false\),\?##'
    # gogoproto removal can result in empty brackets.
    -e 's# \[\]##'
    # gogoproto removal can result in four spaces on a line by itself.
    -e '/^    $/d'
  )
  sed -i "${commands[@]}" "$f"

  # gogoproto removal can leave a comma on the last element in a list.
  # This needs to run separately after all the commands above have finished
  # since it is multi-line and rewrites the output of the above patterns.
  sed -i -e '$!N; s#\(.*\),\([[:space:]]*\];\)#\1\2#; t; P; D;' "$f"
done
