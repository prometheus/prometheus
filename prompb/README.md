The compiled protobufs are version controlled and you won't normally need to
re-compile them when building Prometheus.

If however you have modified the defs and do need to re-compile, run
`make proto` from the parent dir.

In order for the script to run, you'll need `protoc` (version 3.5.1) in your
PATH.

