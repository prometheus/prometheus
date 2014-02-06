FROM       ubuntu:13.10
MAINTAINER Prometheus Team <prometheus-developers@googlegroups.com>
EXPOSE     9090
VOLUME     [ "/prometheus" ]
WORKDIR    /prometheus

ENTRYPOINT [ "/prometheus-src/.build/package/run_prometheus.sh" ]
RUN        apt-get update && apt-get install -yq make git curl sudo mercurial
ADD        . /prometheus-src
RUN        cd /prometheus-src && make binary
