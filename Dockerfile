# Set build arguments for architecture and OS
ARG ARCH="amd64"
ARG OS="linux"

# Use Prometheus BusyBox base image
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest

LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"
LABEL org.opencontainers.image.source="https://github.com/prometheus/prometheus"

# Set environment variables for configuration and storage paths
ENV PROMETHEUS_CONFIG_FILE=/etc/prometheus/prometheus.yml
ENV PROMETHEUS_STORAGE_PATH=/prometheus

# Copy Prometheus binaries and configuration
COPY .build/${OS}-${ARCH}/prometheus        /bin/prometheus
COPY .build/${OS}-${ARCH}/promtool          /bin/promtool
COPY documentation/examples/prometheus.yml  /etc/prometheus/prometheus.yml
COPY LICENSE                                /LICENSE
COPY NOTICE                                 /NOTICE
COPY npm_licenses.tar.bz2                   /npm_licenses.tar.bz2

# Create a dedicated user and group for Prometheus
RUN addgroup -S prometheus && adduser -S prometheus -G prometheus \
    && chown -R prometheus:prometheus /etc/prometheus /prometheus \
    && chmod g+w /prometheus

WORKDIR /prometheus

# Switch to the prometheus user
USER prometheus

EXPOSE 9090
VOLUME [ "/prometheus" ]

# Add a healthcheck to verify Prometheus is running
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD wget --spider -q http://localhost:9090/-/healthy || exit 1

ENTRYPOINT [ "/bin/prometheus" ]
CMD [ "--config.file=${PROMETHEUS_CONFIG_FILE}", "--storage.tsdb.path=${PROMETHEUS_STORAGE_PATH}" ]