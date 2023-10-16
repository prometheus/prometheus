ARG ARCH="amd64"
ARG OS="linux"
FROM node:18 as builder
ARG ARCH
ARG OS
COPY . /prometheus
ENV GO_VERSION="1.20.10"

RUN if [ $ARCH = "armv7" ] ; then ARCH="armv6l" ; fi && \
    curl -s -L https://go.dev/dl/go${GO_VERSION}.${OS}-${ARCH}.tar.gz -o go.tar.gz && \
    tar -xf go.tar.gz && \ 
    ln -sn /go/bin/go /usr/bin/go && \
    cd /prometheus && \
    make build

FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY --from=builder /prometheus/prometheus        /bin/prometheus
COPY --from=builder /prometheus/promtool          /bin/promtool
COPY documentation/examples/prometheus.yml  /etc/prometheus/prometheus.yml
COPY console_libraries/                     /usr/share/prometheus/console_libraries/
COPY consoles/                              /usr/share/prometheus/consoles/
COPY LICENSE                                /LICENSE
COPY NOTICE                                 /NOTICE
COPY npm_licenses.tar.bz2                   /npm_licenses.tar.bz2

WORKDIR /prometheus
RUN ln -s /usr/share/prometheus/console_libraries /usr/share/prometheus/consoles/ /etc/prometheus/ && \
    chown -R nobody:nobody /etc/prometheus /prometheus

USER       nobody
EXPOSE     9090
VOLUME     [ "/prometheus" ]
ENTRYPOINT [ "/bin/prometheus" ]
CMD        [ "--config.file=/etc/prometheus/prometheus.yml", \
             "--storage.tsdb.path=/prometheus", \
             "--web.console.libraries=/usr/share/prometheus/console_libraries", \
             "--web.console.templates=/usr/share/prometheus/consoles" ]
