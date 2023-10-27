# This script returns build tags for Go tests,
# depending on OPT, SANITIZERS and FASTCGO env variables.
# Supported OPT env var values: dbg, opt
# Supported SANITIZERS env var values: with_sanitizers, no_sanitizers
# Supported FASTCGO env var values: with_fastcgo, without_fastcgo
tags=""

function append_tag_if_equal() {
	if [ "$1" = "$2" ]; then
		if [ "$tags" = "" ]; then
			tags="$3"
		else
			tags="${tags},$3"
		fi
	fi
}

append_tag_if_equal "$OPT"        "dbg"             "dbg"
append_tag_if_equal "$SANITIZERS" "with_sanitizers" "sanitizers"
append_tag_if_equal "$FASTCGO"    "without_fastcgo" "without_fastcgo"
append_tag_if_equal "$STATIC"     "static"          "static"

result=""
if [ "$tags" != "" ]; then
	result="--tags=$tags"
fi

if [ "$SANITIZERS" != "with_sanitizers" ]; then
	if [ "$result" != "" ]; then
		result="${result} "
	fi
	result="${result}--race"
fi

echo "$result"
