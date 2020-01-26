#!/usr/bin/env bash

#
# Prerequisite:
# - install "grpc" gem
#   gem install grpc
#
# To generate:
# 
# git clone git@github.com:census-instrumentation/opencensus-proto.git
#
# cd opencensus-proto/src
# ./mkrubygen.sh

OUTDIR="../gen-ruby"
mkdir -p $OUTDIR

grpc_tools_ruby_protoc -I ./ --ruby_out=$OUTDIR --grpc_out=$OUTDIR opencensus/proto/stats/v1/stats.proto \
    && grpc_tools_ruby_protoc -I ./ --ruby_out=$OUTDIR --grpc_out=$OUTDIR opencensus/proto/metrics/v1/metrics.proto \
    && grpc_tools_ruby_protoc -I ./ --ruby_out=$OUTDIR --grpc_out=$OUTDIR opencensus/proto/resource/v1/resource.proto \
    && grpc_tools_ruby_protoc -I ./ --ruby_out=$OUTDIR --grpc_out=$OUTDIR opencensus/proto/trace/v1/trace.proto \
    && grpc_tools_ruby_protoc -I ./ --ruby_out=$OUTDIR --grpc_out=$OUTDIR opencensus/proto/trace/v1/trace_config.proto \
    && grpc_tools_ruby_protoc -I ./ --ruby_out=$OUTDIR --grpc_out=$OUTDIR opencensus/proto/agent/common/v1/common.proto \
    && grpc_tools_ruby_protoc -I ./ --ruby_out=$OUTDIR --grpc_out=$OUTDIR opencensus/proto/agent/metrics/v1/metrics_service.proto \
    && grpc_tools_ruby_protoc -I ./ --ruby_out=$OUTDIR --grpc_out=$OUTDIR opencensus/proto/agent/trace/v1/trace_service.proto
