# Cannot use busybox image since Prometheus depends on libc.
FROM base

MAINTAINER Prometheus Team <prometheus-developers@googlegroups.com>
EXPOSE 9090
ENTRYPOINT ["/opt/prometheus/run_prometheus.sh"]
ADD .build/package/ /opt/prometheus
