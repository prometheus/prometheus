#!/usr/bin/env bash

# If either of the two tests below fail, you may need to install GNU coreutils
# in your environment.

if [ ! -x "$(which readlink)" ]; then
  echo "readlink tool cannot be found." > /dev/stderr
  exit 1
fi

if [ ! -x "$(which dirname)" ]; then
  echo "dirname tool cannot be found." > /dev/stderr
  exit 1
fi

readonly binary="${0}"
readonly binary_path="$(readlink -f ${binary})"
readonly binary_directory="$(dirname ${binary_path})"

if [ -n "${LD_LIBRARY_PATH}" ]; then
  export LD_LIBRARY_PATH="${binary_directory}/lib:${LD_LIBRARY_PATH}"
fi

if [ -n "${DYLD_LIBRARY_PATH}" ]; then
  export DYLD_LIBRARY_PATH="${binary_directory}/lib:${DYLD_LIBRARY_PATH}"
fi

"${binary_directory}/prometheus" "${@}" &
