#!/usr/bin/env bash
set -e

# Get the parent directory of where this script is.
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ] ; do SOURCE="$(readlink "$SOURCE")"; done
DIR="$( cd -P "$( dirname "$SOURCE" )/.." && pwd )"

# Change into that dir because we expect that.
cd $DIR

# Make sure build tools are abailable.
make -f GNUmakefile tools

# Now we are ready to do a clean build of everything.
make -f GNUmakefile bin

exit 0
