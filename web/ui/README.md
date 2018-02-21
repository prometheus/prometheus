The `ui` package contains static files and templates used in the web UI. For
easier distribution they are statically compiled into the Prometheus binary
using the go-bindata tool (c.f. Makefile).

During development it is more convenient to always use the files on disk to
directly see changes without recompiling.
Set the environment variable `DEBUG=1` and run `make assets` for this to work. 
This will put `go-bindata` in DEBUG mode where it serves from your local filesystem.
This is for development purposes only.

After making changes to any file, run `make assets` (without the `DEBUG=1`) before committing to update
the generated inline version of the file.
