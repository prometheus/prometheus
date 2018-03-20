TEST?=$$(go list ./... | grep -v vendor)
VETARGS?=-all
GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor)

default: test vet

run:
	go run $(CURDIR)/main.go --disable-healthcheck $(ARGS)

test: vet
	go test ./sacloud $(TESTARGS) -v -timeout=120m -parallel=4 ;

test-api: vet
	go test ./api $(TESTARGS) -v -timeout=120m -parallel=4 ;

test-builder: vet
	go test ./builder $(TESTARGS) -v -timeout=120m -parallel=4 ;

test-all: test test-api test-builder

vet: golint
	go vet ./...

golint: fmt
	test -z "$$(golint ./... | grep -v '_string.go' | tee /dev/stderr )"

fmt:
	gofmt -s -l -w $(GOFMT_FILES)

godoc:
	docker-compose up godoc

.PHONY: default test vet fmt golint test-api test-builder test-all run
