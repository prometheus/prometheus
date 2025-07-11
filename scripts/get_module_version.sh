#!/usr/bin/env bash

# if no version string is passed as an argument, read VERSION file
if [ $# -eq 0 ]; then
    VERSION="$(< VERSION)"
else
    VERSION=$1
fi


# Remove leading 'v' if present
VERSION="${VERSION#v}"

# Extract MAJOR, MINOR, and REST
MAJOR="${VERSION%%.*}"
MINOR="${VERSION#*.}"; MINOR="${MINOR%%.*}"
REST="${VERSION#*.*.}"

# Format and output based on MAJOR version
if [[ "$MAJOR" == "2" ]]; then
    echo "0.$MINOR.$REST"
elif [[ "$MAJOR" == "3" ]]; then
    printf "0.3%02d.$REST\n" "$MINOR"
fi
