FROM debian:stable

ENV GOVERSION 1.11.4

RUN apt-get update -y && \
    apt-get install --no-install-recommends -y -q \
            build-essential \
            ca-certificates \
            curl \
            git \
            zip

RUN mkdir /goroot && \
    mkdir /gopath && \
    curl https://storage.googleapis.com/golang/go${GOVERSION}.linux-amd64.tar.gz | \
         tar xzf - -C /goroot --strip-components=1

# We want to ensure that release builds never have any cgo dependencies so we
# switch that off at the highest level.
ENV CGO_ENABLED 0
ENV GOROOT /goroot
ENV PATH $GOROOT/bin:/root/go/bin:$PATH

RUN mkdir -p /serf
WORKDIR /serf
CMD ./scripts/dist_build.sh
