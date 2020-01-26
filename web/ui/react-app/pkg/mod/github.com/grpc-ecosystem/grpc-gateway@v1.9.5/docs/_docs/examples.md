---
category: documentation
---

# Examples

Examples are available under `examples` directory.
* `proto/examplepb/echo_service.proto`, `proto/examplepb/a_bit_of_everything.proto`, `proto/examplepb/unannotated_echo_service.proto`: service definition
  * `proto/examplepb/echo_service.pb.go`, `proto/examplepb/a_bit_of_everything.pb.go`, `proto/examplepb/unannotated_echo_service.pb.go`: [generated] stub of the service
  * `proto/examplepb/echo_service.pb.gw.go`, `proto/examplepb/a_bit_of_everything.pb.gw.go`, `proto/examplepb/uannotated_echo_service.pb.gw.go`: [generated] reverse proxy for the service
  * `proto/examplepb/unannotated_echo_service.yaml`: gRPC API Configuration for ```unannotated_echo_service.proto```
* `server/main.go`: service implementation
* `main.go`: entrypoint of the generated reverse proxy

To use the same port for custom HTTP handlers (e.g. serving `swagger.json`), gRPC-gateway, and a gRPC server, see [this code example by CoreOS](https://github.com/philips/grpc-gateway-example/blob/master/cmd/serve.go) (and its accompanying [blog post](https://coreos.com/blog/gRPC-protobufs-swagger.html))


