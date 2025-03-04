#!/bin/bash
echo "# Make sure you've built prometheus with 'make build' first."
GOPATH=$(go env GOPATH)
IDIR="${GOPATH}/github.com/krajorama/prometheus"
promu build --prefix ${IDIR}
"${IDIR}/prometheus" --config.file="${IDIR}/prometheus.yml" --storage.tsdb.path="${IDIR}/data" --enable-feature="exemplar-storage,native-histograms" --log.level=debug
