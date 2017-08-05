#!/bin/bash

# Get date in YYYYMMMDD format
builddate=$(date -u +%Y%m%d)
# Get latest commit id (short version)
commitid=$(git log --pretty=format:'%h' -n 1)
# Get branch version based on remote branch
branchid=$(git rev-parse --abbrev-ref --symbolic-full-name @{u} | sed -e 's?origin/??' -e 's?release-??')
go build -ldflags "-X main.buildDate=$builddate -X main.commitID=$commitid -X main.branchID=$branchid"
