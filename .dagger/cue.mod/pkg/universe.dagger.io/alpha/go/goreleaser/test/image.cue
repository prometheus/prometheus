package goreleaser

import (
	"dagger.io/dagger"

	"universe.dagger.io/docker"
	"universe.dagger.io/alpha/go/goreleaser"
)

dagger.#Plan & {
	actions: test: {
		simple: {
			_image: goreleaser.#Image & {}

			verify: docker.#Run & {
				input: _image.output
				entrypoint: []
				command: {
					name: "/bin/sh"
					args: ["-c", """
							goreleaser --version | grep "goreleaser"
						"""]
				}
			}
		}

		custom: {
			_image: goreleaser.#Image & {
				tag: "v1.9.2"
			}

			verify: docker.#Run & {
				input: _image.output
				entrypoint: []
				command: {
					name: "/bin/sh"
					args: ["-c", """
							goreleaser --version | grep "1.9.2"
						"""]
				}
			}
		}
	}
}
