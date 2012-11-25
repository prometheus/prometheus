TEST_ARTIFACTS = prometheus search_index

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

search_index:
	godoc -index -write_index -index_files='search_index'

documentation: search_index
	godoc -http=:6060 -index -index_files='search_index'

.PHONY: build clean format test
