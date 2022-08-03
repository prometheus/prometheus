// Build, ship and run Docker containers in Dagger
package docker

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
)

// Download an image from a remote registry
#Pull: {
	// Source ref.
	source: #Ref

	// When to pull the image
	resolveMode: *"default" | "forcePull" | "preferLocal"

	// Registry authentication
	auth?: {
		username: string
		secret:   dagger.#Secret
	}

	_pull: core.#Pull & {
		"source":      source
		"resolveMode": resolveMode
		if auth != _|_ {
			"auth": auth
		}
	}

	// Downloaded image
	image: #Image & {
		rootfs: _pull.output
		config: _pull.config
	}

	// FIXME: compat with Build API
	output: image
}
