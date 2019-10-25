The `ui` directory contains static files and templates used in the web UI. For
easier distribution they are statically compiled into the Prometheus binary
using the vfsgen library (c.f. Makefile).

During development it is more convenient to always use the files on disk to
directly see changes without recompiling.
To make this work, remove the `builtinassets` build tag in the `flags` entry
in `.promu.yml`, and then `make build` (or build Prometheus using
`go build ./cmd/prometheus`).

This will serve all files from your local filesystem.
This is for development purposes only.
