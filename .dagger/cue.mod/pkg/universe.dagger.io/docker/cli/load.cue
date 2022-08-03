package cli

import (
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

// Load an image into a docker daemon
#Load: {
	// Image to load
	image: docker.#Image

	// Name and optionally a tag in the 'name:tag' format
	tag: docker.#Ref

	// Exported image ID
	imageID: _export.imageID

	// Root filesystem with exported file
	result: _export.output

	_export: core.#Export & {
		"tag":  tag
		input:  image.rootfs
		config: image.config
	}

	#Run & {
		mounts: src: {
			dest:     "/src"
			contents: _export.output
		}
		command: {
			name: "load"
			flags: "-i": "/src/image.tar"
		}
	}
}
