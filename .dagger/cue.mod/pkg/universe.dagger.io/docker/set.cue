package docker

import (
	"dagger.io/dagger/core"
)

// Change image config
#Set: {
	// The source image
	input: #Image

	// The image config to change
	config: core.#ImageConfig

	_set: core.#Set & {
		"input":  input.config
		"config": config
	}

	// Resulting image with the config changes
	output: #Image & {
		rootfs: input.rootfs
		config: _set.output
	}
}
