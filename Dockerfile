FROM alpine:3.2
MAINTAINER The Prometheus Authors <prometheus-developers@googlegroups.com>

ENV GOPATH=/go \
    REPO_PATH=github.com/prometheus/prometheus
COPY . /go/src/github.com/prometheus/prometheus

RUN apk add --update -t build-deps go git mercurial \
    && apk add -u musl && rm -rf /var/cache/apk/* \
    && go get github.com/tools/godep \
    && cd /go/src/$REPO_PATH \
    && $GOPATH/bin/godep restore && go get -d \
    && go build -ldflags " \
            -X $REPO_PATH/version.Version       $(cat version/VERSION) \
            -X $REPO_PATH/version.Revision      $(git rev-parse --short HEAD) \
            -X $REPO_PATH/version.Branch        $(git rev-parse --abbrev-ref HEAD) \
            -X $REPO_PATH/version.BuildUser     root@$(hostname -f) \
            -X $REPO_PATH/version.BuildDate     $(date +%Y%m%d-%H:%M:%S) \
            -X $REPO_PATH/version.GoVersion     $(go version | awk '{print substr($3,3)}') \
        " -o /bin/prometheus $REPO_PATH/cmd/prometheus \
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
