#!/usr/bin/env bash
# Script: check_go_mod_versions.sh
# Description: Checks if all go.mod files in the current directory and its subdirectories have matching Go versions.

# Read all go.mod files into an array
readarray -t mod_files < <(find . -type f -name go.mod)

# Print the list of go.mod files being checked
echo "Checking files ${mod_files[@]}"

# Extract unique Go versions from go.mod files and count them
matches=$(awk '$1 == "go" {print $2}' "${mod_files[@]}" | sort -u | wc -l)

# Check if there is only one unique Go version across all files
if [[ "${matches}" -ne 1 ]]; then
  echo 'Not all go.mod files have matching go versions'
  exit 1
fi

# If all go.mod files have matching Go versions, print success message
echo 'All go.mod files have matching go versions'
