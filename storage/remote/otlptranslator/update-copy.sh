#!/bin/bash

OTEL_VERSION=v0.71.0

git clone https://github.com/open-telemetry/opentelemetry-collector-contrib ./tmp
cd ./tmp
git checkout $OTEL_VERSION
cd ..
rm -rf ./prometheus/*
rm -rf ./prometheusremotewrite/*
cp -r ./tmp/pkg/translator/prometheus/*.go ./prometheus
cp -r ./tmp/pkg/translator/prometheusremotewrite/*.go ./prometheusremotewrite
rm -rf ./prometheus/*_test.go
rm -rf ./prometheusremotewrite/*_test.go
rm -rf ./tmp

sed -i '' 's#github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus#github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheus#g' ./prometheus/*.go
sed -i '' 's#github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus#github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheus#g' ./prometheusremotewrite/*.go