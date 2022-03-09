#!/usr/bin/env bash
#
# [de]compress static assets

find web/ui/static -type f -exec gzip "$@" {} \;
