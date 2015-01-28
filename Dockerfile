FROM       golang:1.4
MAINTAINER The Prometheus Authors <prometheus-developers@googlegroups.com>
RUN        apt-get -qy update && apt-get -qy install vim-common gcc mercurial && \
           go get github.com/tools/godep

WORKDIR    /go/src/github.com/prometheus/prometheus
ADD        . /go/src/github.com/prometheus/prometheus

RUN  godep restore && go get -d
RUN  ./utility/embed-static.sh web/static web/templates | gofmt > web/blob/files.go

RUN  go build -ldflags " \
       -X main.buildVersion  $(cat VERSION) \
       -X main.buildRevision $(git rev-parse --short HEAD) \
       -X main.buildBranch   $(git rev-parse --abbrev-ref HEAD) \
       -X main.buildUser     root \
       -X main.buildDate     $(date +%Y%m%d-%H:%M:%S) \
       -X main.goVersion     $GOLANG_VERSION \
     "
RUN  cd tools/rule_checker && go build
ADD  ./documentation/examples/prometheus.conf /prometheus.conf

EXPOSE     9090
VOLUME     [ "/prometheus" ]
WORKDIR    /prometheus
ENTRYPOINT [ "/go/src/github.com/prometheus/prometheus/prometheus" ]
CMD        [ "-logtostderr", "-config.file=/prometheus.conf", \
             "-web.console.libraries=/go/src/github.com/prometheus/prometheus/console_libraries", \
             "-web.console.templates=/go/src/github.com/prometheus/prometheus/consoles" ]
