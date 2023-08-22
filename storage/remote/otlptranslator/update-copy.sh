#!/bin/bash

OTEL_VERSION=v0.81.0

git clone https://github.com/open-telemetry/opentelemetry-collector-contrib ./tmp
cd ./tmp
git checkout $OTEL_VERSION
cd ..
rm -rf ./prometheusremotewrite/*
cp -r ./tmp/pkg/translator/prometheusremotewrite/*.go ./prometheusremotewrite
rm -rf ./prometheusremotewrite/*_test.go
rm -rf ./tmp

sed -i '' 's#github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus#github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheus#g' ./prometheusremotewrite/*.go
sed -i '' '1s#^#// DO NOT EDIT. COPIED AS-IS. SEE README.md\n\n#g' ./prometheusremotewrite/*.go
