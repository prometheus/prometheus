#!/usr/bin/env bash
set -e

version() {
  echo "Bats 0.4.0"
}

usage() {
  version
  echo "Usage: bats [-c] [-p | -t] <test> [<test> ...]"
}

help() {
  usage
  echo
  echo "  <test> is the path to a Bats test file, or the path to a directory"
  echo "  containing Bats test files."
  echo
  echo "  -c, --count    Count the number of test cases without running any tests"
  echo "  -h, --help     Display this help message"
  echo "  -p, --pretty   Show results in pretty format (default for terminals)"
  echo "  -t, --tap      Show results in TAP format"
  echo "  -v, --version  Display the version number"
  echo
  echo "  For more information, see https://github.com/sstephenson/bats"
  echo
}

resolve_link() {
  $(type -p greadlink readlink | head -1) "$1"
}

abs_dirname() {
  local cwd="$(pwd)"
  local path="$1"

  while [ -n "$path" ]; do
    cd "${path%/*}"
    local name="${path##*/}"
    path="$(resolve_link "$name" || true)"
  done

  pwd
  cd "$cwd"
}

expand_path() {
  { cd "$(dirname "$1")" 2>/dev/null
    local dirname="$PWD"
    cd "$OLDPWD"
    echo "$dirname/$(basename "$1")"
  } || echo "$1"
}

BATS_LIBEXEC="$(abs_dirname "$0")"
export BATS_PREFIX="$(abs_dirname "$BATS_LIBEXEC")"
export BATS_CWD="$(abs_dirname .)"
export PATH="$BATS_LIBEXEC:$PATH"

options=()
arguments=()
for arg in "$@"; do
  if [ "${arg:0:1}" = "-" ]; then
    if [ "${arg:1:1}" = "-" ]; then
      options[${#options[*]}]="${arg:2}"
    else
      index=1
      while option="${arg:$index:1}"; do
        [ -n "$option" ] || break
        options[${#options[*]}]="$option"
        let index+=1
      done
    fi
  else
    arguments[${#arguments[*]}]="$arg"
  fi
done

unset count_flag pretty
[ -t 0 ] && [ -t 1 ] && pretty="1"
[ -n "$CI" ] && pretty=""

for option in "${options[@]}"; do
  case "$option" in
  "h" | "help" )
    help
    exit 0
    ;;
  "v" | "version" )
    version
    exit 0
    ;;
  "c" | "count" )
    count_flag="-c"
    ;;
  "t" | "tap" )
    pretty=""
    ;;
  "p" | "pretty" )
    pretty="1"
    ;;
  * )
    usage >&2
    exit 1
    ;;
  esac
done

if [ "${#arguments[@]}" -eq 0 ]; then
  usage >&2
  exit 1
fi

filenames=()
for filename in "${arguments[@]}"; do
  if [ -d "$filename" ]; then
    shopt -s nullglob
    for suite_filename in "$(expand_path "$filename")"/*.bats; do
      filenames["${#filenames[@]}"]="$suite_filename"
    done
    shopt -u nullglob
  else
    filenames["${#filenames[@]}"]="$(expand_path "$filename")"
  fi
done

if [ "${#filenames[@]}" -eq 1 ]; then
  command="bats-exec-test"
else
  command="bats-exec-suite"
fi

if [ -n "$pretty" ]; then
  extended_syntax_flag="-x"
  formatter="bats-format-tap-stream"
else
  extended_syntax_flag=""
  formatter="cat"
fi

set -o pipefail execfail
exec "$command" $count_flag $extended_syntax_flag "${filenames[@]}" | "$formatter"
