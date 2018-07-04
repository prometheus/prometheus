FROM golang:1.10-alpine3.7 AS builder

ADD . /go/src/github.com/prometheus/prometheus
WORKDIR /go/src/github.com/prometheus/prometheus

RUN apk update && apk upgrade && apk add --no-cache git alpine-sdk

RUN make build

# -----------------------------------------------------------------------------

FROM        quay.io/prometheus/busybox:latest
LABEL maintainer "The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY --from=builder /go/src/github.com/prometheus/prometheus/prometheus                             /bin/prometheus
COPY --from=builder /go/src/github.com/prometheus/prometheus/promtool                               /bin/promtool
COPY --from=builder /go/src/github.com/prometheus/prometheus/documentation/examples/prometheus.yml  /etc/prometheus/prometheus.yml
COPY --from=builder /go/src/github.com/prometheus/prometheus/console_libraries/                     /usr/share/prometheus/console_libraries/
COPY --from=builder /go/src/github.com/prometheus/prometheus/consoles/                              /usr/share/prometheus/consoles/

RUN ln -s /usr/share/prometheus/console_libraries /usr/share/prometheus/consoles/ /etc/prometheus/
RUN mkdir -p /prometheus && \
    chown -R nobody:nogroup etc/prometheus /prometheus

USER       nobody
EXPOSE     9090
VOLUME     [ "/prometheus" ]
WORKDIR    /prometheus
ENTRYPOINT [ "/bin/prometheus" ]
CMD        [ "--config.file=/etc/prometheus/prometheus.yml", \
             "--storage.tsdb.path=/prometheus", \
             "--web.console.libraries=/usr/share/prometheus/console_libraries", \
             "--web.console.templates=/usr/share/prometheus/consoles" ]
