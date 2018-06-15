#!/usr/bin/env bash
set -e
set -E
set -T

BATS_COUNT_ONLY=""
if [ "$1" = "-c" ]; then
  BATS_COUNT_ONLY=1
  shift
fi

BATS_EXTENDED_SYNTAX=""
if [ "$1" = "-x" ]; then
  BATS_EXTENDED_SYNTAX="$1"
  shift
fi

BATS_TEST_FILENAME="$1"
if [ -z "$BATS_TEST_FILENAME" ]; then
  echo "usage: bats-exec <filename>" >&2
  exit 1
elif [ ! -f "$BATS_TEST_FILENAME" ]; then
  echo "bats: $BATS_TEST_FILENAME does not exist" >&2
  exit 1
else
  shift
fi

BATS_TEST_DIRNAME="$(dirname "$BATS_TEST_FILENAME")"
BATS_TEST_NAMES=()

load() {
  local name="$1"
  local filename

  if [ "${name:0:1}" = "/" ]; then
    filename="${name}"
  else
    filename="$BATS_TEST_DIRNAME/${name}.bash"
  fi

  [ -f "$filename" ] || {
    echo "bats: $filename does not exist" >&2
    exit 1
  }

  source "${filename}"
}

run() {
  local e E T oldIFS
  [[ ! "$-" =~ e ]] || e=1
  [[ ! "$-" =~ E ]] || E=1
  [[ ! "$-" =~ T ]] || T=1
  set +e
  set +E
  set +T
  output="$("$@" 2>&1)"
  status="$?"
  oldIFS=$IFS
  IFS=$'\n' lines=($output)
  [ -z "$e" ] || set -e
  [ -z "$E" ] || set -E
  [ -z "$T" ] || set -T
  IFS=$oldIFS
}

setup() {
  true
}

teardown() {
  true
}

skip() {
  BATS_TEST_SKIPPED=${1:-1}
  BATS_TEST_COMPLETED=1
  exit 0
}

bats_test_begin() {
  BATS_TEST_DESCRIPTION="$1"
  if [ -n "$BATS_EXTENDED_SYNTAX" ]; then
    echo "begin $BATS_TEST_NUMBER $BATS_TEST_DESCRIPTION" >&3
  fi
  setup
}

bats_test_function() {
  local test_name="$1"
  BATS_TEST_NAMES["${#BATS_TEST_NAMES[@]}"]="$test_name"
}

bats_capture_stack_trace() {
  BATS_PREVIOUS_STACK_TRACE=( "${BATS_CURRENT_STACK_TRACE[@]}" )
  BATS_CURRENT_STACK_TRACE=()

  local test_pattern=" $BATS_TEST_NAME $BATS_TEST_SOURCE"
  local setup_pattern=" setup $BATS_TEST_SOURCE"
  local teardown_pattern=" teardown $BATS_TEST_SOURCE"

  local frame
  local index=1

  while frame="$(caller "$index")"; do
    BATS_CURRENT_STACK_TRACE["${#BATS_CURRENT_STACK_TRACE[@]}"]="$frame"
    if [[ "$frame" = *"$test_pattern"     || \
          "$frame" = *"$setup_pattern"    || \
          "$frame" = *"$teardown_pattern" ]]; then
      break
    else
      let index+=1
    fi
  done

  BATS_SOURCE="$(bats_frame_filename "${BATS_CURRENT_STACK_TRACE[0]}")"
  BATS_LINENO="$(bats_frame_lineno "${BATS_CURRENT_STACK_TRACE[0]}")"
}

bats_print_stack_trace() {
  local frame
  local index=1
  local count="${#@}"

  for frame in "$@"; do
    local filename="$(bats_trim_filename "$(bats_frame_filename "$frame")")"
    local lineno="$(bats_frame_lineno "$frame")"

    if [ $index -eq 1 ]; then
      echo -n "# ("
    else
      echo -n "#  "
    fi

    local fn="$(bats_frame_function "$frame")"
    if [ "$fn" != "$BATS_TEST_NAME" ]; then
      echo -n "from function \`$fn' "
    fi

    if [ $index -eq $count ]; then
      echo "in test file $filename, line $lineno)"
    else
      echo "in file $filename, line $lineno,"
    fi

    let index+=1
  done
}

bats_print_failed_command() {
  local frame="$1"
  local status="$2"
  local filename="$(bats_frame_filename "$frame")"
  local lineno="$(bats_frame_lineno "$frame")"

  local failed_line="$(bats_extract_line "$filename" "$lineno")"
  local failed_command="$(bats_strip_string "$failed_line")"
  echo -n "#   \`${failed_command}' "

  if [ $status -eq 1 ]; then
    echo "failed"
  else
    echo "failed with status $status"
  fi
}

bats_frame_lineno() {
  local frame="$1"
  local lineno="${frame%% *}"
  echo "$lineno"
}

bats_frame_function() {
  local frame="$1"
  local rest="${frame#* }"
  local fn="${rest%% *}"
  echo "$fn"
}

bats_frame_filename() {
  local frame="$1"
  local rest="${frame#* }"
  local filename="${rest#* }"

  if [ "$filename" = "$BATS_TEST_SOURCE" ]; then
    echo "$BATS_TEST_FILENAME"
  else
    echo "$filename"
  fi
}

bats_extract_line() {
  local filename="$1"
  local lineno="$2"
  sed -n "${lineno}p" "$filename"
}

bats_strip_string() {
  local string="$1"
  printf "%s" "$string" | sed -e "s/^[ "$'\t'"]*//" -e "s/[ "$'\t'"]*$//"
}

bats_trim_filename() {
  local filename="$1"
  local length="${#BATS_CWD}"

  if [ "${filename:0:length+1}" = "${BATS_CWD}/" ]; then
    echo "${filename:length+1}"
  else
    echo "$filename"
  fi
}

bats_debug_trap() {
  if [ "$BASH_SOURCE" != "$1" ]; then
    bats_capture_stack_trace
  fi
}

bats_error_trap() {
  BATS_ERROR_STATUS="$?"
  BATS_ERROR_STACK_TRACE=( "${BATS_PREVIOUS_STACK_TRACE[@]}" )
  trap - debug
}

bats_teardown_trap() {
  trap "bats_exit_trap" exit
  local status=0
  teardown >>"$BATS_OUT" 2>&1 || status="$?"

  if [ $status -eq 0 ]; then
    BATS_TEARDOWN_COMPLETED=1
  elif [ -n "$BATS_TEST_COMPLETED" ]; then
    BATS_ERROR_STATUS="$status"
    BATS_ERROR_STACK_TRACE=( "${BATS_CURRENT_STACK_TRACE[@]}" )
  fi

  bats_exit_trap
}

bats_exit_trap() {
  local status
  local skipped
  trap - err exit

  skipped=""
  if [ -n "$BATS_TEST_SKIPPED" ]; then
    skipped=" # skip"
    if [ "1" != "$BATS_TEST_SKIPPED" ]; then
      skipped+=" ($BATS_TEST_SKIPPED)"
    fi
  fi

  if [ -z "$BATS_TEST_COMPLETED" ] || [ -z "$BATS_TEARDOWN_COMPLETED" ]; then
    echo "not ok $BATS_TEST_NUMBER $BATS_TEST_DESCRIPTION" >&3
    bats_print_stack_trace "${BATS_ERROR_STACK_TRACE[@]}" >&3
    bats_print_failed_command "${BATS_ERROR_STACK_TRACE[${#BATS_ERROR_STACK_TRACE[@]}-1]}" "$BATS_ERROR_STATUS" >&3
    sed -e "s/^/# /" < "$BATS_OUT" >&3
    status=1
  else
    echo "ok ${BATS_TEST_NUMBER}${skipped} ${BATS_TEST_DESCRIPTION}" >&3
    status=0
  fi

  rm -f "$BATS_OUT"
  exit "$status"
}

bats_perform_tests() {
  echo "1..$#"
  test_number=1
  status=0
  for test_name in "$@"; do
    "$0" $BATS_EXTENDED_SYNTAX "$BATS_TEST_FILENAME" "$test_name" "$test_number" || status=1
    let test_number+=1
  done
  exit "$status"
}

bats_perform_test() {
  BATS_TEST_NAME="$1"
  if [ "$(type -t "$BATS_TEST_NAME" || true)" = "function" ]; then
    BATS_TEST_NUMBER="$2"
    if [ -z "$BATS_TEST_NUMBER" ]; then
      echo "1..1"
      BATS_TEST_NUMBER="1"
    fi

    BATS_TEST_COMPLETED=""
    BATS_TEARDOWN_COMPLETED=""
    trap "bats_debug_trap \"\$BASH_SOURCE\"" debug
    trap "bats_error_trap" err
    trap "bats_teardown_trap" exit
    "$BATS_TEST_NAME" >>"$BATS_OUT" 2>&1
    BATS_TEST_COMPLETED=1

  else
    echo "bats: unknown test name \`$BATS_TEST_NAME'" >&2
    exit 1
  fi
}

if [ -z "$TMPDIR" ]; then
  BATS_TMPDIR="/tmp"
else
  BATS_TMPDIR="${TMPDIR%/}"
fi

BATS_TMPNAME="$BATS_TMPDIR/bats.$$"
BATS_PARENT_TMPNAME="$BATS_TMPDIR/bats.$PPID"
BATS_OUT="${BATS_TMPNAME}.out"

bats_preprocess_source() {
  BATS_TEST_SOURCE="${BATS_TMPNAME}.src"
  { tr -d '\r' < "$BATS_TEST_FILENAME"; echo; } | bats-preprocess > "$BATS_TEST_SOURCE"
  trap "bats_cleanup_preprocessed_source" err exit
  trap "bats_cleanup_preprocessed_source; exit 1" int
}

bats_cleanup_preprocessed_source() {
  rm -f "$BATS_TEST_SOURCE"
}

bats_evaluate_preprocessed_source() {
  if [ -z "$BATS_TEST_SOURCE" ]; then
    BATS_TEST_SOURCE="${BATS_PARENT_TMPNAME}.src"
  fi
  source "$BATS_TEST_SOURCE"
}

exec 3<&1

if [ "$#" -eq 0 ]; then
  bats_preprocess_source
  bats_evaluate_preprocessed_source

  if [ -n "$BATS_COUNT_ONLY" ]; then
    echo "${#BATS_TEST_NAMES[@]}"
  else
    bats_perform_tests "${BATS_TEST_NAMES[@]}"
  fi
else
  bats_evaluate_preprocessed_source
  bats_perform_test "$@"
fi
