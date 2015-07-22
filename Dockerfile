FROM ubuntu:14.04
MAINTAINER D.Ducatel

WORKDIR /app

RUN apt-get update \
        && apt-get install -y curl git make openssl \
        && curl https://www.gandi.net/static/CAs/GandiStandardSSLCA2.pem -o /usr/local/share/ca-certificates/GandiStandardSSLCA2.crt \
        && update-ca-certificates

COPY . /app

RUN make build \ 
        && cp prometheus promtool /bin/ \
        && mkdir -p /etc/prometheus \
        && mv ./documentation/examples/prometheus.yml /etc/prometheus/prometheus.yml \
        && mv ./console_libraries/ ./consoles/ /etc/prometheus/

EXPOSE     9090
VOLUME     [ "/prometheus" ]
WORKDIR    /prometheus
ENTRYPOINT [ "/bin/prometheus" ]
CMD        [ "-config.file=/etc/prometheus/prometheus.yml", \
             "-storage.local.path=/prometheus", \
             "-web.console.libraries=/etc/prometheus/console_libraries", \
             "-web.console.templates=/etc/prometheus/consoles" ]
