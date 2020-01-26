#!/usr/bin/env bash

# Run this if opencensus-proto is checked in the GOPATH.
# go get -d github.com/census-instrumentation/opencensus-proto
# to check in the repo to the GOAPTH.
#
# This also requires the grpc-gateway plugin.
# See: https://github.com/grpc-ecosystem/grpc-gateway#installation
#
# To generate:
#
# cd $(go env GOPATH)/census-instrumentation/opencensus-proto
# ./mkgogen.sh

OUTDIR="$(go env GOPATH)/src"

protoc --go_out=plugins=grpc:$OUTDIR opencensus/proto/stats/v1/stats.proto \
    && protoc --go_out=plugins=grpc:$OUTDIR opencensus/proto/metrics/v1/metrics.proto \
    && protoc --go_out=plugins=grpc:$OUTDIR opencensus/proto/resource/v1/resource.proto \
    && protoc --go_out=plugins=grpc:$OUTDIR opencensus/proto/trace/v1/trace.proto \
    && protoc --go_out=plugins=grpc:$OUTDIR opencensus/proto/trace/v1/trace_config.proto \
    && protoc -I=. --go_out=plugins=grpc:$OUTDIR opencensus/proto/agent/common/v1/common.proto \
    && protoc -I=. --go_out=plugins=grpc:$OUTDIR opencensus/proto/agent/metrics/v1/metrics_service.proto \
    && protoc -I=. --go_out=plugins=grpc:$OUTDIR opencensus/proto/agent/trace/v1/trace_service.proto \
    && protoc --grpc-gateway_out=logtostderr=true,grpc_api_configuration=./opencensus/proto/agent/trace/v1/trace_service_http.yaml:$OUTDIR opencensus/proto/agent/trace/v1/trace_service.proto \
    && protoc --grpc-gateway_out=logtostderr=true,grpc_api_configuration=./opencensus/proto/agent/metrics/v1/metrics_service_http.yaml:$OUTDIR opencensus/proto/agent/metrics/v1/metrics_service.proto

# Generate OpenApi (Swagger) documentation file for grpc-gateway endpoints.
OPENAPI_OUTDIR=../gen-openapi
mkdir -p $OPENAPI_OUTDIR
protoc --swagger_out=logtostderr=true,grpc_api_configuration=./opencensus/proto/agent/trace/v1/trace_service_http.yaml:$OPENAPI_OUTDIR \
  opencensus/proto/agent/trace/v1/trace_service.proto
protoc --swagger_out=logtostderr=true,grpc_api_configuration=./opencensus/proto/agent/metrics/v1/metrics_service_http.yaml:$OPENAPI_OUTDIR \
  opencensus/proto/agent/metrics/v1/metrics_service.proto
