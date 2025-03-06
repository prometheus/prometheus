#!/bin/bash
GOPATH=$(go env GOPATH)
IDIR="${GOPATH}/github.com/krajorama/prometheus"
promu build --prefix ${IDIR}
"${IDIR}/prometheus" --config.file="${IDIR}/prometheus.yml" --storage.tsdb.path="${IDIR}/data" --enable-feature="exemplar-storage,native-histograms" --log.level=debug
