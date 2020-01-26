---
title: Usage without annotations (gRPC API Configuration)
category: documentation
order: 100
---

# gRPC API Configuration
In some sitations annotating the .proto file of a service is not an option. For example you might not have control over the .proto file or you might want to expose the same gRPC API multiple times in completely different ways.

Google Cloud Platform offers a way to do this for services hosted with them called ["gRPC API Configuration"](https://cloud.google.com/endpoints/docs/grpc/grpc-service-config). It can be used to define the behavior of a gRPC API service without modifications to the service itself in the form of [YAML](https://en.wikipedia.org/wiki/YAML) configuration files.

grpc-gateway generators implement the [HTTP rules part](https://cloud.google.com/endpoints/docs/grpc-service-config/reference/rpc/google.api#httprule) of this specification. This allows you to take a completely unannotated service proto file, add a YAML file describing its HTTP endpoints and use them together like a annotated proto file with the grpc-gateway generators.

## Usage of gRPC API Configuration YAML files
The following is equivalent to the basic [usage example](usage.html) but without direct annotation for grpc-gateway in the .proto file. Only some steps require minor changes to use a gRPC API Configuration YAML file instead:

1. Define your service in gRPC as usual
   
   your_service.proto:
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

2. Instead of annotating the .proto file in this step leave it untouched and create a `your_service.yaml` with the following content:
    ```yaml
    type: google.api.Service
    config_version: 3

    http:
      rules:
      - selector: example.YourService.Echo
        post: /v1/example/echo
        body: "*"
    ```
    Use a [linter](http://www.yamllint.com/) to validate your YAML.

3. Generate gRPC stub as before
   
   ```sh
   protoc -I/usr/local/include -I. \
     -I$GOPATH/src \
     -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
     --go_out=plugins=grpc:. \
     path/to/your_service.proto
   ```
   
   It will generate a stub file `path/to/your_service.pb.go`.
4. Implement your service in gRPC as usual
   1. (Optional) Generate gRPC stub in the language you want.
     
     e.g.
     ```sh
     protoc -I/usr/local/include -I. \
       -I$GOPATH/src \
       -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
       --ruby_out=. \
       path/to/your/service_proto
     
     protoc -I/usr/local/include -I. \
       -I$GOPATH/src \
       -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
       --plugin=protoc-gen-grpc=grpc_ruby_plugin \
       --grpc-ruby_out=. \
       path/to/your/service.proto
     ```
   2. Add the googleapis-common-protos gem (or your language equivalent) as a dependency to your project.
   3. Implement your service

5. Generate reverse-proxy. Here we have to pass the path to the `your_service.yaml` in addition to the .proto file:
   ```sh
   protoc -I/usr/local/include -I. \
     -I$GOPATH/src \
     -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
     --grpc-gateway_out=logtostderr=true,grpc_api_configuration=path/to/your_service.yaml:. \
     path/to/your_service.proto
   ```
   
   This will generate a reverse proxy `path/to/your_service.pb.gw.go` that is identical to the one produced for the annotated proto.
 
   Note: After generating the code for each of the stubs, in order to build the code, you will want to run ```go get .``` from the directory containing the stubs.

6. Write an entrypoint 
   
   Now you need to write an entrypoint of the proxy server. This step is the same whether the file is annotated or not.
   ```go
   package main

   import (
     "flag"
     "net/http"
   
     "github.com/golang/glog"
     "golang.org/x/net/context"
     "github.com/grpc-ecosystem/grpc-gateway/runtime"
     "google.golang.org/grpc"
   	
     gw "path/to/your_service_package"
   )
   
   var (
     echoEndpoint = flag.String("echo_endpoint", "localhost:9090", "endpoint of YourService")
   )
   
   func run() error {
     ctx := context.Background()
     ctx, cancel := context.WithCancel(ctx)
     defer cancel()
   
     mux := runtime.NewServeMux()
     opts := []grpc.DialOption{grpc.WithInsecure()}
     err := gw.RegisterYourServiceHandlerFromEndpoint(ctx, mux, *echoEndpoint, opts)
     if err != nil {
       return err
     }
   
     return http.ListenAndServe(":8080", mux)
   }
   
   func main() {
     flag.Parse()
     defer glog.Flush()
   
     if err := run(); err != nil {
       glog.Fatal(err)
     }
   }
   ```

7. (Optional) Generate swagger definitions

Swagger generation in this step is equivalent to gateway generation. Again pass the path to the yaml file in addition to the proto:

   ```sh
   protoc -I/usr/local/include -I. \
     -I$GOPATH/src \
     -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
     --swagger_out=logtostderr=true,grpc_api_configuration=path/to/your_service.yaml:. \
     path/to/your_service.proto
   ```

All other steps work as before. If you want you can remove the googleapis include path in step 3 and 4 as the unannotated proto no longer requires them.
