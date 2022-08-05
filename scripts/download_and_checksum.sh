#!/bin/bash

url="${1}"
target_dir="${2}"
checksum_file="${3}"

target_filename=$(basename "${url}")
target_path="${target_dir}/${target_filename}"

curl -s -L "${url}" -o "${target_path}"
expected_checksum=$(grep "${target_filename}" "${checksum_file}" | awk '{ print $1 }')
if [ "$(shasum -a 256 "${target_path}" | awk '{print $1}')" != "${expected_checksum}" ]
then
	echo "Error: ${url} failed checksum" > /dev/stderr
    exit 1
fi
