#!/bin/bash

set -eux

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Put everything on a docker network called 'frank'
# so we can use docker dns.
if ! (docker network ls | grep frank); then
    docker network create frank
fi

start_container() {
    local replicas=$1
    local image=$2
    local basename=$3
    shift 3

    local docker_args=
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --)
            shift
            break
            ;;
            *)
            docker_args="${docker_args} $1"
            shift
            ;;
        esac
    done
    local container_args="$@"

    for i in $(seq ${replicas}); do
        if docker inspect ${basename}${i} >/dev/null 2>&1; then
            docker rm -f ${basename}${i}
        fi
        docker run -d --net=frank --name=${basename}${i} \
            ${docker_args} ${image} ${container_args}
    done
}

# Infrastructure bits
start_container 1 deangiberson/aws-dynamodb-local dynamodb
start_container 1 lphoward/fake-s3 s3
start_container 1 consul consul -p 8500:8500 -- agent -ui -server -client=0.0.0.0 -bootstrap

# Services
start_container 1 prometheus ingestor \
    -v ${DIR}/ingestor-config:/etc/prometheus/
start_container 1 frankenstein distributor \
    -- -consul.hostname=consul1:8500

# Tell distributor about ingestor
sleep 1
curl -X PUT -d '{"hostname": "http://ingestor1:9090/push", "tokens": [0]}' "http://$(docker-machine ip $(docker-machine active)):8500/v1/kv/collectors/localhost"

# Start a prometheus in retrival mode
start_container 1 prometheus retrieval \
    -v ${DIR}/retrieval-config:/etc/prometheus/ \
        -- \
    -config.file=/etc/prometheus/prometheus.yml \
    -web.listen-address=:9091 -retrieval-only \
    -storage.remote.generic-url=http://distributor1:9094/push
