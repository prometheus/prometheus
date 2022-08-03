//Deprecated: in favor of universe.dagger.io/alpha package
package rust

import (
	"dagger.io/dagger"
	"universe.dagger.io/x/contact@kjuulh.io/rust"
	"universe.dagger.io/docker"
)

dagger.#Plan & {
	actions: test: {
		_source: dagger.#Scratch

		simple: {
			// Default rust.#Image
			_image: rust.#Image

			verify: docker.#Run & {
				input: _image.output
				command: {
					name: "/bin/sh"
					args: ["-c", "cargo version | grep '1.6'"]
				}
			}
		}

		custom: {
			_image: rust.#Image & {
				version: "1.56"
			}

			verify: docker.#Run & {
				input: _image.output
				command: {
					name: "/bin/sh"
					args: ["-c", "cargo version | grep '1.56'"]
				}
			}
		}
	}
}
