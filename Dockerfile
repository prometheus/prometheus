ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"
LABEL org.opencontainers.image.authors="The Prometheus Authors" \
      org.opencontainers.image.vendor="Prometheus" \
      org.opencontainers.image.title="Prometheus" \
      org.opencontainers.image.description="The Prometheus monitoring system and time series database" \
      org.opencontainers.image.source="https://github.com/prometheus/prometheus" \
      org.opencontainers.image.url="https://github.com/prometheus/prometheus" \
      org.opencontainers.image.documentation="https://prometheus.io/docs" \
      org.opencontainers.image.licenses="Apache License 2.0"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/prometheus        /bin/prometheus
COPY .build/${OS}-${ARCH}/promtool          /bin/promtool
COPY documentation/examples/prometheus.yml  /etc/prometheus/prometheus.yml
COPY LICENSE                                /LICENSE
COPY NOTICE                                 /NOTICE
COPY npm_licenses.tar.bz2                   /npm_licenses.tar.bz2

WORKDIR /prometheus
RUN chown -R nobody:nobody /etc/prometheus /prometheus && chmod g+w /prometheus

USER       nobody
EXPOSE     9090
VOLUME     [ "/prometheus" ]
ENTRYPOINT [ "/bin/prometheus" ]
CMD        [ "--config.file=/etc/prometheus/prometheus.yml", \
             "--storage.tsdb.path=/prometheus" ]
