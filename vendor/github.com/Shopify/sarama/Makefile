default: fmt vet errcheck test

test:
	go test -v -timeout 60s -race ./...

vet:
	go vet ./...

errcheck:
	errcheck github.com/Shopify/sarama/...

fmt:
	@if [ -n "$$(go fmt ./...)" ]; then echo 'Please run go fmt on your code.' && exit 1; fi

install_dependencies: install_errcheck get

install_errcheck:
	go get github.com/kisielk/errcheck

get:
	go get -t
