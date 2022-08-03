package go

import (
	"dagger.io/dagger"
	"universe.dagger.io/go"
	"universe.dagger.io/alpine"
)

dagger.#Plan & {
	actions: test: {
		_source: dagger.#Scratch & {}

		simple: go.#Container & {
			source: _source
			command: {
				name: "go"
				args: ["version"]
			}
		}

		override: {
			base: alpine.#Build & {
				packages: go: _
			}

			command: go.#Container & {
				input:  base.output
				source: _source
				command: {
					name: "go"
					args: ["version"]
				}
			}
		}
	}
}
