//Deprecated: in favor of universe.dagger.io/alpha package
// .NET operation
package dotnet

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

// A standalone dotnet environment to run dotnet command
#Container: {
	// Container app name
	name: *"dotnet_publisher" | string

	// Source code
	source: dagger.#FS

	// Use dotnet image
	_image: #Image

	_sourcePath: "/src"

	docker.#Run & {
		input:   *_image.output | docker.#Image
		workdir: "/src"
		command: name: "dotnet"
		mounts: "source": {
			dest:     _sourcePath
			contents: source
		}
	}
}
