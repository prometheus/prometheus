The `ui` package contains static files and templates used in the web UI. For
easier distribution they are statically compiled into the Prometheus binary
using the go-bindata tool (c.f. Makefile).

During development it is more convenient to always use the files on disk to
directly see changes without recompiling.
Set the environment variable `DEBUG=1` and compile Prometheus for this to work.
This is for development purposes only.

After making changes to any file, run `make assets` before committing to update
the generated inline version of the file.
