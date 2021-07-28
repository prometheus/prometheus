#!/bin/sh

ulimit -n `ulimit -Hn`

exec /bin/prometheus $*
