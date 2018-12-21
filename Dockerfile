FROM        quay.io/prometheus/busybox:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY prometheus                             /bin/prometheus
COPY promtool                               /bin/promtool
COPY documentation/examples/prometheus.yml  /etc/prometheus/prometheus.yml
COPY console_libraries/                     /usr/share/prometheus/console_libraries/
COPY consoles/                              /usr/share/prometheus/consoles/

RUN ln -s /usr/share/prometheus/console_libraries /usr/share/prometheus/consoles/ /etc/prometheus/ && \
    mkdir -p /prometheus && \
    chown -R nobody:nogroup etc/prometheus /prometheus && \
    ln -s /prometheus /etc/prometheus/data

USER       nobody
EXPOSE     9090
VOLUME     [ "/prometheus" ]
WORKDIR    /etc/prometheus
ENTRYPOINT [ "/bin/prometheus" ]
