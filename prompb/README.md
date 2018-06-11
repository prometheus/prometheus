The compiled protobufs are version controlled and you won't normally need to
re-compile them when building Prometheus.

If however you have modified the defs and do need to re-compile, run
`./scripts/genproto.sh` from the parent dir.

In order for the script to run, you'll need `protoc` (version 3.5) in your
PATH, and the following Go packages installed:

- github.com/gogo/protobuf
- github.com/gogo/protobuf/protoc-gen-gogofast
- github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/
- github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
- golang.org/x/tools/cmd/goimports
