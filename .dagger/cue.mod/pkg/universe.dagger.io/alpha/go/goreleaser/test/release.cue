package goreleaser

import (
	"dagger.io/dagger"

	"universe.dagger.io/alpha/go/goreleaser"
)

dagger.#Plan & {
	client: filesystem: "./data/hello": read: contents: dagger.#FS

	actions: test: {
		simple: build: goreleaser.#Release & {
			source: client.filesystem."./data/hello".read.contents

			dryRun:   true
			snapshot: true
		}

		customImage: build: goreleaser.#Release & {
			source: client.filesystem."./data/hello".read.contents

			customImage: goreleaser.#Image & {
				tag: "v1.9.2"
			}

			dryRun:   true
			snapshot: true
		}
	}
}
