# grpc-gateway

[![release](https://img.shields.io/github/release/grpc-ecosystem/grpc-gateway.svg?style=flat-square)](https://github.com/grpc-ecosystem/grpc-gateway/releases) [![CircleCI](https://img.shields.io/circleci/project/github/grpc-ecosystem/grpc-gateway/master.svg?style=flat-square)](https://circleci.com/gh/grpc-ecosystem/grpc-gateway) [![coverage](https://img.shields.io/codecov/c/github/grpc-ecosystem/grpc-gateway/master.svg?style=flat-square)](https://codecov.io/gh/grpc-ecosystem/grpc-gateway) [![license](https://img.shields.io/github/license/grpc-ecosystem/grpc-gateway.svg?style=flat-square)](LICENSE.txt)

The grpc-gateway is a plugin of the Google protocol buffers compiler
[protoc](https://github.com/protocolbuffers/protobuf).
It reads protobuf service definitions and generates a reverse-proxy server which
'translates a RESTful HTTP API into gRPC. This server is generated according to the
[`google.api.http`](https://github.com/googleapis/googleapis/blob/master/google/api/http.proto#L46)
annotations in your service definitions.

This helps you provide your APIs in both gRPC and RESTful style at the same time.

![architecture introduction diagram](https://docs.google.com/drawings/d/12hp4CPqrNPFhattL_cIoJptFvlAqm5wLQ0ggqI5mkCg/pub?w=749&amp;h=370)

## Check out our [documentation](https://grpc-ecosystem.github.io/grpc-gateway/)!

## Background
gRPC is great -- it generates API clients and server stubs in many programming
languages, it is fast, easy-to-use, bandwidth-efficient and its design is
combat-proven by Google. However, you might still want to provide a traditional
RESTful JSON API as well. Reasons can range from maintaining
backwards-compatibility, supporting languages or clients not well supported by
gRPC, to simply maintaining the aesthetics and tooling involved with a RESTful
JSON architecture.

This project aims to provide that HTTP+JSON interface to your gRPC service.
A small amount of configuration in your service to attach HTTP semantics is all
that's needed to generate a reverse-proxy with this library.

## Installation

The grpc-gateway requires a local installation of the Google protocol buffers
compiler `protoc` v3.0.0 or above. Please install this via your local package
manager or by downloading one of the releases from the official repository:

https://github.com/protocolbuffers/protobuf/releases


Then use `go get -u` to download the following packages:

```sh
go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
go get -u github.com/golang/protobuf/protoc-gen-go
```

This will place three binaries in your `$GOBIN`;

* `protoc-gen-grpc-gateway`
* `protoc-gen-swagger`
* `protoc-gen-go`

Make sure that your `$GOBIN` is in your `$PATH`.

## Usage

1. Define your [gRPC](https://grpc.io/docs/) service using protocol buffers 

   `your_service.proto`:
   ```protobuf
   syntax = "proto3";
   package example;
   message StringMessage {
     string value = 1;
   }

   service YourService {
     rpc Echo(StringMessage) returns (StringMessage) {}
   }
   ```

2. Add a [`google.api.http`](https://github.com/googleapis/googleapis/blob/master/google/api/http.proto#L46)
annotation to your .proto file

   `your_service.proto`:
   ```diff
    syntax = "proto3";
    package example;
   +
   +import "google/api/annotations.proto";
   +
    message StringMessage {
      string value = 1;
    }

    service YourService {
   -  rpc Echo(StringMessage) returns (StringMessage) {}
   +  rpc Echo(StringMessage) returns (StringMessage) {
   +    option (google.api.http) = {
   +      post: "/v1/example/echo"
   +      body: "*"
   +    };
   +  }
    }
   ```

   If you do not want to modify the proto file for use with grpc-gateway you can
   alternatively use an external
   [gRPC Service Configuration](https://cloud.google.com/endpoints/docs/grpc/grpc-service-config) file.
   [Check our documentation](https://grpc-ecosystem.github.io/grpc-gateway/docs/grpcapiconfiguration.html)
   for more information.

3. Generate gRPC stub

   The following generates gRPC code for Golang based on `path/to/your_service.proto`:
   ```sh
   protoc -I/usr/local/include -I. \
     -I$GOPATH/src \
     -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
     --go_out=plugins=grpc:. \
     path/to/your_service.proto
   ```

   It will generate a stub file `path/to/your_service.pb.go`.

4. Implement your service in gRPC as usual

   1. (Optional) Generate gRPC stub in the [other programming languages](https://grpc.io/docs/).

     For example, the following generates gRPC code for Ruby based on `path/to/your_service.proto`:
     ```sh
     protoc -I/usr/local/include -I. \
       -I$GOPATH/src \
       -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
       --ruby_out=. \
       path/to/your_service.proto

     protoc -I/usr/local/include -I. \
       -I$GOPATH/src \
       -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
       --plugin=protoc-gen-grpc=grpc_ruby_plugin \
       --grpc-ruby_out=. \
       path/to/your_service.proto
     ```
   2. Add the googleapis-common-protos gem (or your language equivalent) as a dependency to your project.
   3. Implement your gRPC service stubs

5. Generate reverse-proxy using `protoc-gen-grpc-gateway`

   ```sh
   protoc -I/usr/local/include -I. \
     -I$GOPATH/src \
     -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
     --grpc-gateway_out=logtostderr=true:. \
     path/to/your_service.proto
   ```

   It will generate a reverse proxy `path/to/your_service.pb.gw.go`.

6. Write an entrypoint for the HTTP reverse-proxy server

   ```go
   package main
 
   import (
     "context"  // Use "golang.org/x/net/context" for Golang version <= 1.6
     "flag"
     "net/http"
 
     "github.com/golang/glog"
     "github.com/grpc-ecosystem/grpc-gateway/runtime"
     "google.golang.org/grpc"
 
     gw "path/to/your_service_package"  // Update
   )
 
   var (
     // command-line options:
     // gRPC server endpoint
     grpcServerEndpoint = flag.String("grpc-server-endpoint",  "localhost:9090", "gRPC server endpoint")
   )
 
   func run() error {
     ctx := context.Background()
     ctx, cancel := context.WithCancel(ctx)
     defer cancel()
 
     // Register gRPC server endpoint
     // Note: Make sure the gRPC server is running properly and accessible
     mux := runtime.NewServeMux()
     opts := []grpc.DialOption{grpc.WithInsecure()}
     err := gw.RegisterYourServiceHandlerFromEndpoint(ctx, mux,  *grpcServerEndpoint, opts)
     if err != nil {
       return err
     }
 
     // Start HTTP server (and proxy calls to gRPC server endpoint)
     return http.ListenAndServe(":8081", mux)
   }
 
   func main() {
     flag.Parse()
     defer glog.Flush()
 
     if err := run(); err != nil {
       glog.Fatal(err)
     }
   }
   ```

7. (Optional) Generate swagger definitions using `protoc-gen-swagger`

   ```sh
   protoc -I/usr/local/include -I. \
     -I$GOPATH/src \
     -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
     --swagger_out=logtostderr=true:. \
     path/to/your_service.proto
   ```

## Parameters and flags
`protoc-gen-grpc-gateway` supports custom mapping from Protobuf `import` to
Golang import paths. They are compatible to
[the parameters with same names in `protoc-gen-go`](https://github.com/golang/protobuf#parameters)
(except `source_relative`).

In addition we also support the `request_context` parameter in order to use the
`http.Request`'s Context (only for Go 1.7 and above). This parameter can be
useful to pass request scoped context between the gateway and the gRPC service.

`protoc-gen-grpc-gateway` also supports some more command line flags to control
logging. You can give these flags together with parameters above. Run
`protoc-gen-grpc-gateway --help` for more details about the flags.

## More Examples
More examples are available under `examples` directory.
* `proto/examplepb/echo_service.proto`, `proto/examplepb/a_bit_of_everything.proto`, `proto/examplepb/unannotated_echo_service.proto`: service definition
  * `proto/examplepb/echo_service.pb.go`, `proto/examplepb/a_bit_of_everything.pb.go`, `proto/examplepb/unannotated_echo_service.pb.go`: [generated] stub of the service
  * `proto/examplepb/echo_service.pb.gw.go`, `proto/examplepb/a_bit_of_everything.pb.gw.go`, `proto/examplepb/uannotated_echo_service.pb.gw.go`: [generated] reverse proxy for the service
  * `proto/examplepb/unannotated_echo_service.yaml`: gRPC API Configuration for ```unannotated_echo_service.proto```
* `server/main.go`: service implementation
* `main.go`: entrypoint of the generated reverse proxy

To use the same port for custom HTTP handlers (e.g. serving `swagger.json`),
gRPC-gateway, and a gRPC server, see
[this example by CoreOS](https://github.com/philips/grpc-gateway-example/blob/master/cmd/serve.go)
(and its accompanying [blog post](https://coreos.com/blog/gRPC-protobufs-swagger.html)).

## Features

### Supported

* Generating JSON API handlers.
* Method parameters in request body.
* Method parameters in request path.
* Method parameters in query string.
* Enum fields in path parameter (including repeated enum fields).
* Mapping streaming APIs to newline-delimited JSON streams.
* Mapping HTTP headers with `Grpc-Metadata-` prefix to gRPC metadata (prefixed with `grpcgateway-`)
* Optionally emitting API definitions for
[OpenAPI (Swagger) v2](https://swagger.io/docs/specification/2-0/basic-structure/).
* Setting [gRPC timeouts](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md#requests)
through inbound HTTP `Grpc-Timeout` header.
* Partial support for [gRPC API Configuration](https://cloud.google.com/endpoints/docs/grpc/grpc-service-config)
files as an alternative to annotation.
* Automatically translating PATCH requests into Field Mask gRPC requests. See
[the docs](https://grpc-ecosystem.github.io/grpc-gateway/docs/patch.html)
for more information.

### No plan to support
But patch is welcome.
* Method parameters in HTTP headers.
* Handling trailer metadata.
* Encoding request/response body in XML.
* True bi-directional streaming.

# Mapping gRPC to HTTP

* [How gRPC error codes map to HTTP status codes in the response](https://github.com/grpc-ecosystem/grpc-gateway/blob/master/runtime/errors.go#L15).
* HTTP request source IP is added as `X-Forwarded-For` gRPC request header.
* HTTP request host is added as `X-Forwarded-Host` gRPC request header.
* HTTP `Authorization` header is added as `authorization` gRPC request header.
* Remaining Permanent HTTP header keys (as specified by the IANA
[here](http://www.iana.org/assignments/message-headers/message-headers.xhtml)
are prefixed with `grpcgateway-` and added with their values to gRPC request
header.
* HTTP headers that start with 'Grpc-Metadata-' are mapped to gRPC metadata
(prefixed with `grpcgateway-`).
* While configurable, the default {un,}marshaling uses
[jsonpb](https://godoc.org/github.com/golang/protobuf/jsonpb) with
`OrigName: true`.

# Contribution
See [CONTRIBUTING.md](http://github.com/grpc-ecosystem/grpc-gateway/blob/master/CONTRIBUTING.md).

# License
grpc-gateway is licensed under the BSD 3-Clause License.
See [LICENSE.txt](https://github.com/grpc-ecosystem/grpc-gateway/blob/master/LICENSE.txt) for more details.
