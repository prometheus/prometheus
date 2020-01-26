#!/usr/bin/env bash

#
# Prerequisite:
# - install gRPC tools
#   python -m pip install grpcio-tools
#
# Learn more about gRPC Python tools at https://grpc.io/docs/quickstart/python.html.
#
# To generate:
#
# git clone git@github.com:census-instrumentation/opencensus-proto.git
#
# cd opencensus-proto/src
# ./mkpygen.sh

OUTDIR="../gen-python"
mkdir -p $OUTDIR

python -m grpc_tools.protoc -I ./ --python_out=$OUTDIR opencensus/proto/stats/v1/stats.proto \
    && python -m grpc_tools.protoc -I ./ --python_out=$OUTDIR opencensus/proto/metrics/v1/metrics.proto \
    && python -m grpc_tools.protoc -I ./ --python_out=$OUTDIR opencensus/proto/resource/v1/resource.proto \
    && python -m grpc_tools.protoc -I ./ --python_out=$OUTDIR opencensus/proto/trace/v1/trace.proto \
    && python -m grpc_tools.protoc -I ./ --python_out=$OUTDIR opencensus/proto/trace/v1/trace_config.proto \
    && python -m grpc_tools.protoc -I ./ --python_out=$OUTDIR opencensus/proto/agent/common/v1/common.proto \
    && python -m grpc_tools.protoc -I ./ --python_out=$OUTDIR --grpc_python_out=$OUTDIR opencensus/proto/agent/metrics/v1/metrics_service.proto \
    && python -m grpc_tools.protoc -I ./ --python_out=$OUTDIR --grpc_python_out=$OUTDIR opencensus/proto/agent/trace/v1/trace_service.proto
