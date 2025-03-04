#!/bin/bash
if [ "$#" -lt 2 ]; then
	echo -e "Usage: $0 <filename> <line>\nFor proper work there git command should be available, and you should call this script inside git repository dir."
	exit 0
fi

result=$(echo "$1:$2:$(git rev-parse HEAD)" | md5sum | awk '{print $1}')
echo ${result::16}

