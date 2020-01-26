# How to contribute

Thank you for your contribution to grpc-gateway.
Here's the recommended process of contribution.

1. `go get github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway`
2. `cd $GOPATH/src/github.com/grpc-ecosystem/grpc-gateway`
3. hack, hack, hack...
4. Make sure that your change follows best practices in Go
   * [Effective Go](https://golang.org/doc/effective_go.html)
   * [Go Code Review Comments](https://golang.org/wiki/CodeReviewComments)
5. Make sure that `make test` passes. (use swagger-codegen 2.2.2, not newer versions)
6. Sign [a Contributor License Agreement](https://cla.developers.google.com/clas)
7. Open a pull request in Github

When you work on a larger contribution, it is also recommended that you get in touch
with us through the issue tracker.

### Code reviews
All submissions, including submissions by project members, require review.

### I want to regenerate the files after making changes!

Great, it should be as simple as thus (run from the root of the directory):

```bash
docker run -v $(pwd):/src/grpc-gateway --rm jfbrandhorst/grpc-gateway-build-env:1.12 \
    /bin/bash -c 'cd /src/grpc-gateway && \
        make realclean && \
        make examples SWAGGER_CODEGEN="${SWAGGER_CODEGEN}"'
docker run -itv $(pwd):/grpc-gateway -w /grpc-gateway --entrypoint /bin/bash --rm \
    l.gcr.io/google/bazel -c 'bazel run :gazelle; bazel run :buildifier'
```

If this has resulted in some file changes in the repo, please ensure you check those in with your merge request.
