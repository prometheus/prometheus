FROM golang:1.7.1-alpine
LABEL maintainer="Kazumichi Yamamoto <yamamoto.febc@gmail.com>"

RUN apk add --no-cache --update ca-certificates git && \
    go get github.com/stretchr/testify/assert && \
    go get golang.org/x/tools/cmd/godoc && \
    go get -u github.com/golang/lint/golint

ENV SRC=$GOPATH/src/github.com/sacloud/libsacloud/
ADD . $SRC
WORKDIR $SRC

ENTRYPOINT [ "go" ]
CMD [ "test", "-v", "./..." ]
