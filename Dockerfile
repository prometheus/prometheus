FROM       ubuntu:13.10
MAINTAINER Prometheus Team <prometheus-developers@googlegroups.com>
EXPOSE     9090
VOLUME     [ "/prometheus" ]
WORKDIR    /prometheus

ENTRYPOINT [ "/prometheus-src/prometheus" ]
CMD        [ "-config.file=/prometheus.conf" ]
RUN        apt-get update && apt-get install -yq make git curl sudo mercurial vim-common gcc
ADD        . /prometheus-src
RUN        cd /prometheus-src && make tools binary
ADD        ./documentation/examples/prometheus.conf /prometheus.conf
