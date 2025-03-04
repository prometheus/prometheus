# This script returns target name for making
# static lib with C bindings, depending on
# OPT and SANITIZERS env variables.
# Supported OPT env var values: dbg, opt
# Supported SANITIZERS env var values: with_sanitizers, no_sanitizers
result=""
need_underscore=0
if [ "$OPT" = "dbg" ]; then
	result="dbg"
	need_underscore=1
fi
if [ "$SANITIZERS" = "with_sanitizers" ]; then
	if [ "$need_underscore" = "1" ]; then
		result="${result}_"
	fi
	result="${result}asan"
fi
echo "$result"
