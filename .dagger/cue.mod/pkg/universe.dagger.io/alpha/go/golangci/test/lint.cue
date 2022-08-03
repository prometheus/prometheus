package golangci

import (
	"dagger.io/dagger"

	"universe.dagger.io/alpha/go/golangci"
)

dagger.#Plan & {
	client: filesystem: "./data/hello": read: contents: dagger.#FS

	actions: test: golangci.#Lint & {
		source: client.filesystem."./data/hello".read.contents
	}
}
