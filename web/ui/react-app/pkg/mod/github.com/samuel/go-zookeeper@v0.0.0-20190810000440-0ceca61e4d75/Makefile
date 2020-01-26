# make file to hold the logic of build and test setup
ZK_VERSION ?= 3.4.12

ZK = zookeeper-$(ZK_VERSION)
ZK_URL = "https://archive.apache.org/dist/zookeeper/$(ZK)/$(ZK).tar.gz"

PACKAGES := $(shell go list ./... | grep -v examples)

.DEFAULT_GOAL := test

$(ZK):
	wget $(ZK_URL)
	tar -zxf $(ZK).tar.gz
	# we link to a standard directory path so then the tests dont need to find based on version
	# in the test code. this allows backward compatable testing.
	ln -s $(ZK) zookeeper

.PHONY: install-covertools
install-covertools:
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover

.PHONY: setup
setup: $(ZK) install-covertools

.PHONY: lint
lint:
	go fmt ./...
	go vet ./...

.PHONY: build
build:
	go build ./...

.PHONY: test
test: build
	go test -timeout 500s -v -race -covermode atomic -coverprofile=profile.cov $(PACKAGES)
	# ignore if we fail to publish coverage
	-goveralls -coverprofile=profile.cov -service=travis-ci
