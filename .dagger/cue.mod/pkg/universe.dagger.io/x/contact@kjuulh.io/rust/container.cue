//Deprecated: in favor of universe.dagger.io/alpha package
package rust

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

#Container: {
	// Source code
	source: dagger.#FS

	// Rust image
	_image: #Image

	_sourcePath: "/src"

	docker.#Run & {
		input:   *_image.output | docker.#Image
		workdir: _sourcePath
		command: name: "cargo"
		mounts: "source": {
			dest:     _sourcePath
			contents: source
		}
	}
}
