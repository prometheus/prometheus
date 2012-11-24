TEST_ARTIFACTS = prometheus

all: test

test: build
	go test ./...

build:
	$(MAKE) -C model
	go build ./...
	go build .

clean:
	rm -rf $(TEST_ARTIFACTS)
	$(MAKE) -C model clean
	-find . -type f -iname '*~' -exec rm '{}' ';'
	-find . -type f -iname '*#' -exec rm '{}' ';'
	-find . -type f -iname '.*' -exec rm '{}' ';'

format:
	find . -iname '*.go' | grep -v generated | xargs -n1 gofmt -w

.PHONY: build clean format test
