#!/bin/bash
set -xe

# Go-fuzz doesn't support modules yet, so ensure we do everything in the old style GOPATH way
export GO111MODULE="off"

# Install go-fuzz
go get -u github.com/dvyukov/go-fuzz/go-fuzz github.com/dvyukov/go-fuzz/go-fuzz-build

# Target names on fuzzit.dev
TARGETS=("promql-parse-metric" "promql-parse-open-metric" "promql-parse-metric-selector" "promql-parse-expr")

# Prometheus fuzz functions
FUZZ_FUNCTIONS=("FuzzParseMetric" "FuzzParseOpenMetric" "FuzzParseMetricSelector" "FuzzParseExpr")

# Compiling prometheus fuzz targets in fuzz.go with go-fuzz (https://github.com/dvyukov/go-fuzz) and libFuzzer support
for ((i=0;i<${#TARGETS[@]};++i));
do
    go-fuzz-build -libfuzzer -func ${FUZZ_FUNCTIONS[i]} -o ${TARGETS[i]}.a ./promql
    clang-9 -fsanitize=fuzzer ${TARGETS[i]}.a -o ${TARGETS[i]}
done

# Install fuzzit CLI
wget -q -O fuzzit https://github.com/fuzzitdev/fuzzit/releases/download/v2.4.45/fuzzit_Linux_x86_64
chmod a+x fuzzit

for TARGET in "${TARGETS[@]}"
do
    ./fuzzit create job --type $1 prometheus/${TARGET} ${TARGET}
done
