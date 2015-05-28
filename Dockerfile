FROM alpine:edge
MAINTAINER The Prometheus Authors <prometheus-developers@googlegroups.com>

ENV GOPATH /go
COPY . /go/src/github.com/prometheus/prometheus

RUN apk add --update -t build-deps go git mercurial vim \
    && apk add -u musl && rm -rf /var/cache/apk/* \
    && go get github.com/tools/godep \
    && cd /go/src/github.com/prometheus/prometheus \
    && $GOPATH/bin/godep restore && go get -d \
    && ./scripts/embed-static.sh web/static web/templates | gofmt > web/blob/files.go \
    && go build -ldflags " \
            -X main.buildVersion  $(cat VERSION) \
            -X main.buildRevision $(git rev-parse --short HEAD) \
            -X main.buildBranch   $(git rev-parse --abbrev-ref HEAD) \
            -X main.buildUser     root \
            -X main.buildDate     $(date +%Y%m%d-%H:%M:%S) \
            -X main.goVersion     $(go version | awk '{print substr($3,3)}') \
        " -o /bin/prometheus \
    && cd tools/rule_checker && go build -o /bin/rule_checker && cd ../.. \
    && mkdir -p /etc/prometheus \
    && mv ./documentation/examples/prometheus.yml /etc/prometheus/prometheus.yml \
    && mv ./console_libraries/ ./consoles/ /etc/prometheus/ \
    && rm -rf /go \
    && apk del --purge build-deps

EXPOSE     9090
VOLUME     [ "/prometheus" ]
WORKDIR    /prometheus
ENTRYPOINT [ "/bin/prometheus" ]
CMD        [ "-config.file=/etc/prometheus/prometheus.yml", \
             "-storage.local.path=/prometheus", \
             "-web.console.libraries=/etc/prometheus/console_libraries", \
             "-web.console.templates=/etc/prometheus/consoles" ]
