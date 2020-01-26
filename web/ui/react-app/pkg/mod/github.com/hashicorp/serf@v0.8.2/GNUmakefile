GOTOOLS = github.com/mitchellh/gox
VERSION = $(shell awk -F\" '/^const Version/ { print $$2; exit }' cmd/serf/version.go)
GITSHA:=$(shell git rev-parse HEAD)
GITBRANCH:=$(shell git symbolic-ref --short HEAD 2>/dev/null)

default:: test

# bin generates the releasable binaries
bin:: tools
	@sh -c "'$(CURDIR)/scripts/build.sh'"

# cov generates the coverage output
cov:: tools
	gocov test ./... | gocov-html > /tmp/coverage.html
	open /tmp/coverage.html

# dev creates binaries for testing locally - these are put into ./bin and
# $GOPATH
dev::
	@SERF_DEV=1 sh -c "'$(CURDIR)/scripts/build.sh'"

# dist creates the binaries for distibution
dist::
	@sh -c "'$(CURDIR)/scripts/dist.sh' $(VERSION)"

get-tools::
	go get -u -v $(GOTOOLS)

# subnet sets up the require subnet for testing on darwin (osx) - you must run
# this before running other tests if you are on osx.
subnet::
	@sh -c "'$(CURDIR)/scripts/setup_test_subnet.sh'"

# test runs the test suite
test:: subnet tools
	@go list ./... | xargs -n1 go test $(TESTARGS)

# testrace runs the race checker
testrace:: subnet
	go test -race ./... $(TESTARGS)

tools::
	@which gox 2>/dev/null ; if [ $$? -eq 1 ]; then \
		$(MAKE) get-tools; \
	fi

# updatedeps installs all the dependencies needed to test, build, and run
updatedeps:: tools
	go get -u
	go mod tidy
	go mod vendor

vet:: tools
	@echo "--> Running go tool vet $(VETARGS) ."
	@go list ./... \
		| cut -d '/' -f 4- \
		| xargs -n1 \
			go tool vet $(VETARGS) ;\
	if [ $$? -ne 0 ]; then \
		echo ""; \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for reviewal."; \
	fi

.PHONY: default bin cov dev dist get-tools subnet test testrace tools updatedeps vet
