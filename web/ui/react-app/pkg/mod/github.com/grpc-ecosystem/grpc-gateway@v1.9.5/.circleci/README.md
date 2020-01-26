## gRPC-Gateway CI testing setup

Contained within is the CI test setup for the Gateway. It runs on Circle CI.

### I want to regenerate the files after making changes!

Great, it should be as simple as thus (run from the root of the directory):

```bash
$ docker run -v $(pwd):/go/src/github.com/grpc-ecosystem/grpc-gateway --rm jfbrandhorst/grpc-gateway-build-env:1.12 \
    /bin/bash -c 'cd /go/src/github.com/grpc-ecosystem/grpc-gateway && \
        make realclean && \
        make examples SWAGGER_CODEGEN="${SWAGGER_CODEGEN}"'
```

If this has resulted in some file changes in the repo, please ensure you check those in with your merge request.

### Whats up with the Dockerfile?

The `Dockerfile` in this folder is used as the build environment when regenerating the files (see above).
The canonical repository for this Dockerfile is `jfbrandhorst/grpc-gateway-build-env`. Please request access
before attempting to make any changes to the Dockerfile.
