package docker

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
)

// Upload an image to a remote repository
#Push: {
	// Destination ref
	dest: #Ref

	// Complete ref after pushing (including digest)
	result: #Ref & _push.result

	// Registry authentication
	auth?: {
		username: string
		secret:   dagger.#Secret
	}

	// Image to push
	image: #Image

	_push: core.#Push & {
		"dest": dest
		if auth != _|_ {
			"auth": auth
		}
		input:  image.rootfs
		config: image.config
	}
}
