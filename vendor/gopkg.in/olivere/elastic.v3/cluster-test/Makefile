.PHONY: build run-omega-cluster-test

default: build

build:
	go build cluster-test.go

run-omega-cluster-test:
	go run -race cluster-test.go \
		-nodes=http://192.168.2.65:8200,http://192.168.2.64:8200 \
		-n=5 \
		-retries=5 \
		-sniff=true -sniffer=10s \
		-healthcheck=true -healthchecker=5s \
		-errorlog=errors.log

