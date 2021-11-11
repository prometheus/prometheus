#!/usr/bin/env bash
#
# [de]compress static assets

find web/ui/templates web/ui/static -type f -exec gzip "$@" {} \;
