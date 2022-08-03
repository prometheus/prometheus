package main

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"github.com/prometheus/promci"
)

dagger.#Plan & {
	client: filesystem: "..": read: contents: dagger.#FS
	client: filesystem: build: {
		write: {contents: actions.build.output, path: ".."}
	}

	actions: {
		test: promci.#Build & {
			cmd: "make test GO_ONLY=1", source: client.filesystem."..".read.contents
		}
		ui: promci.#Build & {
			cmd: "make assets-tarball ui-lint ui-test", source: client.filesystem."..".read.contents
		}
		build: promci.#Build & {
			cmd: "make build", binaries: ["prometheus"]
			source: client.filesystem."..".read.contents
		}
		all: core.#Nop & {
			input: [test.output, build.output, ui.output]}
	}
}
